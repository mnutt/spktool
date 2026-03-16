package contracttests

import (
	"context"
	"testing"

	"github.com/mnutt/spktool/internal/providers"
	"github.com/mnutt/spktool/internal/runner"
	"github.com/mnutt/spktool/internal/templates"
)

type CaptureRunner struct {
	Spec   runner.Spec
	Result runner.Result
	Err    error
}

func (r *CaptureRunner) Run(_ context.Context, spec runner.Spec) (runner.Result, error) {
	r.Spec = spec
	if r.Err != nil {
		return runner.Result{}, r.Err
	}
	if r.Result.Stdout == "" && r.Result.Stderr == "" && r.Result.ExitCode == 0 && r.Result.TraceID == "" && r.Result.Command == "" && r.Result.Duration == 0 {
		return runner.Result{Stdout: "ok"}, nil
	}
	return r.Result, nil
}

type CoreHarness struct {
	NewProvider     func(runner.Runner, *templates.Repository) providers.ProviderCore
	Project         providers.ProjectContext
	StatusStdout    string
	ExpectUp        func(*testing.T, runner.Spec)
	ExpectHalt      func(*testing.T, runner.Spec)
	ExpectDestroy   func(*testing.T, runner.Spec)
	ExpectProvision func(*testing.T, runner.Spec)
	ExpectStatus    func(*testing.T, providers.Status, runner.Spec)
}

func RunProviderCoreContract(t *testing.T, h CoreHarness) {
	t.Helper()

	repo := templates.New()
	r := &CaptureRunner{}
	provider := h.NewProvider(r, repo)

	if err := provider.Up(context.Background(), h.Project); err != nil {
		t.Fatalf("up failed: %v", err)
	}
	h.ExpectUp(t, r.Spec)

	if err := provider.Halt(context.Background(), h.Project); err != nil {
		t.Fatalf("halt failed: %v", err)
	}
	h.ExpectHalt(t, r.Spec)

	if err := provider.Destroy(context.Background(), h.Project); err != nil {
		t.Fatalf("destroy failed: %v", err)
	}
	h.ExpectDestroy(t, r.Spec)

	if err := provider.Provision(context.Background(), h.Project); err != nil {
		t.Fatalf("provision failed: %v", err)
	}
	h.ExpectProvision(t, r.Spec)

	r.Result = runner.Result{Stdout: h.StatusStdout}
	status, err := provider.Status(context.Background(), h.Project)
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if status.Provider != provider.Name() {
		t.Fatalf("status provider = %q want %q", status.Provider, provider.Name())
	}
	if status.InstanceName == "" {
		t.Fatal("expected non-empty instance name")
	}
	h.ExpectStatus(t, status, r.Spec)
}
