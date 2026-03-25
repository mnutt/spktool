package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mnutt/spktool/internal/domain"
	"github.com/mnutt/spktool/internal/providers"
	"github.com/mnutt/spktool/internal/runner"
	"github.com/mnutt/spktool/internal/services"
)

type fakeApp struct {
	setupVM     func(context.Context, string, domain.ProviderName, string, bool) (*domain.ProjectState, error)
	renderCfg   func(context.Context, string, domain.ProviderName) (*services.ConfigRender, error)
	dev         func(context.Context, string, domain.ProviderName) (*domain.ProjectState, error)
	add         func(context.Context, string, string) (*domain.ProjectState, error)
	listUtils   func(context.Context) (*services.UtilityCatalog, error)
	describe    func(context.Context, string) (*services.UtilityDetails, error)
	install     func(context.Context, string, services.InstallSkillsRequest) (*services.InstallSkillsResult, error)
	pack        func(context.Context, string, string, domain.ProviderName) (*domain.ProjectState, error)
	verify      func(context.Context, string, string, domain.ProviderName) (*domain.ProjectState, error)
	publish     func(context.Context, string, string, domain.ProviderName) (*domain.ProjectState, error)
	keygen      func(context.Context, string, []string, domain.ProviderName) (runner.Result, error)
	listkeys    func(context.Context, string, []string, domain.ProviderName) (runner.Result, error)
	getkey      func(context.Context, string, string, domain.ProviderName) (runner.Result, error)
	enterGrain  func(context.Context, string, domain.ProviderName, bool) (*domain.ProjectState, error)
	vmCreate    func(context.Context, string, domain.ProviderName, string) (*domain.ProjectState, error)
	vmUp        func(context.Context, string, domain.ProviderName, int) (*domain.ProjectState, error)
	vmHalt      func(context.Context, string, domain.ProviderName) (*domain.ProjectState, error)
	vmSSH       func(context.Context, string, []string, domain.ProviderName) (*domain.ProjectState, error)
	status      func(context.Context, string, domain.ProviderName) (providers.Status, error)
	vmProvision func(context.Context, string, domain.ProviderName, string) (*domain.ProjectState, error)
	stacks      []string
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
func (a *fakeApp) Add(ctx context.Context, workDir, util string) (*domain.ProjectState, error) {
	return a.add(ctx, workDir, util)
}
func (a *fakeApp) ListUtilities(ctx context.Context) (*services.UtilityCatalog, error) {
	return a.listUtils(ctx)
}
func (a *fakeApp) DescribeUtility(ctx context.Context, name string) (*services.UtilityDetails, error) {
	return a.describe(ctx, name)
}
func (a *fakeApp) InstallSkills(ctx context.Context, workDir string, req services.InstallSkillsRequest) (*services.InstallSkillsResult, error) {
	return a.install(ctx, workDir, req)
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
func (a *fakeApp) EnterGrain(ctx context.Context, workDir string, provider domain.ProviderName, noninteractive bool) (*domain.ProjectState, error) {
	return a.enterGrain(ctx, workDir, provider, noninteractive)
}
func (a *fakeApp) VMCreate(ctx context.Context, workDir string, provider domain.ProviderName, downloadURL string) (*domain.ProjectState, error) {
	return a.vmCreate(ctx, workDir, provider, downloadURL)
}
func (a *fakeApp) VMUp(ctx context.Context, workDir string, provider domain.ProviderName, port int) (*domain.ProjectState, error) {
	return a.vmUp(ctx, workDir, provider, port)
}
func (a *fakeApp) VMHalt(ctx context.Context, workDir string, provider domain.ProviderName) (*domain.ProjectState, error) {
	return a.vmHalt(ctx, workDir, provider)
}
func (a *fakeApp) VMDestroy(context.Context, string, domain.ProviderName) (*domain.ProjectState, error) {
	panic("unexpected call")
}
func (a *fakeApp) VMStatus(ctx context.Context, workDir string, provider domain.ProviderName) (providers.Status, error) {
	return a.status(ctx, workDir, provider)
}
func (a *fakeApp) VMProvision(ctx context.Context, workDir string, provider domain.ProviderName, downloadURL string) (*domain.ProjectState, error) {
	return a.vmProvision(ctx, workDir, provider, downloadURL)
}
func (a *fakeApp) VMSSH(ctx context.Context, workDir string, args []string, provider domain.ProviderName) (*domain.ProjectState, error) {
	return a.vmSSH(ctx, workDir, args, provider)
}
func (a *fakeApp) StackNames() ([]string, error) { return a.stacks, nil }

func appSet(app *fakeApp) Applications {
	return Applications{
		Bootstrap: app,
		Packages:  app,
		Keys:      app,
		Grains:    app,
		Utility:   app,
		Skills:    app,
		VM:        app,
	}
}

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

	err := Run(context.Background(), appSet(app), Config{
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

	err := Run(context.Background(), appSet(app), Config{
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

	err := Run(context.Background(), appSet(app), Config{
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

	err := Run(context.Background(), appSet(app), Config{
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

	err := Run(context.Background(), appSet(app), Config{
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

func TestRunAddDispatchesToApp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var gotWorkDir string
	var gotUtil string
	app := &fakeApp{
		stacks: []string{"node"},
		add: func(_ context.Context, workDir, util string) (*domain.ProjectState, error) {
			gotWorkDir = workDir
			gotUtil = util
			return &domain.ProjectState{Provider: domain.ProviderLima, Stack: "node", VMInstance: "sandstorm-demo"}, nil
		},
	}

	err := Run(context.Background(), appSet(app), Config{
		Program: "spktool",
		Args:    []string{"--work-directory", "/workspace/app", "add", "foo"},
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotWorkDir != "/workspace/app" || gotUtil != "foo" {
		t.Fatalf("unexpected add args: workdir=%q util=%q", gotWorkDir, gotUtil)
	}
	if got := stdout.String(); got != "provider=lima stack=node vm=sandstorm-demo\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRunListUtilsDispatchesToApp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := &fakeApp{
		stacks: []string{"node"},
		listUtils: func(_ context.Context) (*services.UtilityCatalog, error) {
			return &services.UtilityCatalog{
				Utilities: []services.UtilitySpec{{
					Name:    "stay-awake",
					Summary: "keep the grain awake",
				}},
			}, nil
		},
	}

	err := Run(context.Background(), appSet(app), Config{
		Program: "spktool",
		Args:    []string{"list-utils"},
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "stay-awake - keep the grain awake\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRunDescribeUtilDispatchesToApp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := &fakeApp{
		stacks: []string{"node"},
		describe: func(_ context.Context, name string) (*services.UtilityDetails, error) {
			if name != "stay-awake" {
				t.Fatalf("unexpected utility name: %q", name)
			}
			return &services.UtilityDetails{
				Utility: services.UtilitySpec{
					Name:        "stay-awake",
					Summary:     "Wake-lock helper",
					Description: "Acquire, renew, and release Sandstorm wake-lock leases.",
					Examples: []string{
						"stay-awake acquire <sessionId>",
						"stay-awake release <lockId>",
					},
				},
			}, nil
		},
	}

	err := Run(context.Background(), appSet(app), Config{
		Program: "spktool",
		Args:    []string{"describe-util", "stay-awake"},
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	if !bytes.Contains([]byte(got), []byte("stay-awake")) ||
		!bytes.Contains([]byte(got), []byte("Acquire, renew, and release Sandstorm wake-lock leases.")) ||
		!bytes.Contains([]byte(got), []byte("Examples:")) {
		t.Fatalf("unexpected output: %q", got)
	}
	if bytes.Contains([]byte(got), []byte("Wake-lock helper")) {
		t.Fatalf("expected summary to be omitted from describe output, got %q", got)
	}
}

func TestRunDescribeUtilRequiresName(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := Run(context.Background(), appSet(&fakeApp{stacks: []string{"node"}}), Config{
		Program: "spktool",
		Args:    []string{"describe-util"},
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "error: utility name is required\nusage: describe-util <name>\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRunListUtilsHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := Run(context.Background(), appSet(&fakeApp{stacks: []string{"node"}}), Config{
		Program: "spktool",
		Args:    []string{"list-utils", "--help"},
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "list-utils lists installable Sandstorm utilities.\n\nUsage:\n  list-utils\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRunDescribeUtilHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := Run(context.Background(), appSet(&fakeApp{stacks: []string{"node"}}), Config{
		Program: "spktool",
		Args:    []string{"describe-util", "--help"},
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "describe-util shows details for an installable Sandstorm utility.\n\nUsage:\n  describe-util <name>\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRunAddRequiresUtilName(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := Run(context.Background(), appSet(&fakeApp{stacks: []string{"node"}}), Config{
		Program: "spktool",
		Args:    []string{"add"},
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "error: utility name is required\nusage: add <util>\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRunAddHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := Run(context.Background(), appSet(&fakeApp{stacks: []string{"node"}}), Config{
		Program: "spktool",
		Args:    []string{"add", "--help"},
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "add installs a Sandstorm utility into the current project.\n\nUsage:\n  add <util>\n" {
		t.Fatalf("unexpected output: %q", got)
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

	err := Run(context.Background(), appSet(app), Config{
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
		vmCreate: func(_ context.Context, workDir string, provider domain.ProviderName, downloadURL string) (*domain.ProjectState, error) {
			if workDir != "/workspace/app" {
				t.Fatalf("unexpected workdir: %q", workDir)
			}
			if provider != domain.ProviderVagrant {
				t.Fatalf("unexpected provider: %q", provider)
			}
			if downloadURL != "" {
				t.Fatalf("did not expect download url, got %q", downloadURL)
			}
			called = true
			return &domain.ProjectState{Provider: provider, Stack: "node", VMInstance: "demo"}, nil
		},
	}

	err := Run(context.Background(), appSet(app), Config{
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

func TestRunVMProvisionParsesSandstormDownloadURL(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var gotURL string
	app := &fakeApp{
		stacks: []string{"node"},
		vmProvision: func(_ context.Context, workDir string, provider domain.ProviderName, downloadURL string) (*domain.ProjectState, error) {
			if workDir != "/workspace/app" {
				t.Fatalf("unexpected workdir: %q", workDir)
			}
			if provider != domain.ProviderLima {
				t.Fatalf("unexpected provider: %q", provider)
			}
			gotURL = downloadURL
			return &domain.ProjectState{Provider: provider, Stack: "node", VMInstance: "demo"}, nil
		},
	}

	err := Run(context.Background(), appSet(app), Config{
		Program: "spktool",
		Args: []string{
			"--work-directory", "/workspace/app",
			"--provider", "lima",
			"vm", "provision",
			"--sandstorm-download-url", "https://downloads.example.test/sandstorm-0-fast-1.tar.xz",
		},
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotURL != "https://downloads.example.test/sandstorm-0-fast-1.tar.xz" {
		t.Fatalf("unexpected download url: %q", gotURL)
	}
	if got := stdout.String(); got != "provider=lima stack=node vm=demo\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRunVMStartDispatchesToUp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var called bool
	app := &fakeApp{
		stacks: []string{"node"},
		vmUp: func(_ context.Context, workDir string, provider domain.ProviderName, port int) (*domain.ProjectState, error) {
			if workDir != "/workspace/app" {
				t.Fatalf("unexpected workdir: %q", workDir)
			}
			if provider != domain.ProviderLima {
				t.Fatalf("unexpected provider: %q", provider)
			}
			if port != 0 {
				t.Fatalf("did not expect port override, got %d", port)
			}
			called = true
			return &domain.ProjectState{Provider: provider, Stack: "node", VMInstance: "demo"}, nil
		},
	}

	err := Run(context.Background(), appSet(app), Config{
		Program:         "spktool",
		Args:            []string{"--work-directory", "/workspace/app", "vm", "start"},
		DefaultProvider: domain.ProviderLima,
		Stdout:          &stdout,
		Stderr:          &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("expected vm start to dispatch to up")
	}
	if got := stdout.String(); got != "provider=lima stack=node vm=demo\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRunVMUpParsesPortOverride(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var gotPort int
	app := &fakeApp{
		stacks: []string{"node"},
		vmUp: func(_ context.Context, workDir string, provider domain.ProviderName, port int) (*domain.ProjectState, error) {
			if workDir != "/workspace/app" {
				t.Fatalf("unexpected workdir: %q", workDir)
			}
			if provider != domain.ProviderLima {
				t.Fatalf("unexpected provider: %q", provider)
			}
			gotPort = port
			return &domain.ProjectState{Provider: provider, Stack: "node", VMInstance: "demo"}, nil
		},
	}

	err := Run(context.Background(), appSet(app), Config{
		Program: "spktool",
		Args: []string{
			"--work-directory", "/workspace/app",
			"--provider", "lima",
			"vm", "up",
			"--port", "7000",
		},
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPort != 7000 {
		t.Fatalf("unexpected port override: %d", gotPort)
	}
	if got := stdout.String(); got != "provider=lima stack=node vm=demo\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRunVMStopDispatchesToHalt(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var called bool
	app := &fakeApp{
		stacks: []string{"node"},
		vmHalt: func(_ context.Context, workDir string, provider domain.ProviderName) (*domain.ProjectState, error) {
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

	err := Run(context.Background(), appSet(app), Config{
		Program:         "spktool",
		Args:            []string{"--work-directory", "/workspace/app", "--provider", "vagrant", "vm", "stop"},
		DefaultProvider: domain.ProviderLima,
		Stdout:          &stdout,
		Stderr:          &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("expected vm stop to dispatch to halt")
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

	err := Run(context.Background(), appSet(app), Config{
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

	err := Run(context.Background(), appSet(app), Config{
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

	err := Run(context.Background(), appSet(app), Config{
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
	if payload["usage"] == "" {
		t.Fatalf("expected usage payload: %+v", payload)
	}
}

func TestRunSetupVMMissingStackPrintsUsageInTextMode(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := &fakeApp{stacks: []string{"lemp", "meteor"}}

	err := Run(context.Background(), appSet(app), Config{
		Program:         "spktool",
		Args:            []string{"setupvm"},
		DefaultProvider: domain.ProviderLima,
		Stdout:          &stdout,
		Stderr:          &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	if !bytes.Contains([]byte(got), []byte("error: stack is required")) {
		t.Fatalf("expected text error output, got %q", got)
	}
	if !bytes.Contains([]byte(got), []byte("usage: setupvm [--force] <stack>")) {
		t.Fatalf("expected setupvm usage output, got %q", got)
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

	err := Run(context.Background(), appSet(app), Config{
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

	err := Run(context.Background(), appSet(app), Config{
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

func TestRunPackMissingOutputShowsUsageInTextMode(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := Run(context.Background(), appSet(&fakeApp{stacks: []string{"lemp"}}), Config{
		Program: "spktool",
		Args:    []string{"pack"},
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); !bytes.Contains([]byte(got), []byte("usage: pack <output.spk>")) {
		t.Fatalf("expected pack usage output, got %q", got)
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

	err := Run(context.Background(), appSet(app), Config{
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

	err := Run(context.Background(), appSet(app), Config{
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

func TestFormatProjectStateWithoutColor(t *testing.T) {
	t.Parallel()

	got := formatProjectState(&domain.ProjectState{
		Provider:   domain.ProviderVagrant,
		Stack:      "node",
		VMInstance: "video-encoder",
	}, false)

	if got != "provider=vagrant stack=node vm=video-encoder" {
		t.Fatalf("unexpected uncolored output: %q", got)
	}
}

func TestFormatProjectStateWithColor(t *testing.T) {
	t.Parallel()

	got := formatProjectState(&domain.ProjectState{
		Provider:   domain.ProviderVagrant,
		Stack:      "node",
		VMInstance: "video-encoder",
	}, true)

	if !strings.Contains(got, "\x1b[90mprovider\x1b[0m") {
		t.Fatalf("expected dim provider label, got %q", got)
	}
	if !strings.Contains(got, "\x1b[37mvagrant\x1b[0m") {
		t.Fatalf("expected muted provider value, got %q", got)
	}
	if !strings.Contains(got, "\x1b[90m·\x1b[0m") {
		t.Fatalf("expected dim separators, got %q", got)
	}
}

func TestFormatProviderStatusWithoutColor(t *testing.T) {
	t.Parallel()

	got := formatProviderStatus(providers.Status{
		Provider:     domain.ProviderLima,
		InstanceName: "sandstorm-video-encoder-5841bcc4",
		State:        "stopped",
	}, false)

	if got != "provider=lima instance=sandstorm-video-encoder-5841bcc4 status=stopped" {
		t.Fatalf("unexpected uncolored output: %q", got)
	}
}

func TestFormatProviderStatusWithColor(t *testing.T) {
	t.Parallel()

	got := formatProviderStatus(providers.Status{
		Provider:     domain.ProviderLima,
		InstanceName: "sandstorm-video-encoder-5841bcc4",
		State:        "stopped",
	}, true)

	if !strings.Contains(got, "\x1b[90minstance\x1b[0m") {
		t.Fatalf("expected dim instance label, got %q", got)
	}
	if !strings.Contains(got, "\x1b[37msandstorm-video-encoder-5841bcc4\x1b[0m") {
		t.Fatalf("expected muted instance value, got %q", got)
	}
	if !strings.Contains(got, "\x1b[90mstatus\x1b[0m") {
		t.Fatalf("expected dim status label, got %q", got)
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

	err := Run(context.Background(), appSet(app), Config{
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

	err := Run(context.Background(), appSet(app), Config{
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

	err := Run(context.Background(), appSet(app), Config{
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

func TestRunGetKeyMissingIDShowsUsageInJSON(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := Run(context.Background(), appSet(&fakeApp{stacks: []string{"lemp"}}), Config{
		Program: "spktool",
		Args:    []string{"--output", "json", "getkey"},
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["error"] != "getkey key id is required" {
		t.Fatalf("unexpected json payload: %+v", payload)
	}
	if payload["usage"] == "" {
		t.Fatalf("expected usage in payload: %+v", payload)
	}
}

func TestRunEnterGrainDispatchesToApp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := &fakeApp{
		stacks: []string{"lemp"},
		enterGrain: func(_ context.Context, workDir string, _ domain.ProviderName, noninteractive bool) (*domain.ProjectState, error) {
			if workDir != "/workspace/app" {
				t.Fatalf("unexpected workdir: %q", workDir)
			}
			if noninteractive {
				t.Fatal("did not expect noninteractive mode")
			}
			return &domain.ProjectState{Provider: domain.ProviderLima, Stack: "lemp", VMInstance: "sandstorm-app"}, nil
		},
	}

	err := Run(context.Background(), appSet(app), Config{
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

func TestRunEnterGrainPassesNoninteractiveFlag(t *testing.T) {
	t.Parallel()

	app := &fakeApp{
		stacks: []string{"lemp"},
		enterGrain: func(_ context.Context, _ string, _ domain.ProviderName, noninteractive bool) (*domain.ProjectState, error) {
			if !noninteractive {
				t.Fatal("expected noninteractive mode")
			}
			return &domain.ProjectState{Provider: domain.ProviderLima, Stack: "lemp", VMInstance: "sandstorm-app"}, nil
		},
	}

	if err := Run(context.Background(), appSet(app), Config{
		Program:         "spktool",
		Args:            []string{"--noninteractive", "enter-grain"},
		DefaultProvider: domain.ProviderLima,
		Stdout:          &bytes.Buffer{},
		Stderr:          &bytes.Buffer{},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRunSetupVMHelpAfterCommandDoesNotTriggerGlobalHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := Run(context.Background(), appSet(&fakeApp{stacks: []string{"lemp"}}), Config{
		Program: "spktool",
		Args:    []string{"setupvm", "--help"},
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); !bytes.Contains([]byte(got), []byte("Usage:\n  setupvm [--force] <stack>")) {
		t.Fatalf("expected setupvm help output, got %q", got)
	}
}

func TestRunConfigHelpAfterCommandShowsConfigHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := Run(context.Background(), appSet(&fakeApp{stacks: []string{"lemp"}}), Config{
		Program: "spktool",
		Args:    []string{"config", "--help"},
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); !bytes.Contains([]byte(got), []byte("Usage:\n  config render")) {
		t.Fatalf("expected config help output, got %q", got)
	}
}

func TestRunVMHelpAfterCommandShowsVMHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := Run(context.Background(), appSet(&fakeApp{stacks: []string{"lemp"}}), Config{
		Program: "spktool",
		Args:    []string{"vm", "--help"},
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); !bytes.Contains([]byte(got), []byte("Usage:\n  vm create")) {
		t.Fatalf("expected vm help output, got %q", got)
	}
}

func TestRunVMMissingSubcommandShowsVMHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := Run(context.Background(), appSet(&fakeApp{stacks: []string{"lemp"}}), Config{
		Program: "spktool",
		Args:    []string{"vm"},
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); !bytes.Contains([]byte(got), []byte("vm manages provider instances")) {
		t.Fatalf("expected vm help output, got %q", got)
	}
}

func TestRunVMMissingSubcommandShowsUsageInJSON(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := Run(context.Background(), appSet(&fakeApp{stacks: []string{"lemp"}}), Config{
		Program: "spktool",
		Args:    []string{"--output", "json", "vm"},
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["error"] != "vm subcommand is required" {
		t.Fatalf("unexpected json payload: %+v", payload)
	}
	if payload["usage"] == "" {
		t.Fatalf("expected usage in payload: %+v", payload)
	}
}

func TestRunVMSubcommandHelpShowsVMHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := Run(context.Background(), appSet(&fakeApp{stacks: []string{"lemp"}}), Config{
		Program: "spktool",
		Args:    []string{"vm", "ssh", "--help"},
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); !bytes.Contains([]byte(got), []byte("vm ssh [args...]")) {
		t.Fatalf("expected vm help output, got %q", got)
	}
}

func TestRunConfigSubcommandHelpShowsConfigHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := Run(context.Background(), appSet(&fakeApp{stacks: []string{"lemp"}}), Config{
		Program: "spktool",
		Args:    []string{"config", "render", "--help"},
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); !bytes.Contains([]byte(got), []byte("config exposes generated configuration artifacts")) {
		t.Fatalf("expected help output, got %q", got)
	}
}

func TestRunConfigUnknownSubcommandShowsUsage(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := Run(context.Background(), appSet(&fakeApp{stacks: []string{"lemp"}}), Config{
		Program: "spktool",
		Args:    []string{"config", "show"},
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); !bytes.Contains([]byte(got), []byte("usage: config render")) {
		t.Fatalf("expected config usage output, got %q", got)
	}
}

func TestRunVMUnknownSubcommandShowsUsage(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := Run(context.Background(), appSet(&fakeApp{stacks: []string{"lemp"}}), Config{
		Program: "spktool",
		Args:    []string{"vm", "restart"},
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); !bytes.Contains([]byte(got), []byte("usage: vm create|up|halt|destroy|status|provision|ssh")) {
		t.Fatalf("expected vm usage output, got %q", got)
	}
}

func TestRunVMUpUnknownFlagShowsLongFormUsage(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := Run(context.Background(), appSet(&fakeApp{stacks: []string{"lemp"}}), Config{
		Program: "spktool",
		Args:    []string{"vm", "up", "--unknown"},
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected parse error")
	}
	got := stdout.String()
	if !bytes.Contains([]byte(got), []byte("Usage of vm up:")) {
		t.Fatalf("expected vm up usage output, got %q", got)
	}
	if !bytes.Contains([]byte(got), []byte("--port int")) {
		t.Fatalf("expected long-form port flag in usage output, got %q", got)
	}
	if bytes.Contains([]byte(got), []byte("\n  -port int")) {
		t.Fatalf("did not expect single-dash port flag in usage output, got %q", got)
	}
}

func TestRunInstallSkillsDispatchesToApp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var gotWorkDir string
	var gotReq services.InstallSkillsRequest
	app := &fakeApp{
		stacks: []string{"lemp"},
		install: func(_ context.Context, workDir string, req services.InstallSkillsRequest) (*services.InstallSkillsResult, error) {
			gotWorkDir = workDir
			gotReq = req
			return &services.InstallSkillsResult{
				Targets:          []string{"codex", "claude"},
				Directories:      []string{".codex/skills/sandstorm-app-author/"},
				GitignoreUpdated: true,
			}, nil
		},
	}

	err := Run(context.Background(), appSet(app), Config{
		Program: "spktool",
		Args:    []string{"--work-directory", "/tmp/demo", "--noninteractive", "install-skills", "--codex", "--claude", "--force"},
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotWorkDir != "/tmp/demo" {
		t.Fatalf("unexpected workdir: %q", gotWorkDir)
	}
	if !gotReq.Codex || !gotReq.Claude || !gotReq.Force || !gotReq.NonInteractive {
		t.Fatalf("unexpected request: %+v", gotReq)
	}
	if got := stdout.String(); !bytes.Contains([]byte(got), []byte("targets: codex")) {
		t.Fatalf("expected install output, got %q", got)
	}
}

func TestRunInstallSkillsHelpShowsUsage(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := Run(context.Background(), appSet(&fakeApp{stacks: []string{"lemp"}}), Config{
		Program: "spktool",
		Args:    []string{"install-skills", "--help"},
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); !bytes.Contains([]byte(got), []byte("install-skills [--codex] [--claude] [--force]")) {
		t.Fatalf("expected install-skills help output, got %q", got)
	}
}
