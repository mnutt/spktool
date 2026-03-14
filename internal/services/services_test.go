package services_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mnutt/spktool/internal/domain"
	"github.com/mnutt/spktool/internal/keys"
	"github.com/mnutt/spktool/internal/providers"
	"github.com/mnutt/spktool/internal/runner"
	"github.com/mnutt/spktool/internal/services"
	"github.com/mnutt/spktool/internal/state"
	"github.com/mnutt/spktool/internal/templates"
)

type fakePlugin struct {
	name            domain.ProviderName
	detectInstance  string
	bootstrapFiles  []providers.RenderedFile
	execResult      runner.Result
	execErr         error
	execHook        func(providers.ProjectContext, []string) error
	lastExecCtx     providers.ProjectContext
	lastExecCmd     []string
	lastWriteFiles  []providers.RenderedFile
	lastInteractive []string
	grains          []providers.Grain
	attached        *providers.Grain
	attachChecksum  string
}

func (p *fakePlugin) Name() domain.ProviderName { return p.name }

func (p *fakePlugin) BootstrapFiles(_ providers.ProjectContext) ([]providers.RenderedFile, error) {
	return p.bootstrapFiles, nil
}

func (p *fakePlugin) DetectInstanceName(_ string) string { return p.detectInstance }

func (p *fakePlugin) Up(context.Context, providers.ProjectContext) error      { return nil }
func (p *fakePlugin) Halt(context.Context, providers.ProjectContext) error    { return nil }
func (p *fakePlugin) Destroy(context.Context, providers.ProjectContext) error { return nil }
func (p *fakePlugin) SSH(context.Context, providers.ProjectContext, []string) error {
	return nil
}
func (p *fakePlugin) Exec(_ context.Context, project providers.ProjectContext, command []string) (runner.Result, error) {
	p.lastExecCtx = project
	p.lastExecCmd = append([]string(nil), command...)
	if p.execHook != nil {
		if err := p.execHook(project, command); err != nil {
			return runner.Result{}, err
		}
	}
	return p.execResult, p.execErr
}
func (p *fakePlugin) ExecInteractive(_ context.Context, project providers.ProjectContext, command []string) error {
	p.lastExecCtx = project
	p.lastInteractive = append([]string(nil), command...)
	return p.execErr
}
func (p *fakePlugin) WriteFile(_ context.Context, _ providers.ProjectContext, file providers.RenderedFile) error {
	p.lastWriteFiles = append(p.lastWriteFiles, file)
	return nil
}
func (p *fakePlugin) ListGrains(context.Context, providers.ProjectContext) ([]providers.Grain, error) {
	return p.grains, nil
}
func (p *fakePlugin) AttachGrain(_ context.Context, _ providers.ProjectContext, grain providers.Grain, _ []byte, checksum string) error {
	p.attached = &grain
	p.attachChecksum = checksum
	return nil
}
func (p *fakePlugin) Provision(context.Context, providers.ProjectContext) error { return nil }
func (p *fakePlugin) Status(context.Context, providers.ProjectContext) (providers.Status, error) {
	return providers.Status{Provider: p.name, InstanceName: p.detectInstance, State: "reported"}, nil
}

func newService(t *testing.T, plugin providers.Plugin, home string) *services.ProjectService {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return services.NewProjectService(
		logger,
		templates.New(),
		providers.NewRegistry(plugin),
		state.New(),
		keys.NewLocalKeyring(home),
	)
}

