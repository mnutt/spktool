package lima

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mnutt/spktool/internal/providers"
	"github.com/mnutt/spktool/internal/runner"
	"github.com/mnutt/spktool/internal/templates"
)

type captureRunner struct {
	spec runner.Spec
}

func (r *captureRunner) Run(_ context.Context, spec runner.Spec) (runner.Result, error) {
	r.spec = spec
	return runner.Result{Stdout: "ok"}, nil
}

func TestDetectInstanceNameSanitizesWorkspacePath(t *testing.T) {
	t.Parallel()

	provider := New(&captureRunner{}, templates.New())
	got := provider.DetectInstanceName("/tmp/My App@123")
	if !strings.HasPrefix(got, "sandstorm-My-App-123-") {
		t.Fatalf("unexpected instance name: %q", got)
	}
}

func TestBootstrapFilesIncludeWorkdirMount(t *testing.T) {
	t.Parallel()

	provider := New(&captureRunner{}, templates.New())
	files, err := provider.BootstrapFiles(providers.ProjectContext{WorkDir: "/workspace/demo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("unexpected file count: %d", len(files))
	}
	if files[0].Path != filepath.Join(".sandstorm", "lima.yaml") {
		t.Fatalf("unexpected path: %q", files[0].Path)
	}
	if !strings.Contains(string(files[0].Body), `location: "/workspace/demo"`) {
		t.Fatalf("expected workdir mount in lima.yaml: %s", string(files[0].Body))
	}
}

func TestExecUsesLimactlShellWithWorkdir(t *testing.T) {
	t.Parallel()

	r := &captureRunner{}
	provider := New(r, templates.New())
	_, err := provider.Exec(context.Background(), providers.ProjectContext{WorkDir: "/workspace/demo"}, []string{"echo", "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if r.spec.Command != "limactl" {
		t.Fatalf("unexpected command: %q", r.spec.Command)
	}
	got := strings.Join(r.spec.Args, " ")
	if !strings.Contains(got, "shell --workdir /opt/app") {
		t.Fatalf("unexpected args: %q", got)
	}
	if !strings.Contains(got, "bash -lc 'echo' 'hello'") {
		t.Fatalf("unexpected shell args: %q", got)
	}
}
