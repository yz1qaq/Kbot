package sandbox

import (
	"context"
	"errors"
	"testing"
)

func TestLimitedBufferTruncatesOutput(t *testing.T) {
	buffer := newLimitedBuffer(5)

	steps := []struct {
		input         string
		wantOutput    string
		wantTruncated bool
	}{
		{
			input:         "ABC",
			wantOutput:    "ABC",
			wantTruncated: false,
		},
		{
			input:         "DEFG",
			wantOutput:    "ABCDE",
			wantTruncated: true,
		},
		{
			input:         "HI",
			wantOutput:    "ABCDE",
			wantTruncated: true,
		},
	}

	for _, step := range steps {
		n, err := buffer.Write([]byte(step.input))
		if err != nil {
			t.Fatalf("Write(%q): %v", step.input, err)
		}

		if n != len(step.input) {
			t.Fatalf(
				"Write(%q) returned %d, want %d",
				step.input, n, len(step.input),
			)
		}

		if got := buffer.String(); got != step.wantOutput {
			t.Fatalf(
				"after Write(%q): output = %q, want %q",
				step.input, got, step.wantOutput,
			)
		}

		if buffer.truncated != step.wantTruncated {
			t.Fatalf(
				"after Write(%q): truncated = %v, want %v",
				step.input, buffer.truncated, step.wantTruncated,
			)
		}
	}
}

func TestExecuteRejectsWhenAtCapacity(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxConcurrent = 1

	runner := &Sandbox{
		cfg:        cfg,
		dockerPath: "docker",
		semaphore:  make(chan struct{}, cfg.MaxConcurrent),
	}

	// 模拟已有一个任务占用了唯一的执行名额。
	runner.semaphore <- struct{}{}

	_, err := runner.Execute(
		context.Background(),
		"python",
		"print(42)",
	)

	if !errors.Is(err, ErrCapacity) {
		t.Fatalf("Execute() error = %v, want ErrCapacity", err)
	}

	// 被拒绝的任务不能释放其他任务占用的名额。
	if got := len(runner.semaphore); got != 1 {
		t.Fatalf("occupied slots = %d, want 1", got)
	}
}
