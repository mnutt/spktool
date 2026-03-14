package vagrant

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mnutt/spktool/internal/config"
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
	files, err := provider.BootstrapFiles(providers.ProjectContext{
		WorkDir: "/workspace/demo",
		Config: &config.Resolved{
			Vagrant: config.VagrantResolved{Box: "debian/bookworm64"},
			Network: config.NetworkResolved{Sandstorm: config.SandstormResolved{
				Host:          "local.sandstorm.io",
				GuestPort:     6090,
				ExternalPort:  6020,
				LocalhostOnly: true,
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("unexpected file count: %d", len(files))
	}
	if files[0].Path != filepath.Join(".sandstorm", ".generated", "Vagrantfile") {
		t.Fatalf("unexpected path: %q", files[0].Path)
	}
	if !strings.Contains(string(files[0].Body), "Vagrant.configure") {
		t.Fatalf("expected Vagrantfile content, got: %s", string(files[0].Body))
	}
	if !strings.Contains(string(files[0].Body), "guest: 6090, host: 6020") {
		t.Fatalf("expected configured port forwarding, got: %s", string(files[0].Body))
	}
	if !strings.Contains(string(files[0].Body), `host_ip: "127.0.0.1"`) {
		t.Fatalf("expected localhost-only forwarding, got: %s", string(files[0].Body))
	}
	if !strings.Contains(string(files[0].Body), `override.vm.synced_folder "../..", "/opt/app"`) {
		t.Fatalf("expected project root mount mapping, got: %s", string(files[0].Body))
	}
	if !strings.Contains(string(files[0].Body), `override.vm.synced_folder ENV["HOME"] + "/.sandstorm", "/host-dot-sandstorm"`) {
		t.Fatalf("expected host sandstorm mount mapping, got: %s", string(files[0].Body))
	}
}

func TestBootstrapFilesOmitLocalhostOnlyHostIPWhenDisabled(t *testing.T) {
	t.Parallel()

	provider := New(&captureRunner{}, templates.New())
	files, err := provider.BootstrapFiles(providers.ProjectContext{
		WorkDir: "/workspace/demo",
		Config: &config.Resolved{
			Vagrant: config.VagrantResolved{Box: "debian/bookworm64"},
			Network: config.NetworkResolved{Sandstorm: config.SandstormResolved{
				Host:          "demo.local",
				GuestPort:     6090,
				ExternalPort:  6020,
				LocalhostOnly: false,
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(files[0].Body), `host_ip: "127.0.0.1"`) {
		t.Fatalf("did not expect host_ip restriction, got: %s", string(files[0].Body))
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
	if r.spec.Dir != filepath.Join("/workspace/demo", ".sandstorm", ".generated") {
		t.Fatalf("unexpected dir: %q", r.spec.Dir)
	}
	got := strings.Join(r.spec.Args, " ")
	if got != "ssh -c 'echo' 'hello'" {
		t.Fatalf("unexpected args: %q", got)
	}
}

func TestUpRunsVagrantInGeneratedDirWithStreaming(t *testing.T) {
	t.Parallel()

	r := &captureRunner{}
	provider := New(r, templates.New())
	err := provider.Up(context.Background(), providers.ProjectContext{WorkDir: "/workspace/demo"})
	if err != nil {
		t.Fatal(err)
	}
	if r.spec.Command != "vagrant" {
		t.Fatalf("unexpected command: %q", r.spec.Command)
	}
	if r.spec.Dir != filepath.Join("/workspace/demo", ".sandstorm", ".generated") {
		t.Fatalf("unexpected dir: %q", r.spec.Dir)
	}
	if got := strings.Join(r.spec.Args, " "); got != "up" {
		t.Fatalf("unexpected args: %q", got)
	}
	if !r.spec.Stream {
		t.Fatal("expected streamed output for vagrant up")
	}
}

func TestProvisionRunsVagrantProvisionInGeneratedDirWithStreaming(t *testing.T) {
	t.Parallel()

	r := &captureRunner{}
	provider := New(r, templates.New())
	err := provider.Provision(context.Background(), providers.ProjectContext{WorkDir: "/workspace/demo"})
	if err != nil {
		t.Fatal(err)
	}
	if r.spec.Command != "vagrant" {
		t.Fatalf("unexpected command: %q", r.spec.Command)
	}
	if r.spec.Dir != filepath.Join("/workspace/demo", ".sandstorm", ".generated") {
		t.Fatalf("unexpected dir: %q", r.spec.Dir)
	}
	if got := strings.Join(r.spec.Args, " "); got != "provision" {
		t.Fatalf("unexpected args: %q", got)
	}
	if !r.spec.Stream {
		t.Fatal("expected streamed output for vagrant provision")
	}
}
