package main

import (
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/P0lapa/judge-executor/internal/pkg/executor"
	judge "github.com/P0lapa/judge-executor/proto"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

func main() {
	logrus.SetLevel(logrus.InfoLevel)

	lis, err := net.Listen("tcp", ":50051") // Port для gRPC
	if err != nil {
		logrus.Fatalf("listen: %v", err)
	}

	s := grpc.NewServer()
	judge.RegisterExecutorServiceServer(s, executor.NewExecutorServer(10)) // 10 workers

	go func() {
		if err := s.Serve(lis); err != nil {
			logrus.Fatalf("serve: %v", err)
		}
	}()
	logrus.Info("gRPC server started on :50051")

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	s.GracefulStop()
}
