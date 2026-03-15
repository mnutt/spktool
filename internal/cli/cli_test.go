package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/mnutt/spktool/internal/domain"
	"github.com/mnutt/spktool/internal/providers"
	"github.com/mnutt/spktool/internal/runner"
	"github.com/mnutt/spktool/internal/services"
)

type fakeApp struct {
	setupVM    func(context.Context, string, domain.ProviderName, string, bool) (*domain.ProjectState, error)
	renderCfg  func(context.Context, string, domain.ProviderName) (*services.ConfigRender, error)
	dev        func(context.Context, string, domain.ProviderName) (*domain.ProjectState, error)
	pack       func(context.Context, string, string, domain.ProviderName) (*domain.ProjectState, error)
	verify     func(context.Context, string, string, domain.ProviderName) (*domain.ProjectState, error)
	publish    func(context.Context, string, string, domain.ProviderName) (*domain.ProjectState, error)
	keygen     func(context.Context, string, []string, domain.ProviderName) (runner.Result, error)
	listkeys   func(context.Context, string, []string, domain.ProviderName) (runner.Result, error)
	getkey     func(context.Context, string, string, domain.ProviderName) (runner.Result, error)
	enterGrain func(context.Context, string, domain.ProviderName) (*domain.ProjectState, error)
	vmCreate   func(context.Context, string, domain.ProviderName) (*domain.ProjectState, error)
	vmUp       func(context.Context, string, domain.ProviderName) (*domain.ProjectState, error)
	vmSSH      func(context.Context, string, []string, domain.ProviderName) (*domain.ProjectState, error)
	status     func(context.Context, string, domain.ProviderName) (providers.Status, error)
	stacks     []string
}

func (a *fakeApp) SetupVM(ctx context.Context, workDir string, provider domain.ProviderName, stack string, force bool) (*domain.ProjectState, error) {
	return a.setupVM(ctx, workDir, provider, stack, force)
}
func (a *fakeApp) UpgradeVM(context.Context, string, domain.ProviderName) (*domain.ProjectState, error) {
	panic("unexpected call")
}
func (a *fakeApp) RenderConfig(ctx context.Context, workDir string, provider domain.ProviderName) (*services.ConfigRender, error) {
	return a.renderCfg(ctx, workDir, provider)
}
func (a *fakeApp) Init(context.Context, string, domain.ProviderName) (*domain.ProjectState, error) {
	panic("unexpected call")
}
func (a *fakeApp) Dev(ctx context.Context, workDir string, provider domain.ProviderName) (*domain.ProjectState, error) {
	return a.dev(ctx, workDir, provider)
}
func (a *fakeApp) Pack(ctx context.Context, workDir, output string, provider domain.ProviderName) (*domain.ProjectState, error) {
	return a.pack(ctx, workDir, output, provider)
}
func (a *fakeApp) Verify(ctx context.Context, workDir, spkPath string, provider domain.ProviderName) (*domain.ProjectState, error) {
	return a.verify(ctx, workDir, spkPath, provider)
}
func (a *fakeApp) Publish(ctx context.Context, workDir, spkPath string, provider domain.ProviderName) (*domain.ProjectState, error) {
	return a.publish(ctx, workDir, spkPath, provider)
}
func (a *fakeApp) Keygen(ctx context.Context, workDir string, args []string, provider domain.ProviderName) (runner.Result, error) {
	return a.keygen(ctx, workDir, args, provider)
}
func (a *fakeApp) ListKeys(ctx context.Context, workDir string, args []string, provider domain.ProviderName) (runner.Result, error) {
	return a.listkeys(ctx, workDir, args, provider)
}
func (a *fakeApp) GetKey(ctx context.Context, workDir, keyID string, provider domain.ProviderName) (runner.Result, error) {
	return a.getkey(ctx, workDir, keyID, provider)
}
func (a *fakeApp) EnterGrain(ctx context.Context, workDir string, provider domain.ProviderName) (*domain.ProjectState, error) {
	return a.enterGrain(ctx, workDir, provider)
}
func (a *fakeApp) VMCreate(ctx context.Context, workDir string, provider domain.ProviderName) (*domain.ProjectState, error) {
	return a.vmCreate(ctx, workDir, provider)
}
func (a *fakeApp) VMUp(ctx context.Context, workDir string, provider domain.ProviderName) (*domain.ProjectState, error) {
	return a.vmUp(ctx, workDir, provider)
}
func (a *fakeApp) VMHalt(context.Context, string, domain.ProviderName) (*domain.ProjectState, error) {
	panic("unexpected call")
}
func (a *fakeApp) VMDestroy(context.Context, string, domain.ProviderName) (*domain.ProjectState, error) {
	panic("unexpected call")
}
func (a *fakeApp) VMStatus(ctx context.Context, workDir string, provider domain.ProviderName) (providers.Status, error) {
	return a.status(ctx, workDir, provider)
}
func (a *fakeApp) VMProvision(context.Context, string, domain.ProviderName) (*domain.ProjectState, error) {
	panic("unexpected call")
}
func (a *fakeApp) VMSSH(ctx context.Context, workDir string, args []string, provider domain.ProviderName) (*domain.ProjectState, error) {
	return a.vmSSH(ctx, workDir, args, provider)
}
func (a *fakeApp) StackNames() ([]string, error) { return a.stacks, nil }

