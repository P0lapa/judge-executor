package executor

import (
	"context"
	"sync"

	"github.com/P0lapa/judge-executor/internal/pkg/sandbox"
	judge "github.com/P0lapa/judge-executor/proto"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"
)

type ExecutorServer struct {
	judge.UnimplementedExecutorServiceServer
	workerPool *semaphore.Weighted
	mu         sync.Mutex
}

func NewExecutorServer(maxWorkers int64) *ExecutorServer {
	return &ExecutorServer{
		workerPool: semaphore.NewWeighted(maxWorkers),
	}
}

func (s *ExecutorServer) Execute(ctx context.Context, req *judge.ExecuteRequest) (*judge.ExecuteResponse, error) {
	if err := s.workerPool.Acquire(ctx, 1); err != nil {
		return nil, err
	}
	defer s.workerPool.Release(1)

	logrus.Infof("Processing submission %s, lang: %s, batch size: %d", req.SubmissionId, req.Lang, len(req.Input))

	outputs, stderrs, timeUseds, memUseds, exitCodes, errMsg := sandbox.ExecuteInDockerParallel(ctx, req.Lang, req.Code, req.Input, req.TimeLimitMs, req.MemoryLimitMb, req.AdditionalArgs)

	// Детальное логирование возвращаемых значений
	logrus.Infof("Results from sandbox - outputs: %d, stderrs: %d, timeUseds: %d, memUseds: %d, exitCodes: %d",
		len(outputs), len(stderrs), len(timeUseds), len(memUseds), len(exitCodes))

	for i := range outputs {
		logrus.Infof("Result[%d]: time=%f, memory=%d, exit=%d", i, timeUseds[i], memUseds[i], exitCodes[i])
	}

	if errMsg != "" {
		return &judge.ExecuteResponse{ErrorMessage: errMsg}, nil
	}

	// Создаем response и логируем его
	response := &judge.ExecuteResponse{
		Output:       outputs,
		Stderr:       stderrs,
		TimeUsedSec:  timeUseds,
		MemoryUsedKb: memUseds,
		ExitCode:     exitCodes,
		ErrorMessage: "",
	}

	logrus.Infof("GRPC Response - TimeUsedSec: %v", response.TimeUsedSec)
	logrus.Infof("GRPC Response - MemoryUsedKb: %v", response.MemoryUsedKb)

	return response, nil
}
