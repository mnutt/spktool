package lima

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mnutt/spktool/internal/config"
	"github.com/mnutt/spktool/internal/providers"
	"github.com/mnutt/spktool/internal/providers/contracttests"
	"github.com/mnutt/spktool/internal/runner"
	"github.com/mnutt/spktool/internal/templates"
)

type captureRunner struct {
	spec   runner.Spec
	result runner.Result
}

func (r *captureRunner) Run(_ context.Context, spec runner.Spec) (runner.Result, error) {
	r.spec = spec
	if r.result.Stdout == "" && r.result.Stderr == "" && r.result.ExitCode == 0 && r.result.TraceID == "" && r.result.Command == "" && r.result.Duration == 0 {
		return runner.Result{Stdout: "ok"}, nil
	}
	return r.result, nil
}

type sequenceRunner struct {
	specs   []runner.Spec
	results []runner.Result
}

func (r *sequenceRunner) Run(_ context.Context, spec runner.Spec) (runner.Result, error) {
	r.specs = append(r.specs, spec)
	if len(r.results) == 0 {
		return runner.Result{Stdout: "ok"}, nil
	}
	result := r.results[0]
	r.results = r.results[1:]
	return result, nil
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
	provider := New(&captureRunner{}, templates.New())
	wantMountType := "9p"
	if runtime.GOOS != "darwin" {
		prevLookPath := lookPath
		prevUserHomeDir := userHomeDir
		prevReadDir := readDir
		prevReadFile := readFile
		prevCombinedOutput := combinedOutput
		lookPath = func(string) (string, error) { return "", errors.New("not found") }
		userHomeDir = func() (string, error) { return "/home/tester", nil }
		readDir = func(string) ([]os.DirEntry, error) { return nil, errors.New("not found") }
		readFile = func(string) ([]byte, error) { return nil, errors.New("not found") }
		combinedOutput = func(string, ...string) ([]byte, error) { return nil, nil }
		t.Cleanup(func() {
			lookPath = prevLookPath
			userHomeDir = prevUserHomeDir
			readDir = prevReadDir
			readFile = prevReadFile
			combinedOutput = prevCombinedOutput
		})
	}
	files, err := provider.BootstrapFiles(providers.ProjectContext{
		WorkDir: "/workspace/demo",
		Config: &config.Resolved{
			Lima: config.LimaResolved{
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
	if !strings.Contains(string(files[0].Body), "mountType: "+wantMountType) {
		t.Fatalf("expected qemu mount type override %q in lima.yaml: %s", wantMountType, string(files[0].Body))
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
	provider := New(&captureRunner{}, templates.New())
	prevLookPath := lookPath
	prevUserHomeDir := userHomeDir
	prevReadDir := readDir
	prevReadFile := readFile
	prevCombinedOutput := combinedOutput
	tempDir := t.TempDir()
	qemuBin := filepath.Join(tempDir, "bin")
	vhostDir := filepath.Join(tempDir, "share", "qemu", "vhost-user")
	if err := os.MkdirAll(qemuBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(vhostDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vhostDir, "50-virtiofsd.json"), []byte(`{"type":"fs","binary":"/usr/lib/virtiofsd"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	lookPath = func(name string) (string, error) {
		if name == "qemu-system-aarch64" {
			return filepath.Join(qemuBin, name), nil
		}
		return "", errors.New("not found")
	}
	userHomeDir = func() (string, error) { return "/home/tester", nil }
	readDir = os.ReadDir
	readFile = os.ReadFile
	combinedOutput = func(name string, args ...string) ([]byte, error) {
		if name != "/usr/lib/virtiofsd" || len(args) != 1 || args[0] != "--version" {
			t.Fatalf("unexpected virtiofsd probe: %s %v", name, args)
		}
		return []byte("virtiofsd 1.0"), nil
	}
	t.Cleanup(func() {
		lookPath = prevLookPath
		userHomeDir = prevUserHomeDir
		readDir = prevReadDir
		readFile = prevReadFile
		combinedOutput = prevCombinedOutput
	})
	files, err := provider.BootstrapFiles(providers.ProjectContext{
		WorkDir: "/workspace/demo",
		Config: &config.Resolved{
			Lima: config.LimaResolved{
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
	if !strings.Contains(body, `vmType: qemu`) {
		t.Fatalf("expected forced qemu vm type in lima.yaml: %s", body)
	}
	wantMountType := "virtiofs"
	if runtime.GOOS == "darwin" {
		wantMountType = "9p"
	}
	if !strings.Contains(body, `mountType: `+wantMountType) {
		t.Fatalf("expected %s mount type in lima.yaml: %s", wantMountType, body)
	}
	if !strings.Contains(body, `location: "https://example.test/debian-arm64.qcow2"`) {
		t.Fatalf("expected configured arm64 image in lima.yaml: %s", body)
	}
}

func TestDefaultMountType(t *testing.T) {
	prevLookPath := lookPath
	prevUserHomeDir := userHomeDir
	prevReadDir := readDir
	prevReadFile := readFile
	prevCombinedOutput := combinedOutput
	t.Cleanup(func() {
		lookPath = prevLookPath
		userHomeDir = prevUserHomeDir
		readDir = prevReadDir
		readFile = prevReadFile
		combinedOutput = prevCombinedOutput
	})

	if runtime.GOOS == "darwin" {
		if got := defaultMountType("x86_64"); got != "9p" {
			t.Fatalf("expected darwin qemu mount type 9p, got %q", got)
		}
		return
	}

	tempDir := t.TempDir()
	qemuBin := filepath.Join(tempDir, "bin")
	vhostDir := filepath.Join(tempDir, "share", "qemu", "vhost-user")
	if err := os.MkdirAll(qemuBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(vhostDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vhostDir, "50-virtiofsd.json"), []byte(`{"type":"fs","binary":"/usr/lib/virtiofsd"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	lookPath = func(name string) (string, error) {
		if name == "qemu-system-x86_64" {
			return filepath.Join(qemuBin, name), nil
		}
		return "", errors.New("not found")
	}
	userHomeDir = func() (string, error) { return "/home/tester", nil }
	readDir = os.ReadDir
	readFile = os.ReadFile
	combinedOutput = func(string, ...string) ([]byte, error) { return []byte("virtiofsd 1.0"), nil }
	if got := defaultMountType("x86_64"); got != "virtiofs" {
		t.Fatalf("expected non-darwin mount type virtiofs when qemu metadata advertises it, got %q", got)
	}

	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	if got := defaultMountType("x86_64"); got != "9p" {
		t.Fatalf("expected non-darwin mount type 9p when qemu metadata is unavailable, got %q", got)
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

func TestSSHUsesInteractiveModeWithoutArgs(t *testing.T) {
	t.Parallel()

	r := &captureRunner{}
	provider := New(r, templates.New())
	err := provider.SSH(context.Background(), providers.ProjectContext{WorkDir: "/workspace/demo"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !r.spec.Interactive {
		t.Fatal("expected interactive ssh session")
	}
	if r.spec.Stream {
		t.Fatal("did not expect streamed mode for interactive ssh")
	}
	got := strings.Join(r.spec.Args, " ")
	if !strings.Contains(got, "shell --workdir /opt/app sandstorm-demo-") {
		t.Fatalf("unexpected args: %q", got)
	}
}

func TestSSHStreamsWhenArgsAreProvided(t *testing.T) {
	t.Parallel()

	r := &captureRunner{}
	provider := New(r, templates.New())
	err := provider.SSH(context.Background(), providers.ProjectContext{WorkDir: "/workspace/demo"}, []string{"sh", "-lc", "pwd"})
	if err != nil {
		t.Fatal(err)
	}
	if r.spec.Interactive {
		t.Fatal("did not expect interactive mode for ssh command")
	}
	if !r.spec.Stream {
		t.Fatal("expected streamed mode for ssh command")
	}
	got := strings.Join(r.spec.Args, " ")
	if !strings.Contains(got, "shell sandstorm-demo-") || !strings.Contains(got, "sh -lc pwd") {
		t.Fatalf("unexpected args: %q", got)
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

func TestStatusReportsExistingInstanceState(t *testing.T) {
	t.Parallel()

	workDir := "/workspace/demo"
	expectedName := New(&captureRunner{}, templates.New()).DetectInstanceName(workDir)
	r := &captureRunner{
		result: runner.Result{
			Stdout: "{\"name\":\"other\",\"status\":\"Running\"}\n{\"name\":\"" + expectedName + "\",\"status\":\"Stopped\"}\n",
		},
	}
	provider := New(r, templates.New())
	status, err := provider.Status(context.Background(), providers.ProjectContext{WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	if status.InstanceName != expectedName {
		t.Fatalf("unexpected instance name: %+v", status)
	}
	if status.State != "stopped" {
		t.Fatalf("unexpected state: %+v", status)
	}
}

func TestStatusReturnsUnknownWhenInstanceIsMissing(t *testing.T) {
	t.Parallel()

	r := &captureRunner{
		result: runner.Result{
			Stdout: "{\"name\":\"other\",\"status\":\"Running\"}\n",
		},
	}
	provider := New(r, templates.New())
	status, err := provider.Status(context.Background(), providers.ProjectContext{WorkDir: "/workspace/demo"})
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "unknown" {
		t.Fatalf("expected unknown state, got %+v", status)
	}
}

func TestUpStartsExistingInstanceByName(t *testing.T) {
	t.Parallel()

	workDir := "/workspace/demo"
	instanceName := New(&captureRunner{}, templates.New()).DetectInstanceName(workDir)
	r := &sequenceRunner{
		results: []runner.Result{
			{Stdout: "{\"name\":\"" + instanceName + "\",\"status\":\"Stopped\"}\n"},
			{},
		},
	}
	provider := New(r, templates.New())

	if err := provider.Up(context.Background(), providers.ProjectContext{WorkDir: workDir}); err != nil {
		t.Fatal(err)
	}
	if len(r.specs) != 2 {
		t.Fatalf("expected status and start calls, got %d", len(r.specs))
	}
	if got := strings.Join(r.specs[1].Args, " "); strings.Contains(got, "lima.yaml") || strings.Contains(got, "--name") {
		t.Fatalf("expected existing instance start without config file, got %q", got)
	}
	if got := strings.Join(r.specs[1].Args, " "); !strings.Contains(got, "start --tty=false "+instanceName) {
		t.Fatalf("unexpected start args: %q", got)
	}
}

func TestProviderCoreContract(t *testing.T) {
	t.Parallel()

	instanceName := New(&captureRunner{}, templates.New()).DetectInstanceName("/workspace/demo")
	contracttests.RunProviderCoreContract(t, contracttests.CoreHarness{
		NewProvider: func(r runner.Runner, repo *templates.Repository) providers.ProviderCore {
			return New(r, repo)
		},
		Project: providers.ProjectContext{WorkDir: "/workspace/demo"},
		ExpectUp: func(t *testing.T, spec runner.Spec) {
			t.Helper()
			if spec.Command != "limactl" {
				t.Fatalf("unexpected up command: %q", spec.Command)
			}
			if !spec.Stream {
				t.Fatal("expected lima up to stream output")
			}
		},
		ExpectHalt: func(t *testing.T, spec runner.Spec) {
			t.Helper()
			if got := strings.Join(spec.Args, " "); !strings.Contains(got, "stop sandstorm-demo-") {
				t.Fatalf("unexpected halt args: %q", got)
			}
		},
		ExpectDestroy: func(t *testing.T, spec runner.Spec) {
			t.Helper()
			if got := strings.Join(spec.Args, " "); !strings.Contains(got, "delete --force sandstorm-demo-") {
				t.Fatalf("unexpected destroy args: %q", got)
			}
		},
		ExpectProvision: func(t *testing.T, spec runner.Spec) {
			t.Helper()
			if got := strings.Join(spec.Args, " "); !strings.Contains(got, "shell --workdir /opt/app/.sandstorm sandstorm-demo-") {
				t.Fatalf("unexpected provision args: %q", got)
			}
		},
		StatusStdout: "{\"name\":\"" + instanceName + "\",\"status\":\"Running\"}\n",
		ExpectStatus: func(t *testing.T, status providers.Status, spec runner.Spec) {
			t.Helper()
			if spec.Command != "limactl" {
				t.Fatalf("unexpected status command: %q", spec.Command)
			}
			if status.State != "running" {
				t.Fatalf("unexpected status: %+v", status)
			}
		},
	})
}
