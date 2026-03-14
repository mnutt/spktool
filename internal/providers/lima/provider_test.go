package lima

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
	files, err := provider.BootstrapFiles(providers.ProjectContext{
		WorkDir: "/workspace/demo",
		Config: &config.Resolved{
			Lima: config.LimaResolved{
				VMType:    "qemu",
				Arch:      "x86_64",
				Image:     "https://example.test/debian-amd64.qcow2",
				ImageArch: "x86_64",
			},
			Network: config.NetworkResolved{Sandstorm: config.SandstormResolved{
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
	if files[0].Path != filepath.Join(".sandstorm", ".generated", "lima.yaml") {
		t.Fatalf("unexpected path: %q", files[0].Path)
	}
	if !strings.Contains(string(files[0].Body), `location: "/workspace/demo"`) {
		t.Fatalf("expected workdir mount in lima.yaml: %s", string(files[0].Body))
	}
	if !strings.Contains(string(files[0].Body), "mountType: reverse-sshfs") {
		t.Fatalf("expected qemu mount type override in lima.yaml: %s", string(files[0].Body))
	}
	if !strings.Contains(string(files[0].Body), "images:") {
		t.Fatalf("expected base image in lima.yaml: %s", string(files[0].Body))
	}
	if !strings.Contains(string(files[0].Body), `location: "https://example.test/debian-amd64.qcow2"`) {
		t.Fatalf("expected configured image in lima.yaml: %s", string(files[0].Body))
	}
	if !strings.Contains(string(files[0].Body), "containerd:") {
		t.Fatalf("expected containerd to be configured in lima.yaml: %s", string(files[0].Body))
	}
	if !strings.Contains(string(files[0].Body), "mountPoint: /host-dot-sandstorm") {
		t.Fatalf("expected host sandstorm mount in lima.yaml: %s", string(files[0].Body))
	}
	if !strings.Contains(string(files[0].Body), "hostPort: 6020") {
		t.Fatalf("expected configured host port in lima.yaml: %s", string(files[0].Body))
	}
	if !strings.Contains(string(files[0].Body), "ignore: true") {
		t.Fatalf("expected catch-all port forward ignore rule in lima.yaml: %s", string(files[0].Body))
	}
}

func TestBootstrapFilesUseConfiguredArm64Image(t *testing.T) {
	t.Parallel()

	provider := New(&captureRunner{}, templates.New())
	files, err := provider.BootstrapFiles(providers.ProjectContext{
		WorkDir: "/workspace/demo",
		Config: &config.Resolved{
			Lima: config.LimaResolved{
				VMType:    "vz",
				Arch:      "aarch64",
				Image:     "https://example.test/debian-arm64.qcow2",
				ImageArch: "aarch64",
			},
			Network: config.NetworkResolved{Sandstorm: config.SandstormResolved{
				GuestPort:    6090,
				ExternalPort: 6090,
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := string(files[0].Body)
	if !strings.Contains(body, `arch: aarch64`) {
		t.Fatalf("expected configured arch in lima.yaml: %s", body)
	}
	if !strings.Contains(body, `mountType: virtiofs`) {
		t.Fatalf("expected vz mount type in lima.yaml: %s", body)
	}
	if !strings.Contains(body, `location: "https://example.test/debian-arm64.qcow2"`) {
		t.Fatalf("expected configured arm64 image in lima.yaml: %s", body)
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

func TestProvisionUsesExistingInstanceShell(t *testing.T) {
	t.Parallel()

	r := &captureRunner{}
	provider := New(r, templates.New())
	err := provider.Provision(context.Background(), providers.ProjectContext{WorkDir: "/workspace/demo"})
	if err != nil {
		t.Fatal(err)
	}
	if r.spec.Command != "limactl" {
		t.Fatalf("unexpected command: %q", r.spec.Command)
	}
	got := strings.Join(r.spec.Args, " ")
	if !strings.Contains(got, "shell --workdir /opt/app/.sandstorm sandstorm-demo-") {
		t.Fatalf("unexpected args: %q", got)
	}
	if !strings.Contains(got, "sudo bash -lc ./global-setup.sh && ./setup.sh") {
		t.Fatalf("unexpected provision command: %q", got)
	}
	if !r.spec.Stream {
		t.Fatal("expected streamed provisioning output")
	}
}
