package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	judgecore "github.com/P0lapa/judge-executor/internal/pkg/judge"
	"github.com/P0lapa/judge-executor/internal/pkg/languages"
)

func TestParseExecResult(t *testing.T) {
	stdout, stderr, metrics, err := parseExecResult(
		"user output\n",
		"warning line\n__JUDGE_META__:{\"time_used_sec\":0.12,\"memory_used_kb\":321,\"exit_code\":0,\"timed_out\":false,\"memory_exceeded\":false}\n",
	)
	if err != nil {
		t.Fatalf("parseExecResult() error = %v", err)
	}

	if stdout != "user output\n" {
		t.Fatalf("stdout = %q", stdout)
	}
	if strings.Contains(stderr, "__JUDGE_META__") {
		t.Fatalf("stderr still contains metadata: %q", stderr)
	}
	if metrics.ExitCode != 0 || metrics.MemoryUsedKB != 321 {
		t.Fatalf("unexpected metrics: %+v", metrics)
	}
}

func TestParseExecResultIncludesStderrWhenMetadataMissing(t *testing.T) {
	_, _, _, err := parseExecResult("", "/bin/sh: /usr/bin/time: not found\n")
	if err == nil {
		t.Fatal("parseExecResult() expected error")
	}
	if !strings.Contains(err.Error(), "/usr/bin/time") {
		t.Fatalf("expected stderr snippet in error, got %v", err)
	}
}

func TestBuildExecCommandUsesTimeFallback(t *testing.T) {
	command := buildExecCommand(&languages.LanguageConfig{RunCmd: []string{"python3", "code.py"}}, 1000, 64)
	if !strings.Contains(command, "command -v /usr/bin/time") {
		t.Fatalf("buildExecCommand() does not check for time availability: %s", command)
	}
	if !strings.Contains(command, "date +%s%N") {
		t.Fatalf("buildExecCommand() does not include fallback timing path: %s", command)
	}
}

func TestBuildExecCommandSkipsVirtualMemoryLimitForJvm(t *testing.T) {
	command := buildExecCommand(&languages.LanguageConfig{
		RunCmd: []string{"java", "-Xmx128m", "-cp", ".", "Main"},
		IsJVM:  true,
	}, 1000, 256)

	if strings.Contains(command, "ulimit -v") {
		t.Fatalf("buildExecCommand() should skip ulimit -v for JVM: %s", command)
	}
}

func TestBuildExecCommandUsesVirtualMemoryLimitForNonJvm(t *testing.T) {
	command := buildExecCommand(&languages.LanguageConfig{
		RunCmd: []string{"python3", "code.py"},
	}, 1000, 256)

	if !strings.Contains(command, "ulimit -v 262144") {
		t.Fatalf("buildExecCommand() should keep ulimit -v for non-JVM: %s", command)
	}
}

func TestValidateRunCases(t *testing.T) {
	err := validateRunCases([]judgecore.RunResult{
		{Input: strings.Repeat("x", maxInputSize+1), ExpectedOutput: "ok"},
	})
	if err == nil {
		t.Fatal("validateRunCases() expected error for oversized input")
	}
}

func TestBuildGeneratedResultStatuses(t *testing.T) {
	tests := []struct {
		name   string
		input  judgecore.RunResult
		status judgecore.Status
	}{
		{
			name: "accepted result for successful execution",
			input: judgecore.RunResult{
				Input:        "1",
				ActualOutput: "ok",
				ExitCode:     0,
			},
			status: judgecore.StatusAccepted,
		},
		{
			name: "runtime error result",
			input: judgecore.RunResult{
				Input:        "1",
				ActualOutput: "boom",
				ExitCode:     1,
			},
			status: judgecore.StatusRuntimeError,
		},
		{
			name: "time limit result",
			input: judgecore.RunResult{
				Input:    "1",
				ExitCode: 124,
				TimedOut: true,
			},
			status: judgecore.StatusTimeLimitExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildGeneratedResult(tt.input)
			if got.Status != tt.status {
				t.Fatalf("buildGeneratedResult().Status = %q, want %q", got.Status, tt.status)
			}
		})
	}
}

func TestGetWorkdirRootsUsesEnvironment(t *testing.T) {
	t.Setenv("JUDGE_WORKDIR_CONTAINER", "/judge-workdir")
	t.Setenv("JUDGE_WORKDIR_HOST", "C:/host/judge-workdir")

	containerRoot, hostRoot := getWorkdirRoots()
	if containerRoot != "/judge-workdir" {
		t.Fatalf("containerRoot = %q", containerRoot)
	}
	if hostRoot != "C:/host/judge-workdir" {
		t.Fatalf("hostRoot = %q", hostRoot)
	}
}

func TestGetWorkdirRootsFallsBackToContainerRoot(t *testing.T) {
	t.Setenv("JUDGE_WORKDIR_CONTAINER", "/judge-workdir")
	t.Setenv("JUDGE_WORKDIR_HOST", "")

	containerRoot, hostRoot := getWorkdirRoots()
	if containerRoot != "/judge-workdir" || hostRoot != "/judge-workdir" {
		t.Fatalf("roots = %q %q", containerRoot, hostRoot)
	}
}

func TestPrepareWorkdirMapsContainerDirToHostDir(t *testing.T) {
	containerRoot := t.TempDir()
	hostRoot := filepath.Join("C:", "judge-workdir")

	containerDir, hostDir, err := prepareWorkdir(containerRoot, hostRoot)
	if err != nil {
		t.Fatalf("prepareWorkdir() error = %v", err)
	}
	defer os.RemoveAll(containerDir)

	if filepath.Dir(containerDir) != containerRoot {
		t.Fatalf("containerDir = %q, want inside %q", containerDir, containerRoot)
	}
	wantHostDir := filepath.ToSlash(filepath.Join(hostRoot, filepath.Base(containerDir)))
	if hostDir != wantHostDir {
		t.Fatalf("hostDir = %q, want %q", hostDir, wantHostDir)
	}
}
