package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/mnutt/spktool/internal/domain"
	"github.com/mnutt/spktool/internal/providers"
	"github.com/mnutt/spktool/internal/runner"
)

type fakeApp struct {
	setupVM    func(context.Context, string, domain.ProviderName, string) (*domain.ProjectState, error)
	dev        func(context.Context, string) (*domain.ProjectState, error)
	pack       func(context.Context, string, string) (*domain.ProjectState, error)
	verify     func(context.Context, string, string) (*domain.ProjectState, error)
	publish    func(context.Context, string, string) (*domain.ProjectState, error)
	keygen     func(context.Context, string, []string) (runner.Result, error)
	listkeys   func(context.Context, string, []string) (runner.Result, error)
	getkey     func(context.Context, string, string) (runner.Result, error)
	enterGrain func(context.Context, string) (*domain.ProjectState, error)
	vmUp       func(context.Context, string) (*domain.ProjectState, error)
	status     func(context.Context, string) (providers.Status, error)
	stacks     []string
}

func (a *fakeApp) SetupVM(ctx context.Context, workDir string, provider domain.ProviderName, stack string) (*domain.ProjectState, error) {
	return a.setupVM(ctx, workDir, provider, stack)
}
func (a *fakeApp) UpgradeVM(context.Context, string) (*domain.ProjectState, error) {
	panic("unexpected call")
}
func (a *fakeApp) Init(context.Context, string) (*domain.ProjectState, error) {
	panic("unexpected call")
}
func (a *fakeApp) Dev(ctx context.Context, workDir string) (*domain.ProjectState, error) {
	return a.dev(ctx, workDir)
}
func (a *fakeApp) Pack(ctx context.Context, workDir, output string) (*domain.ProjectState, error) {
	return a.pack(ctx, workDir, output)
}
func (a *fakeApp) Verify(ctx context.Context, workDir, spkPath string) (*domain.ProjectState, error) {
	return a.verify(ctx, workDir, spkPath)
}
func (a *fakeApp) Publish(ctx context.Context, workDir, spkPath string) (*domain.ProjectState, error) {
	return a.publish(ctx, workDir, spkPath)
}
func (a *fakeApp) Keygen(ctx context.Context, workDir string, args []string) (runner.Result, error) {
	return a.keygen(ctx, workDir, args)
}
func (a *fakeApp) ListKeys(ctx context.Context, workDir string, args []string) (runner.Result, error) {
	return a.listkeys(ctx, workDir, args)
}
func (a *fakeApp) GetKey(ctx context.Context, workDir, keyID string) (runner.Result, error) {
	return a.getkey(ctx, workDir, keyID)
}
func (a *fakeApp) EnterGrain(ctx context.Context, workDir string) (*domain.ProjectState, error) {
	return a.enterGrain(ctx, workDir)
}
func (a *fakeApp) VMUp(ctx context.Context, workDir string) (*domain.ProjectState, error) {
	return a.vmUp(ctx, workDir)
}
func (a *fakeApp) VMHalt(context.Context, string) (*domain.ProjectState, error) {
	panic("unexpected call")
}
func (a *fakeApp) VMDestroy(context.Context, string) (*domain.ProjectState, error) {
	panic("unexpected call")
}
func (a *fakeApp) VMStatus(ctx context.Context, workDir string) (providers.Status, error) {
	return a.status(ctx, workDir)
}
func (a *fakeApp) VMProvision(context.Context, string) (*domain.ProjectState, error) {
	panic("unexpected call")
}
func (a *fakeApp) VMSSH(context.Context, string, []string) (*domain.ProjectState, error) {
	panic("unexpected call")
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
		setupVM: func(_ context.Context, workDir string, provider domain.ProviderName, stack string) (*domain.ProjectState, error) {
			gotWorkDir = workDir
			gotProvider = provider
			gotStack = stack
			return &domain.ProjectState{
				Provider:   provider,
				Stack:      stack,
				VMInstance: "sandstorm-demo",
				Migration:  1,
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
	if got := stdout.String(); got != "provider=vagrant stack=lemp vm=sandstorm-demo migration=1\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRunVMStatusJSONOutput(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := &fakeApp{
		stacks: []string{"node"},
		status: func(_ context.Context, workDir string) (providers.Status, error) {
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
		dev: func(_ context.Context, workDir string) (*domain.ProjectState, error) {
			if workDir != "/workspace/app" {
				t.Fatalf("unexpected workdir: %q", workDir)
			}
			return &domain.ProjectState{
				Provider:   domain.ProviderLima,
				Stack:      "lemp",
				VMInstance: "sandstorm-app",
				Migration:  1,
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
	if got := stdout.String(); got != "provider=lima stack=lemp vm=sandstorm-app migration=1\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRunPackDispatchesToApp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := &fakeApp{
		stacks: []string{"lemp"},
		pack: func(_ context.Context, workDir, output string) (*domain.ProjectState, error) {
			if workDir != "/workspace/app" || output != "out.spk" {
				t.Fatalf("unexpected args: %q %q", workDir, output)
			}
			return &domain.ProjectState{Provider: domain.ProviderLima, Stack: "lemp", VMInstance: "sandstorm-app", Migration: 1}, nil
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
	if got := stdout.String(); got != "provider=lima stack=lemp vm=sandstorm-app migration=1\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRunVerifyDispatchesToApp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := &fakeApp{
		stacks: []string{"lemp"},
		verify: func(_ context.Context, workDir, spkPath string) (*domain.ProjectState, error) {
			if workDir != "/workspace/app" || spkPath != "input.spk" {
				t.Fatalf("unexpected args: %q %q", workDir, spkPath)
			}
			return &domain.ProjectState{Provider: domain.ProviderVagrant, Stack: "lemp", VMInstance: "app", Migration: 1}, nil
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
	if got := stdout.String(); got != "provider=vagrant stack=lemp vm=app migration=1\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRunPublishDispatchesToApp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := &fakeApp{
		stacks: []string{"lemp"},
		publish: func(_ context.Context, workDir, spkPath string) (*domain.ProjectState, error) {
			if workDir != "/workspace/app" || spkPath != "input.spk" {
				t.Fatalf("unexpected args: %q %q", workDir, spkPath)
			}
			return &domain.ProjectState{Provider: domain.ProviderLima, Stack: "lemp", VMInstance: "sandstorm-app", Migration: 1}, nil
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
	if got := stdout.String(); got != "provider=lima stack=lemp vm=sandstorm-app migration=1\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRunKeygenPrintsCommandStdout(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := &fakeApp{
		stacks: []string{"lemp"},
		keygen: func(_ context.Context, workDir string, args []string) (runner.Result, error) {
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
		listkeys: func(_ context.Context, workDir string, args []string) (runner.Result, error) {
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
		getkey: func(_ context.Context, workDir, keyID string) (runner.Result, error) {
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
		enterGrain: func(_ context.Context, workDir string) (*domain.ProjectState, error) {
			if workDir != "/workspace/app" {
				t.Fatalf("unexpected workdir: %q", workDir)
			}
			return &domain.ProjectState{Provider: domain.ProviderLima, Stack: "lemp", VMInstance: "sandstorm-app", Migration: 1}, nil
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
	if got := stdout.String(); got != "provider=lima stack=lemp vm=sandstorm-app migration=1\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}
