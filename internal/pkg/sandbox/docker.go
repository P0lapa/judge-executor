package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	judgecore "github.com/P0lapa/judge-executor/internal/pkg/judge"
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

	metaPrefix = "__JUDGE_META__:"
)

type execMetrics struct {
	TimeUsedSec    float64 `json:"time_used_sec"`
	MemoryUsedKB   int32   `json:"memory_used_kb"`
	ExitCode       int32   `json:"exit_code"`
	TimedOut       bool    `json:"timed_out"`
	MemoryExceeded bool    `json:"memory_exceeded"`
}

func JudgeInDocker(
	ctx context.Context,
	lang string,
	code string,
	testCases []judgecore.RunResult,
	timeLimitMs int32,
	memLimitMb int32,
	additionalArgs []string,
	comparison judgecore.ComparisonMode,
) ([]judgecore.TestResult, string) {
	runResults, errMsg := runInDocker(ctx, lang, code, testCases, timeLimitMs, memLimitMb, additionalArgs)
	if errMsg != "" {
		return nil, errMsg
	}

	results := make([]judgecore.TestResult, 0, len(runResults))
	for _, runResult := range runResults {
		results = append(results, judgecore.BuildTestResult(runResult, comparison))
	}

	return results, ""
}

func GenerateOutputsInDocker(
	ctx context.Context,
	lang string,
	code string,
	inputs []string,
	timeLimitMs int32,
	memLimitMb int32,
	additionalArgs []string,
) ([]judgecore.RunResult, string) {
	testCases := make([]judgecore.RunResult, 0, len(inputs))
	for _, input := range inputs {
		testCases = append(testCases, judgecore.RunResult{Input: input})
	}

	return runInDocker(ctx, lang, code, testCases, timeLimitMs, memLimitMb, additionalArgs)
}

func runInDocker(
	ctx context.Context,
	lang string,
	code string,
	testCases []judgecore.RunResult,
	timeLimitMs int32,
	memLimitMb int32,
	additionalArgs []string,
) ([]judgecore.RunResult, string) {
	if err := validateCode(code); err != nil {
		return nil, err.Error()
	}
	if err := validateRunCases(testCases); err != nil {
		return nil, err.Error()
	}

	config, err := languages.GetConfig(lang)
	if err != nil {
		return nil, err.Error()
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Sprintf("docker client error: %v", err)
	}
	defer cli.Close()

	containerWorkdirRoot, hostWorkdirRoot := getWorkdirRoots()
	workDir, hostWorkDir, err := prepareWorkdir(containerWorkdirRoot, hostWorkdirRoot)
	if err != nil {
		return nil, fmt.Sprintf("temp dir error: %v", err)
	}
	defer os.RemoveAll(workDir)

	if err := os.WriteFile(filepath.Join(workDir, config.SourceFile), []byte(code), 0644); err != nil {
		return nil, fmt.Sprintf("write code error: %v", err)
	}

	if compileErr := compileIfNeeded(ctx, cli, config, hostWorkDir, additionalArgs); compileErr != "" {
		return nil, compileErr
	}

	containerID, err := startRunnerContainer(ctx, cli, config.Image, hostWorkDir)
	if err != nil {
		return nil, err.Error()
	}
	defer removeContainer(cli, containerID)

	results := make([]judgecore.RunResult, 0, len(testCases))
	for i, testCase := range testCases {
		inputPath := filepath.Join(workDir, "input.txt")
		if err := os.WriteFile(inputPath, []byte(testCase.Input), 0644); err != nil {
			return nil, fmt.Sprintf("write input error: %v", err)
		}

		runResult, err := executeSingleTest(ctx, cli, containerID, config, testCase, timeLimitMs, memLimitMb)
		if err != nil {
			return nil, fmt.Sprintf("test %d failed: %v", i, err)
		}

		results = append(results, runResult)
	}

	return results, ""
}

func getWorkdirRoots() (string, string) {
	containerRoot := os.Getenv("JUDGE_WORKDIR_CONTAINER")
	if containerRoot == "" {
		containerRoot = "/judge-workdir"
	}

	hostRoot := os.Getenv("JUDGE_WORKDIR_HOST")
	if hostRoot == "" {
		hostRoot = containerRoot
	}

	return containerRoot, hostRoot
}

