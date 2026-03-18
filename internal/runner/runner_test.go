package runner

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestRunLogsCapturedOutputOnFailure(t *testing.T) {
	t.Parallel()

	var logOutput bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug}))
	r := New(logger)

	_, err := r.Run(context.Background(), Spec{
		Name:    "failing-command",
		Command: "sh",
		Args:    []string{"-c", "echo guest-stdout && echo guest-stderr 1>&2 && exit 1"},
	})
	if err == nil {
		t.Fatal("expected command failure")
	}

	logText := logOutput.String()
	if !strings.Contains(logText, "stderr=guest-stderr") {
		t.Fatalf("expected stderr in log output, got %q", logText)
	}
	if !strings.Contains(logText, "stdout=guest-stdout") {
		t.Fatalf("expected stdout in log output, got %q", logText)
	}
}
