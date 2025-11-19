package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/P0lapa/judge-executor/internal/pkg/languages"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
)

const (
	maxInputSize  = 1024 * 1024
	maxCodeSize   = 1024 * 1024
	maxOutputSize = 1024 * 1024
	maxInputs     = 100
)

// ExecuteInDocker запускает код в Docker контейнере
func ExecuteInDocker(ctx context.Context, lang string, code string, inputs []string, timeLimitMs int32, memLimitMb int32, additionalArgs []string) ([]string, []string, []float64, []int32, []int32, string) {
	if len(code) > maxCodeSize {
		return nil, nil, nil, nil, nil, "Code too large"
	}
	if len(inputs) > maxInputs {
		return nil, nil, nil, nil, nil, "Too many inputs"
	}
	for _, input := range inputs {
		if len(input) > maxInputSize {
			return nil, nil, nil, nil, nil, "Input too large"
		}
	}

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Sprintf("docker client error: %v", err)
	}
	defer cli.Close()

	// Image
	var image string
	switch lang {
	case "cpp":
		image = "gcc:latest"
	case "java":
		image = "eclipse-temurin:17-jdk"
	case "python":
		image = "python:3.10"
	case "kotlin":
		image = "my-openjdk-kotlin:17"
	default:
		return nil, nil, nil, nil, nil, "Unsupported lang"
	}

	// Temp dir
	tempDir, err := os.MkdirTemp("", "judge-*")
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Sprintf("temp dir error: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Write code
	codePath := filepath.Join(tempDir, "code"+GetFileExt(lang))
	if err := os.WriteFile(codePath, []byte(code), 0644); err != nil {
		return nil, nil, nil, nil, nil, fmt.Sprintf("write code error: %v", err)
	}

	// Config
	config, err := languages.GetConfig(lang)
	if err != nil {
		return nil, nil, nil, nil, nil, err.Error()
	}

	// Compile once
	if config.NeedsCompile {
		compileCmd := config.CompileCmd
		if len(additionalArgs) > 0 {
			compileCmd = append(compileCmd, additionalArgs...)
		}
		compileStr := strings.Join(append([]string{"cd /workdir;"}, compileCmd...), " ")
		_, _, _, _, _, compileErrMsg := runContainer(cli, ctx, image, tempDir, compileStr, 10000, 0)
		if compileErrMsg != "" {
			return nil, nil, nil, nil, nil, "Compilation failed: " + compileErrMsg
		}
	}

	// Run loop
	runCmd := config.RunCmd
	runStr := fmt.Sprintf("cd /workdir; timeout %ds %s < input.txt", timeLimitMs/1000, strings.Join(runCmd, " "))

	outputs := make([]string, 0, len(inputs))
	stderrs := make([]string, 0, len(inputs))
	timeUseds := make([]float64, 0, len(inputs))
	memUseds := make([]int32, 0, len(inputs))
	exitCodes := make([]int32, 0, len(inputs))

	for i, input := range inputs {
		inputPath := filepath.Join(tempDir, "input.txt")
		if err := os.WriteFile(inputPath, []byte(input), 0644); err != nil {
			return nil, nil, nil, nil, nil, fmt.Sprintf("write input error: %v", err)
		}

		output, stderr, timeUsed, memUsed, exitCode, errMsg := runContainer(cli, ctx, image, tempDir, runStr, timeLimitMs, memLimitMb)
		if errMsg != "" {
			return nil, nil, nil, nil, nil, errMsg
		}

		outputs = append(outputs, output)
		stderrs = append(stderrs, stderr)
		timeUseds = append(timeUseds, timeUsed)
		memUseds = append(memUseds, memUsed)
		exitCodes = append(exitCodes, exitCode)

		logrus.Infof("Input %d processed: time=%.6fs, memory=%dkb, exit=%d", i, timeUsed, memUsed, exitCode)
	}

	logrus.Infof("Returning from ExecuteInDocker: %d outputs, %d time values, %d memory values",
		len(outputs), len(timeUseds), len(memUseds))

	return outputs, stderrs, timeUseds, memUseds, exitCodes, ""
}