func prepareWorkdir(containerRoot, hostRoot string) (string, string, error) {
	if err := os.MkdirAll(containerRoot, 0755); err != nil {
		return "", "", err
	}

	containerDir, err := os.MkdirTemp(containerRoot, "judge-*")
	if err != nil {
		return "", "", err
	}

	hostDir := filepath.ToSlash(filepath.Join(hostRoot, filepath.Base(containerDir)))
	return containerDir, hostDir, nil
}

func validateCode(code string) error {
	if len(code) > maxCodeSize {
		return errors.New("code too large")
	}
	return nil
}

func validateRunCases(testCases []judgecore.RunResult) error {
	if len(testCases) == 0 {
		return errors.New("no test cases provided")
	}
	if len(testCases) > maxInputs {
		return errors.New("too many test cases")
	}
	for _, testCase := range testCases {
		if len(testCase.Input) > maxInputSize {
			return errors.New("input too large")
		}
		if testCase.ExpectedOutput == "" {
			continue
		}
		if len(testCase.ExpectedOutput) > maxOutputSize {
			return errors.New("expected output too large")
		}
	}
	return nil
}

func compileIfNeeded(ctx context.Context, cli *client.Client, config *languages.LanguageConfig, workDir string, additionalArgs []string) string {
	if !config.NeedsCompile {
		return ""
	}

	cmd := append([]string{}, config.CompileCmd...)
	if len(additionalArgs) > 0 {
		cmd = append(cmd, additionalArgs...)
	}

	command := "cd /workdir && " + shellJoin(cmd)
	stdout, stderr, _, _, err := runEphemeralContainer(ctx, cli, config.Image, workDir, command, 0)
	if err != nil {
		return fmt.Sprintf("compilation failed: %v\n%s%s", err, stdout, stderr)
	}

	return ""
}

func startRunnerContainer(ctx context.Context, cli *client.Client, image, workDir string) (string, error) {
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: image,
		Cmd:   []string{"/bin/sh", "-lc", "trap 'exit 0' TERM INT; while true; do sleep 1; done"},
		Tty:   false,
	}, &container.HostConfig{
		Mounts: []mount.Mount{
			{Type: mount.TypeBind, Source: workDir, Target: "/workdir"},
		},
		Resources: container.Resources{
			Memory:   0,
			NanoCPUs: 1e9,
		},
	}, &network.NetworkingConfig{}, &ocispec.Platform{}, "")
	if err != nil {
		return "", fmt.Errorf("container create: %w", err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("container start: %w", err)
	}

	return resp.ID, nil
}

func executeSingleTest(
	ctx context.Context,
	cli *client.Client,
	containerID string,
	config *languages.LanguageConfig,
	testCase judgecore.RunResult,
	timeLimitMs int32,
	memLimitMb int32,
) (judgecore.RunResult, error) {
	command := buildExecCommand(config, timeLimitMs, memLimitMb)
	execID, err := createExec(ctx, cli, containerID, command)
	if err != nil {
		return judgecore.RunResult{}, err
	}

	attachResp, err := cli.ContainerExecAttach(ctx, execID, container.ExecAttachOptions{})
	if err != nil {
		return judgecore.RunResult{}, fmt.Errorf("exec attach: %w", err)
	}
	defer attachResp.Close()

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, attachResp.Reader); err != nil && !errors.Is(err, io.EOF) {
		return judgecore.RunResult{}, fmt.Errorf("read exec output: %w", err)
	}

	inspect, err := cli.ContainerExecInspect(ctx, execID)
	if err != nil {
		return judgecore.RunResult{}, fmt.Errorf("exec inspect: %w", err)
	}

	stdout, stderr, metrics, err := parseExecResult(stdoutBuf.String(), stderrBuf.String())
	if err != nil {
		return judgecore.RunResult{}, err
	}
	if metrics.ExitCode == 0 {
		metrics.ExitCode = int32(inspect.ExitCode)
	}

	if len(stdout) > maxOutputSize {
		stdout = stdout[:maxOutputSize] + "... truncated"
	}
	if len(stderr) > maxOutputSize {
		stderr = stderr[:maxOutputSize] + "... truncated"
	}

	if !metrics.MemoryExceeded && memLimitMb > 0 && metrics.MemoryUsedKB >= memLimitMb*1024 {
		metrics.MemoryExceeded = true
	}

	result := judgecore.RunResult{
		Input:          testCase.Input,
		ExpectedOutput: testCase.ExpectedOutput,
		ActualOutput:   stdout,
		Stderr:         stderr,
		TimeUsedSec:    metrics.TimeUsedSec,
		MemoryUsedKB:   metrics.MemoryUsedKB,
		ExitCode:       metrics.ExitCode,
		TimedOut:       metrics.TimedOut,
		MemoryExceeded: metrics.MemoryExceeded,
	}

	return result, nil
}