func TestSetupVMWritesProjectFilesAndState(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{
		name:           domain.ProviderLima,
		detectInstance: "sandstorm-app-1234",
		bootstrapFiles: []providers.RenderedFile{{
			Path: filepath.Join(".sandstorm", "provider-test.txt"),
			Body: []byte("provider bootstrap\n"),
			Mode: 0o644,
		}},
	}
	svc := newService(t, plugin, home)

	st, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "meteor")
	if err != nil {
		t.Fatal(err)
	}

	if st.Provider != domain.ProviderLima || st.VMInstance != "sandstorm-app-1234" || st.Stack != "meteor" {
		t.Fatalf("unexpected state: %+v", st)
	}

	wantFiles := []string{
		filepath.Join(workDir, ".sandstorm", "global-setup.sh"),
		filepath.Join(workDir, ".sandstorm", "setup.sh"),
		filepath.Join(workDir, ".sandstorm", "build.sh"),
		filepath.Join(workDir, ".sandstorm", "launcher.sh"),
		filepath.Join(workDir, ".sandstorm", ".gitignore"),
		filepath.Join(workDir, ".sandstorm", ".gitattributes"),
		filepath.Join(workDir, ".sandstorm", "stack"),
		filepath.Join(workDir, ".sandstorm", "provider-test.txt"),
		filepath.Join(workDir, ".sandstorm", state.FileName),
		filepath.Join(home, ".sandstorm", "sandstorm-keyring"),
	}
	for _, path := range wantFiles {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}

	stackData, err := os.ReadFile(filepath.Join(workDir, ".sandstorm", "stack"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(stackData); got != "meteor\n" {
		t.Fatalf("unexpected stack marker: %q", got)
	}

	loaded, err := state.New().Load(context.Background(), workDir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || loaded.Provider != domain.ProviderLima || loaded.VMInstance != "sandstorm-app-1234" {
		t.Fatalf("unexpected loaded state: %+v", loaded)
	}
}

func TestInitBuildsSPKCommandWithStackInitArgs(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{
		name:           domain.ProviderLima,
		detectInstance: "sandstorm-node-9999",
	}
	svc := newService(t, plugin, home)

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "meteor"); err != nil {
		t.Fatal(err)
	}

	st, err := svc.Init(context.Background(), workDir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Provider != domain.ProviderLima {
		t.Fatalf("unexpected provider: %+v", st)
	}

	got := strings.Join(plugin.lastExecCmd, " ")
	if !strings.Contains(got, "spk init -p 8000") {
		t.Fatalf("missing spk init prelude: %q", got)
	}
	if !strings.Contains(got, "--keyring=/host-dot-sandstorm/sandstorm-keyring") {
		t.Fatalf("missing keyring flag: %q", got)
	}
	if !strings.Contains(got, "--output=/opt/app/.sandstorm/sandstorm-pkgdef.capnp") {
		t.Fatalf("missing output flag: %q", got)
	}
	if !strings.Contains(got, "-I /home/vagrant/bundle") || !strings.Contains(got, "-I /opt/meteor-spk/meteor-spk.deps") {
		t.Fatalf("expected stack initargs to be included: %q", got)
	}
	if !strings.Contains(got, "/bin/bash /opt/app/.sandstorm/launcher.sh") {
		t.Fatalf("missing launcher invocation: %q", got)
	}
	if plugin.lastExecCtx.State == nil || plugin.lastExecCtx.State.Stack != "meteor" {
		t.Fatalf("provider context missing state: %+v", plugin.lastExecCtx.State)
	}
}

func TestUpgradeVMBumpsMigrationAndRewritesBootstrapFiles(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{
		name:           domain.ProviderVagrant,
		detectInstance: "demo",
		bootstrapFiles: []providers.RenderedFile{{
			Path: filepath.Join(".sandstorm", "provider-file.txt"),
			Body: []byte("v1\n"),
			Mode: 0o644,
		}},
	}
	svc := newService(t, plugin, home)

	st, err := svc.SetupVM(context.Background(), workDir, domain.ProviderVagrant, "lemp")
	if err != nil {
		t.Fatal(err)
	}
	if st.Migration != 1 {
		t.Fatalf("initial migration = %d", st.Migration)
	}

	plugin.bootstrapFiles = []providers.RenderedFile{{
		Path: filepath.Join(".sandstorm", "provider-file.txt"),
		Body: []byte("v2\n"),
		Mode: 0o644,
	}}
	st, err = svc.UpgradeVM(context.Background(), workDir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Migration != 2 {
		t.Fatalf("expected migration 2, got %d", st.Migration)
	}

	data, err := os.ReadFile(filepath.Join(workDir, ".sandstorm", "provider-file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "v2\n" {
		t.Fatalf("unexpected provider file contents: %q", string(data))
	}
}

func TestDevUploadsHelpersAndStartsInteractiveSession(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{
		name:           domain.ProviderLima,
		detectInstance: "sandstorm-app-1234",
	}
	svc := newService(t, plugin, home)

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "lemp"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Dev(context.Background(), workDir); err != nil {
		t.Fatal(err)
	}

	if len(plugin.lastWriteFiles) != 2 {
		t.Fatalf("expected 2 helper uploads, got %d", len(plugin.lastWriteFiles))
	}
	if !strings.HasSuffix(plugin.lastWriteFiles[0].Path, "/lima-spk-devhelpers/grain-log-tailer.sh") {
		t.Fatalf("unexpected helper path: %q", plugin.lastWriteFiles[0].Path)
	}
	if !strings.HasSuffix(plugin.lastWriteFiles[1].Path, "/lima-spk-devhelpers/dev-with-tail.sh") {
		t.Fatalf("unexpected helper path: %q", plugin.lastWriteFiles[1].Path)
	}
	interactive := strings.Join(plugin.lastInteractive, " ")
	if !strings.Contains(interactive, "sg sandstorm -c") {
		t.Fatalf("unexpected interactive command: %q", interactive)
	}
	if !strings.Contains(interactive, "spk dev --pkg-def=/opt/app/.sandstorm/sandstorm-pkgdef.capnp:pkgdef") {
		t.Fatalf("missing spk dev command: %q", interactive)
	}
}

func TestPackBuildsGuestPackageAndMovesHostArtifact(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "app.spk")
	plugin := &fakePlugin{
		name:           domain.ProviderLima,
		detectInstance: "sandstorm-app-1234",
		execResult:     runner.Result{},
		execHook: func(project providers.ProjectContext, _ []string) error {
			return os.WriteFile(filepath.Join(project.WorkDir, "sandstorm-package.spk"), []byte("pkg"), 0o644)
		},
	}
	svc := newService(t, plugin, home)

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "lemp"); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Pack(context.Background(), workDir, outputPath); err != nil {
		t.Fatal(err)
	}

	gotCmd := strings.Join(plugin.lastExecCmd, " ")
	if !strings.Contains(gotCmd, "spk pack") || !strings.Contains(gotCmd, "spk verify --details /tmp/sandstorm-package.spk") {
		t.Fatalf("unexpected pack command: %q", gotCmd)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "pkg" {
		t.Fatalf("unexpected output contents: %q", string(data))
	}
}

func TestVerifyStagesPackageRunsGuestVerifyAndCleansUp(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	input := filepath.Join(t.TempDir(), "input.spk")
	plugin := &fakePlugin{
		name:           domain.ProviderVagrant,
		detectInstance: "app",
	}
	svc := newService(t, plugin, home)

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderVagrant, "lemp"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(input, []byte("verify-me"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Verify(context.Background(), workDir, input); err != nil {
		t.Fatal(err)
	}

	gotCmd := strings.Join(plugin.lastExecCmd, " ")
	if !strings.Contains(gotCmd, "spk verify --details /opt/app/.sandstorm/input.spk") {
		t.Fatalf("unexpected verify command: %q", gotCmd)
	}
	if _, err := os.Stat(filepath.Join(workDir, ".sandstorm", "input.spk")); !os.IsNotExist(err) {
		t.Fatalf("expected staged file cleanup, got err=%v", err)
	}
}

func TestPublishStagesPackageRunsGuestPublishAndCleansUp(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	input := filepath.Join(t.TempDir(), "publish.spk")
	plugin := &fakePlugin{
		name:           domain.ProviderLima,
		detectInstance: "sandstorm-app",
	}
	svc := newService(t, plugin, home)

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "lemp"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(input, []byte("publish-me"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Publish(context.Background(), workDir, input); err != nil {
		t.Fatal(err)
	}

	gotCmd := strings.Join(plugin.lastExecCmd, " ")
	if !strings.Contains(gotCmd, "spk publish --keyring=/host-dot-sandstorm/sandstorm-keyring /opt/app/.sandstorm/publish.spk") {
		t.Fatalf("unexpected publish command: %q", gotCmd)
	}
	if _, err := os.Stat(filepath.Join(workDir, ".sandstorm", "publish.spk")); !os.IsNotExist(err) {
		t.Fatalf("expected staged file cleanup, got err=%v", err)
	}
}

func TestKeygenRunsSPKInsideGuest(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{
		name:           domain.ProviderLima,
		detectInstance: "sandstorm-app",
		execResult:     runner.Result{Stdout: "kid123\n"},
	}
	svc := newService(t, plugin, home)

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "lemp"); err != nil {
		t.Fatal(err)
	}
	result, err := svc.Keygen(context.Background(), workDir, []string{"--admin"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "kid123\n" {
		t.Fatalf("unexpected result: %+v", result)
	}
	gotCmd := strings.Join(plugin.lastExecCmd, " ")
	if !strings.Contains(gotCmd, "spk keygen --keyring=/host-dot-sandstorm/sandstorm-keyring --admin") {
		t.Fatalf("unexpected keygen command: %q", gotCmd)
	}
}

func TestListKeysRunsSPKInsideGuest(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{
		name:           domain.ProviderVagrant,
		detectInstance: "app",
		execResult:     runner.Result{Stdout: "key-a\nkey-b\n"},
	}
	svc := newService(t, plugin, home)

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderVagrant, "lemp"); err != nil {
		t.Fatal(err)
	}
	result, err := svc.ListKeys(context.Background(), workDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "key-a\nkey-b\n" {
		t.Fatalf("unexpected result: %+v", result)
	}
	gotCmd := strings.Join(plugin.lastExecCmd, " ")
	if !strings.Contains(gotCmd, "spk listkeys --keyring=/host-dot-sandstorm/sandstorm-keyring") {
		t.Fatalf("unexpected listkeys command: %q", gotCmd)
	}
}

func TestGetKeyRequiresKeyIDAndRunsSPKInsideGuest(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{
		name:           domain.ProviderLima,
		detectInstance: "sandstorm-app",
		execResult:     runner.Result{Stdout: "private-key\n"},
	}
	svc := newService(t, plugin, home)

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "lemp"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetKey(context.Background(), workDir, ""); err == nil {
		t.Fatal("expected missing key id error")
	}
	result, err := svc.GetKey(context.Background(), workDir, "kid123")
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "private-key\n" {
		t.Fatalf("unexpected result: %+v", result)
	}
	gotCmd := strings.Join(plugin.lastExecCmd, " ")
	if !strings.Contains(gotCmd, "spk getkey --keyring=/host-dot-sandstorm/sandstorm-keyring kid123") {
		t.Fatalf("unexpected getkey command: %q", gotCmd)
	}
}

func TestEnterGrainChoosesSingleGrainAndAttaches(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{
		name:           domain.ProviderLima,
		detectInstance: "sandstorm-app",
		grains: []providers.Grain{{
			SupervisorPID: "100",
			GrainID:       "grain123",
			ChildPID:      200,
		}},
	}
	svc := newService(t, plugin, home)

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "lemp"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EnterGrain(context.Background(), workDir); err != nil {
		t.Fatal(err)
	}
	if plugin.attached == nil || plugin.attached.GrainID != "grain123" {
		t.Fatalf("unexpected attached grain: %+v", plugin.attached)
	}
	if plugin.attachChecksum == "" {
		t.Fatal("expected checksum to be passed to provider")
	}
}

func TestLoadProjectMigratesLegacyVagrantLayout(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{
		name:           domain.ProviderVagrant,
		detectInstance: "legacy-vagrant-instance",
		execResult:     runner.Result{Stdout: "legacy-key\n"},
	}
	svc := newService(t, plugin, home)

	if err := os.MkdirAll(filepath.Join(workDir, ".sandstorm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".sandstorm", "stack"), []byte("lemp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".sandstorm", "Vagrantfile"), []byte("Vagrant.configure(\"2\") do |config|\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := svc.ListKeys(context.Background(), workDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "legacy-key\n" {
		t.Fatalf("unexpected result: %+v", result)
	}

	st, err := state.New().Load(context.Background(), workDir)
	if err != nil {
		t.Fatal(err)
	}
	if st == nil || st.Provider != domain.ProviderVagrant || st.Stack != "lemp" || st.VMInstance != "legacy-vagrant-instance" {
		t.Fatalf("unexpected migrated state: %+v", st)
	}
}

func TestLoadProjectMigratesLegacyLimaLayout(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{
		name:           domain.ProviderLima,
		detectInstance: "legacy-lima-instance",
		execResult:     runner.Result{Stdout: "legacy-key\n"},
	}
	svc := newService(t, plugin, home)

	if err := os.MkdirAll(filepath.Join(workDir, ".sandstorm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".sandstorm", "stack"), []byte("node\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".sandstorm", "lima.yaml"), []byte("mounts: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := svc.ListKeys(context.Background(), workDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "legacy-key\n" {
		t.Fatalf("unexpected result: %+v", result)
	}

	st, err := state.New().Load(context.Background(), workDir)
	if err != nil {
		t.Fatal(err)
	}
	if st == nil || st.Provider != domain.ProviderLima || st.Stack != "node" || st.VMInstance != "legacy-lima-instance" {
		t.Fatalf("unexpected migrated state: %+v", st)
	}
}