func TestRunSetupVMUsesDefaultProviderAndPrintsText(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var gotWorkDir string
	var gotProvider domain.ProviderName
	var gotStack string

	app := &fakeApp{
		stacks: []string{"lemp"},
		setupVM: func(_ context.Context, workDir string, provider domain.ProviderName, stack string, force bool) (*domain.ProjectState, error) {
			gotWorkDir = workDir
			gotProvider = provider
			gotStack = stack
			if force {
				t.Fatal("did not expect force")
			}
			return &domain.ProjectState{
				Provider:   provider,
				Stack:      stack,
				VMInstance: "sandstorm-demo",
			}, nil
		},
	}

	err := Run(context.Background(), app, Config{
		Program:         "vagrant-spk",
		Args:            []string{"--work-directory", "/tmp/demo", "setupvm", "lemp"},
		DefaultProvider: domain.ProviderVagrant,
		Stdout:          &stdout,
		Stderr:          &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}

	if gotWorkDir != "/tmp/demo" || gotProvider != domain.ProviderVagrant || gotStack != "lemp" {
		t.Fatalf("unexpected call args: workdir=%q provider=%q stack=%q", gotWorkDir, gotProvider, gotStack)
	}
	if got := stdout.String(); got != "provider=vagrant stack=lemp vm=sandstorm-demo\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRunSetupVMParsesProviderAfterCommand(t *testing.T) {
	t.Parallel()

	var gotProvider domain.ProviderName
	var gotForce bool
	app := &fakeApp{
		stacks: []string{"node"},
		setupVM: func(_ context.Context, _ string, provider domain.ProviderName, stack string, force bool) (*domain.ProjectState, error) {
			gotProvider = provider
			gotForce = force
			return &domain.ProjectState{Provider: provider, Stack: stack, VMInstance: "sandstorm-demo"}, nil
		},
	}

	err := Run(context.Background(), app, Config{
		Program: "spktool",
		Args:    []string{"setupvm", "node", "--provider", "lima"},
		Stdout:  &bytes.Buffer{},
		Stderr:  &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotProvider != domain.ProviderLima {
		t.Fatalf("expected provider override to be parsed, got %q", gotProvider)
	}
	if gotForce {
		t.Fatal("did not expect force")
	}
}

func TestRunSetupVMParsesForceFlag(t *testing.T) {
	t.Parallel()

	var gotForce bool
	app := &fakeApp{
		stacks: []string{"node"},
		setupVM: func(_ context.Context, _ string, _ domain.ProviderName, stack string, force bool) (*domain.ProjectState, error) {
			gotForce = force
			return &domain.ProjectState{Provider: domain.ProviderLima, Stack: stack, VMInstance: "sandstorm-demo"}, nil
		},
	}

	err := Run(context.Background(), app, Config{
		Program: "spktool",
		Args:    []string{"setupvm", "--force", "node"},
		Stdout:  &bytes.Buffer{},
		Stderr:  &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !gotForce {
		t.Fatal("expected force flag to be passed through")
	}
}

func TestRunConfigRenderDispatchesToApp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var gotWorkDir string
	var gotProvider domain.ProviderName

	app := &fakeApp{
		stacks: []string{"node"},
		renderCfg: func(_ context.Context, workDir string, provider domain.ProviderName) (*services.ConfigRender, error) {
			gotWorkDir = workDir
			gotProvider = provider
			return &services.ConfigRender{
				Provider: provider,
				Files: []services.ConfigRenderFile{
					{Path: ".sandstorm/.generated/lima.yaml", Body: "vmType: vz\n"},
					{Path: ".sandstorm/.generated/runtime.env", Body: "SANDSTORM_EXTERNAL_PORT=6090\n"},
				},
			}, nil
		},
	}

	err := Run(context.Background(), app, Config{
		Program: "spktool",
		Args:    []string{"config", "render", "--provider", "lima", "--work-directory", "/workspace/app"},
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}

	if gotWorkDir != "/workspace/app" || gotProvider != domain.ProviderLima {
		t.Fatalf("unexpected call args: workdir=%q provider=%q", gotWorkDir, gotProvider)
	}
	got := stdout.String()
	if !bytes.Contains([]byte(got), []byte("== .sandstorm/.generated/lima.yaml ==")) {
		t.Fatalf("expected lima.yaml section, got %q", got)
	}
	if !bytes.Contains([]byte(got), []byte("SANDSTORM_EXTERNAL_PORT=6090")) {
		t.Fatalf("expected runtime.env body, got %q", got)
	}
}

func TestRunDevUsesAliasDefaultProvider(t *testing.T) {
	t.Parallel()

	var gotProvider domain.ProviderName
	app := &fakeApp{
		stacks: []string{"node"},
		dev: func(_ context.Context, _ string, provider domain.ProviderName) (*domain.ProjectState, error) {
			gotProvider = provider
			return &domain.ProjectState{Provider: provider, Stack: "node", VMInstance: "sandstorm-demo"}, nil
		},
	}

	err := Run(context.Background(), app, Config{
		Program:         "lima-spk",
		Args:            []string{"dev"},
		DefaultProvider: domain.ProviderLima,
		Stdout:          &bytes.Buffer{},
		Stderr:          &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotProvider != domain.ProviderLima {
		t.Fatalf("expected alias default provider, got %q", gotProvider)
	}
}

func TestRunVMStatusJSONOutput(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := &fakeApp{
		stacks: []string{"node"},
		status: func(_ context.Context, workDir string, _ domain.ProviderName) (providers.Status, error) {
			if workDir != "/workspace/app" {
				t.Fatalf("unexpected workdir: %q", workDir)
			}
			return providers.Status{
				Provider:     domain.ProviderLima,
				InstanceName: "sandstorm-app-1234",
				State:        "running",
			}, nil
		},
	}

	err := Run(context.Background(), app, Config{
		Program:         "spktool",
		Args:            []string{"--work-directory", "/workspace/app", "--output", "json", "vm", "status"},
		DefaultProvider: domain.ProviderLima,
		Stdout:          &stdout,
		Stderr:          &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}

	var payload struct {
		OK     bool             `json:"ok"`
		Result providers.Status `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || payload.Result.Provider != domain.ProviderLima || payload.Result.State != "running" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestRunVMCreateDispatchesToCreate(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var called bool
	app := &fakeApp{
		stacks: []string{"node"},
		vmCreate: func(_ context.Context, workDir string, provider domain.ProviderName) (*domain.ProjectState, error) {
			if workDir != "/workspace/app" {
				t.Fatalf("unexpected workdir: %q", workDir)
			}
			if provider != domain.ProviderVagrant {
				t.Fatalf("unexpected provider: %q", provider)
			}
			called = true
			return &domain.ProjectState{Provider: provider, Stack: "node", VMInstance: "demo"}, nil
		},
	}

	err := Run(context.Background(), app, Config{
		Program:         "spktool",
		Args:            []string{"--work-directory", "/workspace/app", "--provider", "vagrant", "vm", "create"},
		DefaultProvider: domain.ProviderLima,
		Stdout:          &stdout,
		Stderr:          &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("expected vm create dispatch")
	}
	if got := stdout.String(); got != "provider=vagrant stack=node vm=demo\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRunVMSSHPrintsProjectState(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var gotArgs []string
	app := &fakeApp{
		stacks: []string{"node"},
		vmSSH: func(_ context.Context, workDir string, args []string, provider domain.ProviderName) (*domain.ProjectState, error) {
			if workDir != "/workspace/app" {
				t.Fatalf("unexpected workdir: %q", workDir)
			}
			if provider != domain.ProviderVagrant {
				t.Fatalf("unexpected provider: %q", provider)
			}
			gotArgs = append([]string(nil), args...)
			return &domain.ProjectState{Provider: provider, Stack: "node", VMInstance: "demo"}, nil
		},
	}

	err := Run(context.Background(), app, Config{
		Program:         "spktool",
		Args:            []string{"--work-directory", "/workspace/app", "--provider", "vagrant", "vm", "ssh", "-c", "pwd"},
		DefaultProvider: domain.ProviderLima,
		Stdout:          &stdout,
		Stderr:          &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "provider=vagrant stack=node vm=demo\n" {
		t.Fatalf("unexpected CLI output: %q", got)
	}
	if len(gotArgs) != 2 || gotArgs[0] != "-c" || gotArgs[1] != "pwd" {
		t.Fatalf("unexpected ssh args: %#v", gotArgs)
	}
}

func TestRunVMSSHJSONErrorUsesResponseWrapper(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := &fakeApp{
		stacks: []string{"node"},
		vmSSH: func(_ context.Context, _ string, _ []string, _ domain.ProviderName) (*domain.ProjectState, error) {
			return nil, &domain.Error{Code: domain.ErrExternal, Op: "cli-test", Message: "ssh failed"}
		},
	}

	err := Run(context.Background(), app, Config{
		Program:         "spktool",
		Args:            []string{"--output", "json", "vm", "ssh", "-c", "pwd"},
		DefaultProvider: domain.ProviderLima,
		Stdout:          &stdout,
		Stderr:          &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["ok"] != false || payload["error"] == "" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestRunSetupVMMissingStackReturnsAvailableStacks(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := &fakeApp{stacks: []string{"lemp", "meteor"}}

	err := Run(context.Background(), app, Config{
		Program:         "spktool",
		Args:            []string{"--output", "json", "setupvm"},
		DefaultProvider: domain.ProviderLima,
		Stdout:          &stdout,
		Stderr:          &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["error"] != "stack is required" {
		t.Fatalf("unexpected error payload: %+v", payload)
	}
}

func TestRunDevDispatchesToApp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := &fakeApp{
		stacks: []string{"lemp"},
		dev: func(_ context.Context, workDir string, _ domain.ProviderName) (*domain.ProjectState, error) {
			if workDir != "/workspace/app" {
				t.Fatalf("unexpected workdir: %q", workDir)
			}
			return &domain.ProjectState{
				Provider:   domain.ProviderLima,
				Stack:      "lemp",
				VMInstance: "sandstorm-app",
			}, nil
		},
	}

	err := Run(context.Background(), app, Config{
		Program:         "spktool",
		Args:            []string{"--work-directory", "/workspace/app", "dev"},
		DefaultProvider: domain.ProviderLima,
		Stdout:          &stdout,
		Stderr:          &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "provider=lima stack=lemp vm=sandstorm-app\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRunPackDispatchesToApp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := &fakeApp{
		stacks: []string{"lemp"},
		pack: func(_ context.Context, workDir, output string, _ domain.ProviderName) (*domain.ProjectState, error) {
			if workDir != "/workspace/app" || output != "out.spk" {
				t.Fatalf("unexpected args: %q %q", workDir, output)
			}
			return &domain.ProjectState{Provider: domain.ProviderLima, Stack: "lemp", VMInstance: "sandstorm-app"}, nil
		},
	}

	err := Run(context.Background(), app, Config{
		Program:         "spktool",
		Args:            []string{"--work-directory", "/workspace/app", "pack", "out.spk"},
		DefaultProvider: domain.ProviderLima,
		Stdout:          &stdout,
		Stderr:          &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "provider=lima stack=lemp vm=sandstorm-app\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRunVerifyDispatchesToApp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := &fakeApp{
		stacks: []string{"lemp"},
		verify: func(_ context.Context, workDir, spkPath string, _ domain.ProviderName) (*domain.ProjectState, error) {
			if workDir != "/workspace/app" || spkPath != "input.spk" {
				t.Fatalf("unexpected args: %q %q", workDir, spkPath)
			}
			return &domain.ProjectState{Provider: domain.ProviderVagrant, Stack: "lemp", VMInstance: "app"}, nil
		},
	}

	err := Run(context.Background(), app, Config{
		Program:         "spktool",
		Args:            []string{"--work-directory", "/workspace/app", "verify", "input.spk"},
		DefaultProvider: domain.ProviderLima,
		Stdout:          &stdout,
		Stderr:          &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "provider=vagrant stack=lemp vm=app\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRunPublishDispatchesToApp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := &fakeApp{
		stacks: []string{"lemp"},
		publish: func(_ context.Context, workDir, spkPath string, _ domain.ProviderName) (*domain.ProjectState, error) {
			if workDir != "/workspace/app" || spkPath != "input.spk" {
				t.Fatalf("unexpected args: %q %q", workDir, spkPath)
			}
			return &domain.ProjectState{Provider: domain.ProviderLima, Stack: "lemp", VMInstance: "sandstorm-app"}, nil
		},
	}

	err := Run(context.Background(), app, Config{
		Program:         "spktool",
		Args:            []string{"--work-directory", "/workspace/app", "publish", "input.spk"},
		DefaultProvider: domain.ProviderLima,
		Stdout:          &stdout,
		Stderr:          &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "provider=lima stack=lemp vm=sandstorm-app\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRunKeygenPrintsCommandStdout(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := &fakeApp{
		stacks: []string{"lemp"},
		keygen: func(_ context.Context, workDir string, args []string, _ domain.ProviderName) (runner.Result, error) {
			if workDir != "/workspace/app" || len(args) != 1 || args[0] != "--admin" {
				t.Fatalf("unexpected args: %q %v", workDir, args)
			}
			return runner.Result{Stdout: "abcdef123456\n"}, nil
		},
	}

	err := Run(context.Background(), app, Config{
		Program:         "spktool",
		Args:            []string{"--work-directory", "/workspace/app", "keygen", "--admin"},
		DefaultProvider: domain.ProviderLima,
		Stdout:          &stdout,
		Stderr:          &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "abcdef123456\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRunListKeysJSONOutput(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := &fakeApp{
		stacks: []string{"lemp"},
		listkeys: func(_ context.Context, workDir string, args []string, _ domain.ProviderName) (runner.Result, error) {
			if workDir != "/workspace/app" || len(args) != 0 {
				t.Fatalf("unexpected args: %q %v", workDir, args)
			}
			return runner.Result{Stdout: "key-1\nkey-2\n", Command: "spk listkeys"}, nil
		},
	}

	err := Run(context.Background(), app, Config{
		Program:         "spktool",
		Args:            []string{"--work-directory", "/workspace/app", "--output", "json", "listkeys"},
		DefaultProvider: domain.ProviderLima,
		Stdout:          &stdout,
		Stderr:          &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}

	var payload struct {
		OK     bool          `json:"ok"`
		Result runner.Result `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || payload.Result.Stdout != "key-1\nkey-2\n" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestRunGetKeyDispatchesToApp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := &fakeApp{
		stacks: []string{"lemp"},
		getkey: func(_ context.Context, workDir, keyID string, _ domain.ProviderName) (runner.Result, error) {
			if workDir != "/workspace/app" || keyID != "kid123" {
				t.Fatalf("unexpected args: %q %q", workDir, keyID)
			}
			return runner.Result{Stdout: "private-key\n"}, nil
		},
	}

	err := Run(context.Background(), app, Config{
		Program:         "spktool",
		Args:            []string{"--work-directory", "/workspace/app", "getkey", "kid123"},
		DefaultProvider: domain.ProviderLima,
		Stdout:          &stdout,
		Stderr:          &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "private-key\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRunEnterGrainDispatchesToApp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := &fakeApp{
		stacks: []string{"lemp"},
		enterGrain: func(_ context.Context, workDir string, _ domain.ProviderName) (*domain.ProjectState, error) {
			if workDir != "/workspace/app" {
				t.Fatalf("unexpected workdir: %q", workDir)
			}
			return &domain.ProjectState{Provider: domain.ProviderLima, Stack: "lemp", VMInstance: "sandstorm-app"}, nil
		},
	}

	err := Run(context.Background(), app, Config{
		Program:         "spktool",
		Args:            []string{"--work-directory", "/workspace/app", "enter-grain"},
		DefaultProvider: domain.ProviderLima,
		Stdout:          &stdout,
		Stderr:          &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "provider=lima stack=lemp vm=sandstorm-app\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}
