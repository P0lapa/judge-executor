package executor

import (
	"context"
	"testing"

	judgecore "github.com/P0lapa/judge-executor/internal/pkg/judge"
	pb "github.com/P0lapa/judge-executor/proto"
)

func TestExecuteMapsJudgeResults(t *testing.T) {
	server := NewExecutorServer(1)
	server.judgeFunc = func(_ context.Context, lang string, code string, testCases []judgecore.RunResult, timeLimitMs int32, memLimitMb int32, additionalArgs []string, comparison judgecore.ComparisonMode) ([]judgecore.TestResult, string) {
		if lang != "python" || code == "" || len(testCases) != 1 || comparison != judgecore.ComparisonModeTrimmedExact {
			t.Fatalf("unexpected arguments: lang=%s code=%q cases=%d comparison=%s", lang, code, len(testCases), comparison)
		}
		return []judgecore.TestResult{
			{
				Input:          "2",
				ExpectedOutput: "4",
				ActualOutput:   "4\n",
				Passed:         true,
				Status:         judgecore.StatusAccepted,
				TimeUsedSec:    0.01,
				MemoryUsedKB:   128,
				ExitCode:       0,
			},
		}, ""
	}

	resp, err := server.Execute(context.Background(), &pb.ExecuteRequest{
		Code:           "print(4)",
		Lang:           "python",
		TimeLimitMs:    1000,
		MemoryLimitMb:  64,
		ComparisonMode: pb.OutputComparisonMode_OUTPUT_COMPARISON_MODE_TRIMMED_EXACT,
		TestCases: []*pb.TestCase{
			{Input: "2", ExpectedOutput: "4"},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("len(resp.Results) = %d", len(resp.Results))
	}
	if !resp.Results[0].Passed || resp.Results[0].Status != pb.TestStatus_TEST_STATUS_AC {
		t.Fatalf("unexpected result: %+v", resp.Results[0])
	}
}

func TestGenerateOutputsMapsSandboxResults(t *testing.T) {
	server := NewExecutorServer(1)
	server.generateFunc = func(_ context.Context, lang string, code string, inputs []string, timeLimitMs int32, memLimitMb int32, additionalArgs []string) ([]judgecore.RunResult, string) {
		if lang != "python" || code == "" || len(inputs) != 1 || inputs[0] != "2" {
			t.Fatalf("unexpected arguments: lang=%s code=%q inputs=%v", lang, code, inputs)
		}
		return []judgecore.RunResult{
			{
				Input:        "2",
				ActualOutput: "4\n",
				Stderr:       "",
				TimeUsedSec:  0.02,
				MemoryUsedKB: 256,
				ExitCode:     0,
			},
		}, ""
	}

	resp, err := server.GenerateOutputs(context.Background(), &pb.GenerateOutputsRequest{
		Code:          "print(4)",
		Lang:          "python",
		TimeLimitMs:   1000,
		MemoryLimitMb: 64,
		TestCases: []*pb.InputCase{
			{Input: "2"},
		},
	})
	if err != nil {
		t.Fatalf("GenerateOutputs() error = %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("len(resp.Results) = %d", len(resp.Results))
	}
	if resp.Results[0].Status != pb.TestStatus_TEST_STATUS_AC || resp.Results[0].Output != "4\n" {
		t.Fatalf("unexpected result: %+v", resp.Results[0])
	}
}
