package judge

import "strings"

type ComparisonMode string

const (
	ComparisonModeExact                ComparisonMode = "EXACT"
	ComparisonModeTrimmedExact         ComparisonMode = "TRIMMED_EXACT"
	ComparisonModeIgnoreTrailingSpaces ComparisonMode = "IGNORE_TRAILING_SPACES"
)

type Status string

const (
	StatusAccepted            Status = "AC"
	StatusWrongAnswer         Status = "WA"
	StatusRuntimeError        Status = "RE"
	StatusTimeLimitExceeded   Status = "TLE"
	StatusMemoryLimitExceeded Status = "MLE"
	StatusSystemError         Status = "SE"
)

type RunResult struct {
	Input          string
	ExpectedOutput string
	ActualOutput   string
	Stderr         string
	TimeUsedSec    float64
	MemoryUsedKB   int32
	ExitCode       int32
	TimedOut       bool
	MemoryExceeded bool
}

type TestResult struct {
	Input          string
	ExpectedOutput string
	ActualOutput   string
	Stderr         string
	TimeUsedSec    float64
	MemoryUsedKB   int32
	ExitCode       int32
	Passed         bool
	Status         Status
}

func CompareOutput(mode ComparisonMode, actual, expected string) bool {
	switch mode {
	case ComparisonModeExact:
		return actual == expected
	case ComparisonModeIgnoreTrailingSpaces:
		return normalizeTrailingSpaces(actual) == normalizeTrailingSpaces(expected)
	case ComparisonModeTrimmedExact:
		fallthrough
	default:
		return strings.TrimSpace(actual) == strings.TrimSpace(expected)
	}
}

func BuildTestResult(result RunResult, mode ComparisonMode) TestResult {
	verdict := TestResult{
		Input:          result.Input,
		ExpectedOutput: result.ExpectedOutput,
		ActualOutput:   result.ActualOutput,
		Stderr:         result.Stderr,
		TimeUsedSec:    result.TimeUsedSec,
		MemoryUsedKB:   result.MemoryUsedKB,
		ExitCode:       result.ExitCode,
		Status:         StatusSystemError,
	}

	switch {
	case result.MemoryExceeded:
		verdict.Status = StatusMemoryLimitExceeded
	case result.TimedOut:
		verdict.Status = StatusTimeLimitExceeded
	case result.ExitCode != 0:
		verdict.Status = StatusRuntimeError
	case CompareOutput(mode, result.ActualOutput, result.ExpectedOutput):
		verdict.Status = StatusAccepted
		verdict.Passed = true
	default:
		verdict.Status = StatusWrongAnswer
	}

	return verdict
}

func normalizeTrailingSpaces(value string) string {
	lines := strings.Split(value, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t\r")
	}
	return strings.Join(lines, "\n")
}