// runContainer helper: создаёт и запускает контейнер
func runContainer(cli *client.Client, ctx context.Context, image string, hostDir string, cmdStr string, timeLimitMs int32, memLimitMb int32) (string, string, float64, int32, int32, string) {
	start := time.Now()

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: image,
		Cmd:   []string{"/bin/sh", "-c", cmdStr},
		Tty:   false,
	}, &container.HostConfig{
		Mounts: []mount.Mount{
			{Type: "bind", Source: hostDir, Target: "/workdir"},
		},
		Resources: container.Resources{
			Memory:   int64(memLimitMb) * 1024 * 1024,
			NanoCPUs: 1e9,
		},
	}, &network.NetworkingConfig{}, &ocispec.Platform{}, "")
	if err != nil {
		return "", "", 0, 0, 0, fmt.Sprintf("container create: %v", err)
	}

	containerID := resp.ID
	defer func() {
		// Cleanup в отдельной горутине чтобы не влиять на время
		go func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cli.ContainerRemove(cleanupCtx, containerID, container.RemoveOptions{Force: true})
		}()
	}()

	if err := cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return "", "", 0, 0, 0, fmt.Sprintf("container start: %v", err)
	}

	// Ждем завершения с таймаутом
	waitCtx, waitCancel := context.WithTimeout(ctx, time.Duration(timeLimitMs)*time.Millisecond)
	defer waitCancel()

	statusCh, errCh := cli.ContainerWait(waitCtx, containerID, container.WaitConditionNotRunning)

	var exitCode int32 = 0
	select {
	case err := <-errCh:
		if err != nil {
			cli.ContainerKill(ctx, containerID, "SIGKILL")
			return "", "", 0, 0, 124, "Timeout or error: " + err.Error()
		}
	case status := <-statusCh:
		exitCode = int32(status.StatusCode)
		if status.StatusCode != 0 {
			logrus.Warnf("Non-zero exit: %d", status.StatusCode)
		}
	}

	// Получаем логи
	out, err := cli.ContainerLogs(ctx, containerID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return "", "", 0, 0, 0, fmt.Sprintf("logs error: %v", err)
	}
	defer out.Close()

	var stdout, stderr bytes.Buffer
	_, err = stdcopy.StdCopy(&stdout, &stderr, out)
	if err != nil {
		logrus.Warnf("Error copying stdout/stderr: %v", err)
	}

	// Время выполнения - ТОЛЬКО до получения логов
	timeUsed := time.Since(start).Seconds()

	// Получаем использование памяти
	memUsed := getMemoryUsage(cli, ctx, containerID)

	output := strings.TrimSpace(stdout.String())
	if len(output) > maxOutputSize {
		output = output[:maxOutputSize] + "... truncated"
	}

	return output, stderr.String(), timeUsed, memUsed, exitCode, ""
}

// getMemoryUsage получает использование памяти из статистики контейнера
func getMemoryUsage(cli *client.Client, ctx context.Context, containerID string) int32 {
	// Даем контейнеру немного времени для сбора статистики
	time.Sleep(100 * time.Millisecond)

	stats, err := cli.ContainerStats(ctx, containerID, false)
	if err != nil {
		logrus.Warnf("Failed to get container stats: %v", err)
		return 0
	}
	defer stats.Body.Close()

	var data struct {
		MemoryStats struct {
			Usage int64 `json:"usage"`
			Limit int64 `json:"limit"`
		} `json:"memory_stats"`
		PreciseStats struct {
			Rss int64 `json:"rss"` // Более точный показатель
		} `json:"precise_stats"`
	}

	body, err := io.ReadAll(stats.Body)
	if err != nil {
		logrus.Warnf("Failed to read stats body: %v", err)
		return 0
	}

	if err := json.Unmarshal(body, &data); err != nil {
		logrus.Warnf("Failed to decode memory stats: %v", err)
		return 0
	}

	// Пробуем разные источники данных о памяти
	var memUsed int64
	if data.PreciseStats.Rss > 0 {
		memUsed = data.PreciseStats.Rss // Более точные данные
	} else if data.MemoryStats.Usage > 0 {
		memUsed = data.MemoryStats.Usage // Основные данные
	} else {
		// Если нет данных, используем эмпирические значения
		return 1024 // Минимальное значение 1MB
	}

	// Конвертируем в KB
	memUsedKB := int32(memUsed / 1024)

	// Проверяем на корректность
	if memUsedKB <= 0 {
		return 1024 // Возвращаем минимальное значение
	}

	logrus.Debugf("Memory usage: %d KB", memUsedKB)
	return memUsedKB
}

// GetFileExt helper
func GetFileExt(lang string) string {
	config, _ := languages.GetConfig(lang)
	return config.FileExt
}

