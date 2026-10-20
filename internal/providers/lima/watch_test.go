package lima

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mnutt/spktool/internal/providers"
	"github.com/mnutt/spktool/internal/runner"
	"github.com/mnutt/spktool/internal/templates"
)

func fakeWatchCLI(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "limactl"), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("LIMA_HOME", dir)
	t.Setenv("WATCH_CALLS", filepath.Join(dir, "calls"))
	return dir
}

func awaitWatchCall(t *testing.T, dir, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(filepath.Join(dir, "calls"))
		if strings.Contains(string(data), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("watch did not invoke %q", want)
}

const watchCLI = `if [ "$2" = "--help" ]; then
  echo probe >> "$WATCH_CALLS"
  exit 0
fi
echo 'forwarding tcp 127.0.0.1:6090 to 127.0.0.1:6090'
echo "$*" >> "$WATCH_CALLS"
exec /bin/sleep 30
`

func TestWatchUnavailable(t *testing.T) {
	dir := fakeWatchCLI(t, `echo "$*" >> "$WATCH_CALLS"; exit 1`)
	var output bytes.Buffer
	stop := startWatch(context.Background(), "demo", true, &output)
	stop()
	data, err := os.ReadFile(filepath.Join(dir, "calls"))
	if err != nil || string(data) != "watch --help\n" {
		t.Fatalf("expected only capability probe, got %q (%v)", data, err)
	}
	if output.Len() != 0 {
		t.Fatalf("unsupported watch should be silent: %s", output.String())
	}
}

func TestWatchExistingInstanceStreamsAndStops(t *testing.T) {
	dir := fakeWatchCLI(t, watchCLI)
	var output bytes.Buffer
	stop := startWatch(context.Background(), "demo", true, &output)
	t.Cleanup(stop)
	awaitWatchCall(t, dir, "watch --tty=false demo")
	stop()
	stop() // Cleanup is safe more than once.
	if !strings.Contains(output.String(), "forwarding tcp") {
		t.Fatalf("missing event output: %s", output.String())
	}
	if strings.Contains(output.String(), "watch stopped") {
		t.Fatalf("normal cancellation should be silent: %s", output.String())
	}
}

func TestWatchNewInstanceWaitsForLogAndReplaysHistory(t *testing.T) {
	dir := fakeWatchCLI(t, watchCLI)
	var output bytes.Buffer
	stop := startWatch(context.Background(), "demo", false, &output)
	t.Cleanup(stop)
	// Allow a polling interval to pass before the instance exists.
	time.Sleep(150 * time.Millisecond)
	data, _ := os.ReadFile(filepath.Join(dir, "calls"))
	if string(data) != "probe\n" {
		t.Fatalf("watch started before instance creation: %s", data)
	}
	if err := os.Mkdir(filepath.Join(dir, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "demo", "ha.stdout.log"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	awaitWatchCall(t, dir, "watch --tty=false --history demo")
	stop()
}

func TestWatchCancellationBeforeInstanceCreation(t *testing.T) {
	fakeWatchCLI(t, watchCLI)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := startWatch(ctx, "missing", false, &bytes.Buffer{})
	cancel()
	done := make(chan struct{})
	go func() { stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("watch did not stop while waiting for instance creation")
	}
}

func TestWatchFailureKeepsSerialDiagnostics(t *testing.T) {
	dir := fakeWatchCLI(t, `if [ "$2" = "--help" ]; then exit 0; fi
echo "$*" >> "$WATCH_CALLS"
exit 1
`)
	output := watchFailureOutput{failed: make(chan struct{})}
	stop := startWatch(context.Background(), "demo", true, &output)
	t.Cleanup(stop)
	awaitWatchCall(t, dir, "watch --tty=false demo")
	select {
	case <-output.failed:
	case <-time.After(3 * time.Second):
		t.Fatal("watch failure was not reported")
	}
	stop()
	if !strings.Contains(output.String(), "serial diagnostics remain active") {
		t.Fatalf("missing diagnostic fallback message: %s", output.String())
	}
}

type watchFailureOutput struct {
	bytes.Buffer
	failed chan struct{}
}

func (w *watchFailureOutput) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	if bytes.Contains(p, []byte("serial diagnostics remain active")) {
		close(w.failed)
	}
	return n, err
}

type watchRunnerFunc func(context.Context, runner.Spec) (runner.Result, error)

func (f watchRunnerFunc) Run(ctx context.Context, spec runner.Spec) (runner.Result, error) {
	return f(ctx, spec)
}

func TestVerboseUpPreservesStartFailure(t *testing.T) {
	dir := fakeWatchCLI(t, watchCLI)
	instance := New(nil, nil).DetectInstanceName("/workspace/demo")
	startErr := errors.New("VM failed to start")
	r := watchRunnerFunc(func(_ context.Context, spec runner.Spec) (runner.Result, error) {
		if spec.Name == "lima-list" {
			return runner.Result{Stdout: `{"name":"` + instance + `","status":"Stopped"}`}, nil
		}
		if spec.Name != "lima-start" {
			t.Fatalf("unexpected command: %s", spec.Name)
		}
		awaitWatchCall(t, dir, "watch --tty=false "+instance)
		return runner.Result{}, startErr
	})
	p := New(r, templates.New())
	if err := p.Up(context.Background(), providers.ProjectContext{WorkDir: "/workspace/demo", Verbose: true}); !errors.Is(err, startErr) {
		t.Fatalf("startup error lost: %v", err)
	}
}