func createExec(ctx context.Context, cli *client.Client, containerID, command string) (string, error) {
	resp, err := cli.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          []string{"/bin/sh", "-lc", command},
		AttachStdout: true,
		AttachStderr: true,
		WorkingDir:   "/workdir",
	})
	if err != nil {
		return "", fmt.Errorf("exec create: %w", err)
	}
	return resp.ID, nil
}

func buildExecCommand(config *languages.LanguageConfig, timeLimitMs int32, memLimitMb int32) string {
	timeoutSec := max(1, int((timeLimitMs+999)/1000))
	cpuLimitSec := timeoutSec
	memLimitKB := 0
	if memLimitMb > 0 {
		memLimitKB = int(memLimitMb) * 1024
	}

	safeRun := shellJoin(config.RunCmd)
	parts := []string{
		"cd /workdir",
		"ulimit -t " + strconv.Itoa(cpuLimitSec),
	}
	if memLimitKB > 0 && !config.IsJVM {
		parts = append(parts, "ulimit -v "+strconv.Itoa(memLimitKB))
	}

	parts = append(parts,
		"set +e",
		"__judge_stderr=$(mktemp)",
		"__judge_meta=",
		"if command -v /usr/bin/time >/dev/null 2>&1; then "+
			fmt.Sprintf("/usr/bin/time -f '%s{\"time_used_sec\":%%e,\"memory_used_kb\":%%M,\"exit_code\":%%x,\"timed_out\":false,\"memory_exceeded\":false}' -o \"$__judge_stderr\" timeout -k 1s %ds %s < input.txt; ", metaPrefix, timeoutSec, safeRun)+
			"__judge_exit=$?; __judge_meta=$(cat \"$__judge_stderr\"); "+
			"else "+
			fmt.Sprintf("__judge_start=$(date +%%s%%N); timeout -k 1s %ds %s < input.txt 2>\"$__judge_stderr\"; ", timeoutSec, safeRun)+
			"__judge_exit=$?; __judge_end=$(date +%s%N); "+
			"__judge_elapsed=$(awk \"BEGIN { printf \\\"%.6f\\\", ("+"$__judge_end - $__judge_start"+")/1000000000 }\"); "+
			"__judge_meta=$(printf '%s{\"time_used_sec\":%s,\"memory_used_kb\":0,\"exit_code\":%s,\"timed_out\":false,\"memory_exceeded\":false}' \""+metaPrefix+"\" \"$__judge_elapsed\" \"$__judge_exit\"); "+
			"fi",
		"if [ \"$__judge_exit\" -eq 124 ]; then __judge_meta=$(printf '%s{\"time_used_sec\":0,\"memory_used_kb\":0,\"exit_code\":124,\"timed_out\":true,\"memory_exceeded\":false}' \""+metaPrefix+"\"); fi",
		"if [ \"$__judge_exit\" -eq 137 ]; then __judge_meta=$(printf '%s{\"time_used_sec\":0,\"memory_used_kb\":0,\"exit_code\":137,\"timed_out\":false,\"memory_exceeded\":true}' \""+metaPrefix+"\"); fi",
		"cat \"$__judge_stderr\" 1>&2",
		"rm -f \"$__judge_stderr\"",
		"printf '\\n%s\\n' \"$__judge_meta\" 1>&2",
		"exit 0",
	)

	return strings.Join(parts, "; ")
}

