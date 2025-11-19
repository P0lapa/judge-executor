package sandbox

import (
	"context"
	"strings"
	"testing"
)

func TestExecuteInDocker_Python(t *testing.T) {
	ctx := context.Background()
	output, stderr, timeUsed, memUsed, exitCode, errMsg := ExecuteInDocker(ctx, "python", "print('Hello')", "", 1000, 256, nil)
	if errMsg != "" || exitCode != 0 || !strings.Contains(output, "Hello") {
		t.Errorf("Failed: output=%s, err=%s", output, stderr)
	}
}
