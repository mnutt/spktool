package runner

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/mnutt/spktool/internal/domain"
	"github.com/mnutt/spktool/internal/logging"
)

type Spec struct {
	Name        string
	Command     string
	Args        []string
	Dir         string
	Env         []string
	Stdin       []byte
	Timeout     time.Duration
	Retries     int
	Redactions  []string
	Interactive bool
	Stream      bool
}

type Result struct {
	TraceID  string        `json:"traceId"`
	Command  string        `json:"command"`
	Stdout   string        `json:"stdout,omitempty"`
	Stderr   string        `json:"stderr,omitempty"`
	ExitCode int           `json:"exitCode"`
	Duration time.Duration `json:"duration"`
}

func (r Result) GetStdout() string { return r.Stdout }

type Runner interface {
	Run(ctx context.Context, spec Spec) (Result, error)
}

type CommandError struct {
	Result Result
	Err    error
}

func (e *CommandError) Error() string {
	base := fmt.Sprintf("command %q failed", e.Result.Command)
	if e.Result.ExitCode != 0 {
		base = fmt.Sprintf("%s with exit code %d", base, e.Result.ExitCode)
	}

	stderr := strings.TrimSpace(e.Result.Stderr)
	stdout := strings.TrimSpace(e.Result.Stdout)
	switch {
	case stderr != "" && stdout != "":
		return fmt.Sprintf("%s\nstderr:\n%s\nstdout:\n%s", base, stderr, stdout)
	case stderr != "":
		return fmt.Sprintf("%s\nstderr:\n%s", base, stderr)
	case stdout != "":
		return fmt.Sprintf("%s\nstdout:\n%s", base, stdout)
	case e.Err != nil:
		return fmt.Sprintf("%s: %v", base, e.Err)
	default:
		return base
	}
}

func (e *CommandError) Unwrap() error { return e.Err }

type ExecRunner struct {
	logger *slog.Logger
}

func New(logger *slog.Logger) *ExecRunner {
	return &ExecRunner{logger: logger}
}

func (r *ExecRunner) Run(ctx context.Context, spec Spec) (Result, error) {
	traceID := newTraceID()
	log := logging.WithTrace(ctx, r.logger, traceID)
	attempts := spec.Retries + 1
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	var lastResult Result
	for attempt := 1; attempt <= attempts; attempt++ {
		result, err := r.runOnce(ctx, spec, traceID)
		lastResult = result
		if err == nil {
			log.Debug("command completed", "name", spec.Name, "command", redact(result.Command, spec.Redactions), "duration", result.Duration)
			return result, nil
		}
		lastErr = err
		log.Warn("command attempt failed", "name", spec.Name, "attempt", attempt, "command", redact(result.Command, spec.Redactions), "error", err)
	}

	return lastResult, &domain.Error{
		Code:      domain.ErrExternal,
		Op:        "runner.Run",
		Message:   "external command failed",
		Retryable: spec.Retries > 0,
		Cause:     &CommandError{Result: lastResult, Err: lastErr},
	}
}

func (r *ExecRunner) runOnce(ctx context.Context, spec Spec, traceID string) (Result, error) {
	start := time.Now()
	if spec.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, spec.Command, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = append(cmd.Env, spec.Env...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if spec.Interactive {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else if spec.Stream {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	}
	if len(spec.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(spec.Stdin)
	}

	err := cmd.Run()
	result := Result{
		TraceID:  traceID,
		Command:  strings.TrimSpace(strings.Join(append([]string{spec.Command}, spec.Args...), " ")),
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: time.Since(start),
	}

	if err == nil {
		return result, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result.ExitCode = -1
	}
	return result, err
}

func redact(input string, redactions []string) string {
	out := input
	for _, item := range redactions {
		if item == "" {
			continue
		}
		out = strings.ReplaceAll(out, item, "[REDACTED]")
	}
	return out
}

func newTraceID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("trace-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
