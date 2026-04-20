package judge

import "testing"

func TestCompareOutputStrategies(t *testing.T) {
	tests := []struct {
		name     string
		mode     ComparisonMode
		actual   string
		expected string
		want     bool
	}{
		{
			name:     "exact match preserves whitespace sensitivity",
			mode:     ComparisonModeExact,
			actual:   "42\n",
			expected: "42",
			want:     false,
		},
		{
			name:     "trimmed exact ignores outer whitespace",
			mode:     ComparisonModeTrimmedExact,
			actual:   " 42\n",
			expected: "42",
			want:     true,
		},
		{
			name:     "ignore trailing spaces compares line by line",
			mode:     ComparisonModeIgnoreTrailingSpaces,
			actual:   "a  \n b\t\n",
			expected: "a\n b\n",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompareOutput(tt.mode, tt.actual, tt.expected)
			if got != tt.want {
				t.Fatalf("CompareOutput() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildTestResultStatus(t *testing.T) {
	tests := []struct {
		name   string
		result RunResult
		mode   ComparisonMode
		want   Status
	}{
		{
			name: "accepted result",
			result: RunResult{
				ExpectedOutput: "4",
				ActualOutput:   "4\n",
				ExitCode:       0,
			},
			mode: ComparisonModeTrimmedExact,
			want: StatusAccepted,
		},
		{
			name: "wrong answer result",
			result: RunResult{
				ExpectedOutput: "4",
				ActualOutput:   "5",
				ExitCode:       0,
			},
			mode: ComparisonModeTrimmedExact,
			want: StatusWrongAnswer,
		},
		{
			name: "runtime error result",
			result: RunResult{
				ExpectedOutput: "4",
				ActualOutput:   "4",
				ExitCode:       1,
			},
			mode: ComparisonModeTrimmedExact,
			want: StatusRuntimeError,
		},
		{
			name: "time limit result",
			result: RunResult{
				ExpectedOutput: "4",
				ActualOutput:   "",
				ExitCode:       124,
				TimedOut:       true,
			},
			mode: ComparisonModeTrimmedExact,
			want: StatusTimeLimitExceeded,
		},
		{
			name: "memory limit result",
			result: RunResult{
				ExpectedOutput: "4",
				ActualOutput:   "",
				ExitCode:       137,
				MemoryExceeded: true,
			},
			mode: ComparisonModeTrimmedExact,
			want: StatusMemoryLimitExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildTestResult(tt.result, tt.mode)
			if got.Status != tt.want {
				t.Fatalf("BuildTestResult().Status = %q, want %q", got.Status, tt.want)
			}
		})
	}
}
