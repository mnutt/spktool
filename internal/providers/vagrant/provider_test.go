package vagrant

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

func TestBootstrapFilesIncludeVagrantfile(t *testing.T) {
	t.Parallel()

	provider := New(&captureRunner{}, templates.New())
	files, err := provider.BootstrapFiles(providers.ProjectContext{WorkDir: "/workspace/demo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("unexpected file count: %d", len(files))
	}
	if files[0].Path != filepath.Join(".sandstorm", "Vagrantfile") {
		t.Fatalf("unexpected path: %q", files[0].Path)
	}
	if !strings.Contains(string(files[0].Body), "Vagrant.configure") {
		t.Fatalf("expected Vagrantfile content, got: %s", string(files[0].Body))
	}
}

func TestExecUsesVagrantSSHInSandstormDir(t *testing.T) {
	t.Parallel()

	r := &captureRunner{}
	provider := New(r, templates.New())
	_, err := provider.Exec(context.Background(), providers.ProjectContext{WorkDir: "/workspace/demo"}, []string{"echo", "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if r.spec.Command != "vagrant" {
		t.Fatalf("unexpected command: %q", r.spec.Command)
	}
	if r.spec.Dir != filepath.Join("/workspace/demo", ".sandstorm") {
		t.Fatalf("unexpected dir: %q", r.spec.Dir)
	}
	got := strings.Join(r.spec.Args, " ")
	if got != "ssh -c 'echo' 'hello'" {
		t.Fatalf("unexpected args: %q", got)
	}
}
