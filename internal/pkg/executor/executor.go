package executor

import (
	"context"

	judgecore "github.com/P0lapa/judge-executor/internal/pkg/judge"
	"github.com/P0lapa/judge-executor/internal/pkg/sandbox"
	pb "github.com/P0lapa/judge-executor/proto"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"
)

type judgeFuncType func(context.Context, string, string, []judgecore.RunResult, int32, int32, []string, judgecore.ComparisonMode) ([]judgecore.TestResult, string)

type ExecutorServer struct {
	pb.UnimplementedExecutorServiceServer
	workerPool *semaphore.Weighted
	judgeFunc  judgeFuncType
}

func NewExecutorServer(maxWorkers int64) *ExecutorServer {
	return &ExecutorServer{
		workerPool: semaphore.NewWeighted(maxWorkers),
		judgeFunc:  sandbox.JudgeInDocker,
	}
}

func (s *ExecutorServer) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
	if err := s.workerPool.Acquire(ctx, 1); err != nil {
		return nil, err
	}
	defer s.workerPool.Release(1)

	logrus.Infof("Processing submission %s, lang: %s, batch size: %d", req.SubmissionId, req.Lang, len(req.TestCases))

	testCases := make([]judgecore.RunResult, 0, len(req.TestCases))
	for _, testCase := range req.TestCases {
		testCases = append(testCases, judgecore.RunResult{
			Input:          testCase.Input,
			ExpectedOutput: testCase.ExpectedOutput,
		})
	}

	results, errMsg := s.judgeFunc(
		ctx,
		req.Lang,
		req.Code,
		testCases,
		req.TimeLimitMs,
		req.MemoryLimitMb,
		req.AdditionalArgs,
		mapComparisonMode(req.ComparisonMode),
	)
	if errMsg != "" {
		return &pb.ExecuteResponse{ErrorMessage: errMsg}, nil
	}

	response := &pb.ExecuteResponse{
		Results: make([]*pb.TestResult, 0, len(results)),
	}
	for _, result := range results {
		response.Results = append(response.Results, &pb.TestResult{
			Passed:         result.Passed,
			Status:         mapStatus(result.Status),
			Input:          result.Input,
			ExpectedOutput: result.ExpectedOutput,
			ActualOutput:   result.ActualOutput,
			Stderr:         result.Stderr,
			TimeUsedSec:    result.TimeUsedSec,
			MemoryUsedKb:   result.MemoryUsedKB,
			ExitCode:       result.ExitCode,
		})
	}

	return response, nil
}

func mapComparisonMode(mode pb.OutputComparisonMode) judgecore.ComparisonMode {
	switch mode {
	case pb.OutputComparisonMode_OUTPUT_COMPARISON_MODE_EXACT:
		return judgecore.ComparisonModeExact
	case pb.OutputComparisonMode_OUTPUT_COMPARISON_MODE_IGNORE_TRAILING_SPACES:
		return judgecore.ComparisonModeIgnoreTrailingSpaces
	case pb.OutputComparisonMode_OUTPUT_COMPARISON_MODE_TRIMMED_EXACT:
		fallthrough
	default:
		return judgecore.ComparisonModeTrimmedExact
	}
}

func mapStatus(status judgecore.Status) pb.TestStatus {
	switch status {
	case judgecore.StatusAccepted:
		return pb.TestStatus_TEST_STATUS_AC
	case judgecore.StatusWrongAnswer:
		return pb.TestStatus_TEST_STATUS_WA
	case judgecore.StatusRuntimeError:
		return pb.TestStatus_TEST_STATUS_RE
	case judgecore.StatusTimeLimitExceeded:
		return pb.TestStatus_TEST_STATUS_TLE
	case judgecore.StatusMemoryLimitExceeded:
		return pb.TestStatus_TEST_STATUS_MLE
	default:
		return pb.TestStatus_TEST_STATUS_SE
	}
}