func parseExecResult(stdout, stderr string) (string, string, execMetrics, error) {
	var metrics execMetrics

	lines := strings.Split(stderr, "\n")
	filtered := make([]string, 0, len(lines))
	foundMeta := false
	for _, line := range lines {
		if strings.HasPrefix(line, metaPrefix) {
			payload := strings.TrimPrefix(line, metaPrefix)
			if err := json.Unmarshal([]byte(payload), &metrics); err != nil {
				return "", "", execMetrics{}, fmt.Errorf("parse metrics: %w", err)
			}
			foundMeta = true
			continue
		}
		filtered = append(filtered, line)
	}

	if !foundMeta {
		snippet := strings.TrimSpace(stderr)
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}
		if snippet == "" {
			return "", "", execMetrics{}, errors.New("missing execution metadata")
		}
		return "", "", execMetrics{}, fmt.Errorf("missing execution metadata; stderr: %s", snippet)
	}

	cleanStderr := strings.Join(filtered, "\n")
	cleanStderr = strings.TrimSuffix(cleanStderr, "\n")
	if metrics.ExitCode == 0 && strings.TrimSpace(cleanStderr) != "" {
		logrus.Debugf("non-empty stderr with zero exit code: %s", cleanStderr)
	}

	return stdout, cleanStderr, metrics, nil
}

func buildGeneratedResult(result judgecore.RunResult) judgecore.TestResult {
	generated := judgecore.TestResult{
		Input:        result.Input,
		ActualOutput: result.ActualOutput,
		Stderr:       result.Stderr,
		TimeUsedSec:  result.TimeUsedSec,
		MemoryUsedKB: result.MemoryUsedKB,
		ExitCode:     result.ExitCode,
	}

	switch {
	case result.MemoryExceeded:
		generated.Status = judgecore.StatusMemoryLimitExceeded
	case result.TimedOut:
		generated.Status = judgecore.StatusTimeLimitExceeded
	case result.ExitCode != 0:
		generated.Status = judgecore.StatusRuntimeError
	default:
		generated.Status = judgecore.StatusAccepted
		generated.Passed = true
	}

	return generated
}

func runEphemeralContainer(
	ctx context.Context,
	cli *client.Client,
	image string,
	workDir string,
	command string,
	memLimitMb int32,
) (string, string, int32, float64, error) {
	start := time.Now()

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: image,
		Cmd:   []string{"/bin/sh", "-lc", command},
		Tty:   false,
	}, &container.HostConfig{
		Mounts: []mount.Mount{
			{Type: mount.TypeBind, Source: workDir, Target: "/workdir"},
		},
		Resources: container.Resources{
			Memory:   int64(memLimitMb) * 1024 * 1024,
			NanoCPUs: 1e9,
		},
	}, &network.NetworkingConfig{}, &ocispec.Platform{}, "")
	if err != nil {
		return "", "", 0, 0, fmt.Errorf("container create: %w", err)
	}
	defer removeContainer(cli, resp.ID)

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", "", 0, 0, fmt.Errorf("container start: %w", err)
	}

	statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return "", "", 0, 0, fmt.Errorf("container wait: %w", err)
		}
	case status := <-statusCh:
		out, logsErr := cli.ContainerLogs(ctx, resp.ID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
		if logsErr != nil {
			return "", "", 0, 0, fmt.Errorf("logs error: %w", logsErr)
		}
		defer out.Close()

		var stdout, stderr bytes.Buffer
		if _, err := stdcopy.StdCopy(&stdout, &stderr, out); err != nil {
			return "", "", 0, 0, fmt.Errorf("copy logs: %w", err)
		}

		return stdout.String(), stderr.String(), int32(status.StatusCode), time.Since(start).Seconds(), nil
	}

	return "", "", 0, 0, errors.New("container wait ended unexpectedly")
}

func removeContainer(cli *client.Client, containerID string) {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := cli.ContainerRemove(cleanupCtx, containerID, container.RemoveOptions{Force: true}); err != nil {
		logrus.Warnf("container cleanup failed for %s: %v", containerID, err)
	}
}

func shellJoin(args []string) string {
	escaped := make([]string, len(args))
	for i, arg := range args {
		escaped[i] = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
	}
	return strings.Join(escaped, " ")
}