// Run loop - параллельная версия
func ExecuteInDockerParallel(ctx context.Context, lang string, code string, inputs []string, timeLimitMs int32, memLimitMb int32, additionalArgs []string) ([]string, []string, []float64, []int32, []int32, string) {
	if len(code) > maxCodeSize {
		return nil, nil, nil, nil, nil, "Code too large"
	}
	if len(inputs) > maxInputs {
		return nil, nil, nil, nil, nil, "Too many inputs"
	}
	for _, input := range inputs {
		if len(input) > maxInputSize {
			return nil, nil, nil, nil, nil, "Input too large"
		}
	}

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Sprintf("docker client error: %v", err)
	}
	defer cli.Close()

	// Image
	var image string
	switch lang {
	case "cpp":
		image = "gcc:latest"
	case "java":
		image = "eclipse-temurin:17-jdk"
	case "python":
		image = "python:3.10"
	case "kotlin":
		image = "my-openjdk-kotlin:17"
	default:
		return nil, nil, nil, nil, nil, "Unsupported lang"
	}

	// Основная временная директория для компиляции
	mainTempDir, err := os.MkdirTemp("", "judge-main-*")
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Sprintf("temp dir error: %v", err)
	}
	defer os.RemoveAll(mainTempDir)

	// Write code в основную директорию
	codePath := filepath.Join(mainTempDir, "code"+GetFileExt(lang))
	if err := os.WriteFile(codePath, []byte(code), 0644); err != nil {
		return nil, nil, nil, nil, nil, fmt.Sprintf("write code error: %v", err)
	}

	// Config
	config, err := languages.GetConfig(lang)
	if err != nil {
		return nil, nil, nil, nil, nil, err.Error()
	}

	// Compile once в основной директории
	if config.NeedsCompile {
		compileCmd := config.CompileCmd
		if len(additionalArgs) > 0 {
			compileCmd = append(compileCmd, additionalArgs...)
		}
		compileStr := strings.Join(append([]string{"cd /workdir;"}, compileCmd...), " ")
		_, _, _, _, _, compileErrMsg := runContainer(cli, ctx, image, mainTempDir, compileStr, 10000, 0)
		if compileErrMsg != "" {
			return nil, nil, nil, nil, nil, "Compilation failed: " + compileErrMsg
		}
	}

	// Подготавливаем команду для выполнения
	runCmd := config.RunCmd
	runStr := fmt.Sprintf("cd /workdir; %s < input.txt", strings.Join(runCmd, " "))

	type result struct {
		output   string
		stderr   string
		timeUsed float64
		memUsed  int32
		exitCode int32
		index    int
	}

	results := make(chan result, len(inputs))
	errors := make(chan error, len(inputs))
	var wg sync.WaitGroup

	// Семафор для ограничения количества одновременно запущенных контейнеров
	sem := make(chan struct{}, 10) // Максимум 3 параллельных контейнера

	for i, input := range inputs {
		wg.Add(1)
		go func(idx int, inp string) {
			defer wg.Done()

			// Ограничиваем количество параллельных контейнеров
			sem <- struct{}{}
			defer func() { <-sem }()

			// Создаем временную директорию для каждого теста
			testDir, err := os.MkdirTemp("", fmt.Sprintf("judge-%d-*", idx))
			if err != nil {
				errors <- fmt.Errorf("failed to create temp dir for test %d: %v", idx, err)
				return
			}
			defer os.RemoveAll(testDir)

			// Копируем ВСЕ файлы из основной директории в тестовую
			if err := copyDir(mainTempDir, testDir); err != nil {
				errors <- fmt.Errorf("failed to copy files for test %d: %v", idx, err)
				return
			}

			// Записываем input (перезаписываем существующий если есть)
			inputPath := filepath.Join(testDir, "input.txt")
			if err := os.WriteFile(inputPath, []byte(inp), 0644); err != nil {
				errors <- fmt.Errorf("failed to write input for test %d: %v", idx, err)
				return
			}

			output, stderr, timeUsed, memUsed, exitCode, errMsg := runContainer(cli, ctx, image, testDir, runStr, timeLimitMs, memLimitMb)
			if errMsg != "" {
				errors <- fmt.Errorf("test %d failed: %s", idx, errMsg)
				return
			}

			results <- result{
				output:   output,
				stderr:   stderr,
				timeUsed: timeUsed,
				memUsed:  memUsed,
				exitCode: exitCode,
				index:    idx,
			}
		}(i, input)
	}

	// Закрываем каналы после завершения всех горутин
	go func() {
		wg.Wait()
		close(results)
		close(errors)
	}()

	// Собираем результаты
	outputs := make([]string, len(inputs))
	stderrs := make([]string, len(inputs))
	timeUseds := make([]float64, len(inputs))
	memUseds := make([]int32, len(inputs))
	exitCodes := make([]int32, len(inputs))

	// Обрабатываем ошибки
	for err := range errors {
		logrus.Errorf("Error in parallel execution: %v", err)
		return nil, nil, nil, nil, nil, err.Error()
	}

	// Собираем результаты в правильном порядке
	for res := range results {
		outputs[res.index] = res.output
		stderrs[res.index] = res.stderr
		timeUseds[res.index] = res.timeUsed
		memUseds[res.index] = res.memUsed
		exitCodes[res.index] = res.exitCode

		logrus.Infof("Input %d processed: time=%.6fs, memory=%dkb, exit=%d",
			res.index, res.timeUsed, res.memUsed, res.exitCode)
	}

	logrus.Infof("Returning from ExecuteInDockerParallel: %d outputs, %d time values, %d memory values",
		len(outputs), len(timeUseds), len(memUseds))

	return outputs, stderrs, timeUseds, memUseds, exitCodes, ""
}

// copyDir копирует всю директорию из src в dst
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Вычисляем относительный путь
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		// Копируем файл
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, info.Mode())
	})
}
