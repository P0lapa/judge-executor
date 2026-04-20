# Judge Batch Validation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace raw execution responses with batch judging that validates actual output against expected output for each test case in a single container run.

**Architecture:** The gRPC contract will move from `input[] -> output[]` to `test_cases[] -> verdict[]`. The sandbox will compile once, prepare a single workdir with all test inputs plus a runner script, execute one container that enforces `ulimit`/timeout per test, write per-test artifacts to JSON, and let Go validate outputs using a configurable comparison mode.

**Tech Stack:** Go, gRPC/protobuf, Docker API, shell runner scripts, Go tests.

---

### Task 1: Contract and comparison behavior

**Files:**
- Modify: `proto/judge.proto`
- Create: `internal/pkg/judge/compare.go`
- Create: `internal/pkg/judge/compare_test.go`

- [ ] Write a failing test for configurable output comparison and verdict statuses.
- [ ] Run the comparison tests and verify they fail for missing symbols.
- [ ] Implement minimal comparison helpers and statuses.
- [ ] Re-run comparison tests and verify they pass.

### Task 2: Batch sandbox execution

**Files:**
- Modify: `internal/pkg/sandbox/docker.go`
- Create: `internal/pkg/sandbox/docker_test.go`

- [ ] Write failing tests for request validation and result decoding around batch judging.
- [ ] Run the sandbox-focused tests and verify they fail for missing judge flow.
- [ ] Implement compile-once plus single-container execution with runner/result JSON parsing.
- [ ] Re-run sandbox tests and verify they pass.

### Task 3: gRPC integration and generated protobufs

**Files:**
- Modify: `internal/pkg/executor/executor.go`
- Modify: `proto/judge.pb.go`
- Modify: `proto/judge_grpc.pb.go`
- Modify: `request.json`
- Modify: `proto/request.json`

- [ ] Add a failing integration-style test for mapping sandbox judge results into the gRPC response.
- [ ] Run targeted tests and verify they fail.
- [ ] Update the executor and regenerate protobuf bindings from the new contract.
- [ ] Re-run targeted tests and verify they pass.

### Task 4: Verification

**Files:**
- Verify only

- [ ] Run `go test ./...` with `GOCACHE` inside the workspace.
- [ ] Run `go test` for the modified packages individually if the full suite exposes unrelated issues.
- [ ] Summarize any remaining risk around Docker-dependent runtime verification.
