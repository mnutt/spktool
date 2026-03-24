package services_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mnutt/spktool/internal/config"
	"github.com/mnutt/spktool/internal/domain"
	"github.com/mnutt/spktool/internal/keys"
	"github.com/mnutt/spktool/internal/providers"
	"github.com/mnutt/spktool/internal/runner"
	"github.com/mnutt/spktool/internal/services"
	"github.com/mnutt/spktool/internal/templates"
)

type fakePlugin struct {
	name            domain.ProviderName
	detectInstance  string
	bootstrapFiles  []providers.RenderedFile
	execResult      runner.Result
	execErr         error
	upErr           error
	haltErr         error
	destroyErr      error
	sshErr          error
	provisionErr    error
	statusResult    providers.Status
	statusErr       error
	execHook        func(providers.ProjectContext, []string) error
	upHook          func(providers.ProjectContext) error
	haltHook        func(providers.ProjectContext) error
	destroyHook     func(providers.ProjectContext) error
	sshHook         func(providers.ProjectContext, []string) error
	provisionHook   func(providers.ProjectContext) error
	statusHook      func(providers.ProjectContext) (providers.Status, error)
	lastExecCtx     providers.ProjectContext
	lastExecCmd     []string
	lastActionCtx   providers.ProjectContext
	lastSSHArgs     []string
	lastWriteFiles  []providers.RenderedFile
	lastInteractive []string
	grains          []providers.Grain
	attached        *providers.Grain
	attachChecksum  string
	forwarded       *providers.Grain
	forwardLocal    int
	forwardTarget   int
}

func (p *fakePlugin) Name() domain.ProviderName { return p.name }

func (p *fakePlugin) BootstrapFiles(_ providers.ProjectContext) ([]providers.RenderedFile, error) {
	return p.bootstrapFiles, nil
}

func (p *fakePlugin) DetectInstanceName(_ string) string { return p.detectInstance }

func (p *fakePlugin) Up(_ context.Context, project providers.ProjectContext) error {
	p.lastActionCtx = project
	if p.upHook != nil {
		return p.upHook(project)
	}
	return p.upErr
}
func (p *fakePlugin) Halt(_ context.Context, project providers.ProjectContext) error {
	p.lastActionCtx = project
	if p.haltHook != nil {
		return p.haltHook(project)
	}
	return p.haltErr
}
func (p *fakePlugin) Destroy(_ context.Context, project providers.ProjectContext) error {
	p.lastActionCtx = project
	if p.destroyHook != nil {
		return p.destroyHook(project)
	}
	return p.destroyErr
}
func (p *fakePlugin) SSH(_ context.Context, project providers.ProjectContext, args []string) error {
	p.lastActionCtx = project
	p.lastSSHArgs = append([]string(nil), args...)
	if p.sshHook != nil {
		return p.sshHook(project, args)
	}
	return p.sshErr
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
func (p *fakePlugin) ForwardGrainPort(_ context.Context, _ providers.ProjectContext, grain providers.Grain, _ []byte, checksum string, localPort, grainPort int) error {
	p.forwarded = &grain
	p.attachChecksum = checksum
	p.forwardLocal = localPort
	p.forwardTarget = grainPort
	return nil
}
func (p *fakePlugin) Provision(_ context.Context, project providers.ProjectContext) error {
	p.lastActionCtx = project
	if p.provisionHook != nil {
		return p.provisionHook(project)
	}
	return p.provisionErr
}
func (p *fakePlugin) Status(_ context.Context, project providers.ProjectContext) (providers.Status, error) {
	p.lastActionCtx = project
	if p.statusHook != nil {
		return p.statusHook(project)
	}
	if p.statusResult.Provider != "" || p.statusResult.InstanceName != "" || p.statusResult.State != "" || p.statusErr != nil {
		return p.statusResult, p.statusErr
	}
	return providers.Status{Provider: p.name, InstanceName: p.detectInstance, State: "reported"}, nil
}

type testServices struct {
	*services.ProjectBootstrapService
	*services.PackageService
	*services.GrainService
	*services.KeyService
	*services.UtilityService
	*services.SkillService
	*services.VMLifecycleService
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newService(t *testing.T, plugin providers.Plugin, home string) *testServices {
	return newServiceWithHTTPClient(t, plugin, home, &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("unexpected HTTP request: " + req.URL.String())
	})})
}

func newServiceWithHTTPClient(t *testing.T, plugin providers.Plugin, home string, httpClient *http.Client) *testServices {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := services.NewServicesWithHTTPClient(
		logger,
		templates.New(),
		providers.NewRegistry(plugin),
		keys.NewLocalKeyring(home),
		httpClient,
	)
	return &testServices{
		ProjectBootstrapService: svc.ProjectBootstrap,
		PackageService:          svc.Package,
		GrainService:            svc.Grain,
		KeyService:              svc.Key,
		UtilityService:          svc.Utility,
		SkillService:            svc.Skill,
		VMLifecycleService:      svc.VM,
	}
}

func jsonResponse(t *testing.T, status int, body any) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Body:       io.NopCloser(bytes.NewReader(data)),
		Header:     make(http.Header),
	}
}

func tarballResponse(t *testing.T, status int, files map[string]string) *http.Response {
	t.Helper()
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o755,
			Size: int64(len(body)),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Body:       io.NopCloser(bytes.NewReader(archive.Bytes())),
		Header:     make(http.Header),
	}
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

	st, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "meteor", false)
	if err != nil {
		t.Fatal(err)
	}

	if st.Provider != domain.ProviderLima || st.VMInstance != "sandstorm-app-1234" || st.Stack != "meteor" {
		t.Fatalf("unexpected state: %+v", st)
	}

	wantFiles := []string{
		filepath.Join(workDir, ".sandstorm", config.ProjectFile),
		filepath.Join(workDir, ".sandstorm", config.LocalFile),
		filepath.Join(workDir, ".sandstorm", "global-setup.sh"),
		filepath.Join(workDir, ".sandstorm", "setup.sh"),
		filepath.Join(workDir, ".sandstorm", "build.sh"),
		filepath.Join(workDir, ".sandstorm", "launcher.sh"),
		filepath.Join(workDir, ".sandstorm", ".gitignore"),
		filepath.Join(workDir, ".sandstorm", ".gitattributes"),
		filepath.Join(workDir, ".sandstorm", ".generated", "runtime.env"),
		filepath.Join(workDir, ".sandstorm", "provider-test.txt"),
		filepath.Join(home, ".sandstorm", "sandstorm-keyring"),
	}
	for _, path := range wantFiles {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}

	stackData, err := os.ReadFile(filepath.Join(workDir, ".sandstorm", config.ProjectFile))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(stackData); !strings.Contains(got, `stack = "meteor"`) {
		t.Fatalf("unexpected project config: %q", got)
	}

	loaded, err := os.ReadFile(filepath.Join(workDir, ".sandstorm", config.LocalFile))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(loaded); !strings.Contains(got, `provider = "lima"`) {
		t.Fatalf("unexpected local config: %q", got)
	}
}

func TestAddRequiresInitializedProject(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{name: domain.ProviderLima, detectInstance: "sandstorm-app-1234"}
	svc := newService(t, plugin, home)

	_, err := svc.Add(context.Background(), workDir, "foo")
	if err == nil {
		t.Fatal("expected missing project error")
	}
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("expected domain error, got %T", err)
	}
	if domainErr.Code != domain.ErrNotFound {
		t.Fatalf("unexpected error: %+v", domainErr)
	}
}

func TestAddInstallsUtilityAndTracksVersion(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{name: domain.ProviderLima, detectInstance: "sandstorm-app-1234"}
	if err := os.MkdirAll(filepath.Join(workDir, ".sandstorm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".sandstorm", config.ProjectFile), config.InitialProject("node"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".sandstorm", config.LocalFile), config.InitialLocal(domain.ProviderLima), 0o644); err != nil {
		t.Fatal(err)
	}

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "https://api.github.com/repos/mnutt/sandstorm-utils/releases/latest":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"tag_name": "v0.1.0",
				"assets": []map[string]string{
					{
						"name":                 "utils.json",
						"browser_download_url": "https://downloads.example/utils.json",
					},
					{
						"name":                 "sandstorm-utils_v0.1.0_linux_amd64.tar.gz",
						"browser_download_url": "https://downloads.example/sandstorm-utils_v0.1.0_linux_amd64.tar.gz",
					},
				},
			}), nil
		case "https://downloads.example/utils.json":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"version": 1,
				"utilities": []map[string]string{{
					"name":        "foo",
					"summary":     "utility foo",
					"description": "utility foo description",
				}},
			}), nil
		case "https://downloads.example/sandstorm-utils_v0.1.0_linux_amd64.tar.gz":
			return tarballResponse(t, http.StatusOK, map[string]string{
				"sandstorm-utils_v0.1.0_linux_amd64/bin/foo": "#!/bin/sh\necho from foo\n",
				"sandstorm-utils_v0.1.0_linux_amd64/bin/bar": "#!/bin/sh\necho from bar\n",
			}), nil
		default:
			return nil, errors.New("unexpected request: " + req.URL.String())
		}
	})}
	svc := newServiceWithHTTPClient(t, plugin, home, client)

	state, err := svc.Add(context.Background(), workDir, "foo")
	if err != nil {
		t.Fatal(err)
	}
	if state.Provider != domain.ProviderLima || state.Stack != "node" {
		t.Fatalf("unexpected state: %+v", state)
	}

	body, err := os.ReadFile(filepath.Join(workDir, ".sandstorm", "utils", "foo"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(body); got != "#!/bin/sh\necho from foo\n" {
		t.Fatalf("unexpected utility body: %q", got)
	}
	info, err := os.Stat(filepath.Join(workDir, ".sandstorm", "utils", "foo"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("expected installed utility to be executable, got %o", info.Mode().Perm())
	}

	utilsCfg, err := config.LoadUtils(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := utilsCfg.Installed["foo"]; got != "v0.1.0" {
		t.Fatalf("unexpected installed version: %q", got)
	}
}

func TestAddOverwritesExistingUtility(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{name: domain.ProviderLima, detectInstance: "sandstorm-app-1234"}
	if err := os.MkdirAll(filepath.Join(workDir, ".sandstorm", "utils"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".sandstorm", config.ProjectFile), config.InitialProject("node"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".sandstorm", config.LocalFile), config.InitialLocal(domain.ProviderLima), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".sandstorm", "utils", "foo"), []byte("old\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() == "https://api.github.com/repos/mnutt/sandstorm-utils/releases/latest" {
			return jsonResponse(t, http.StatusOK, map[string]any{
				"tag_name": "v0.1.0",
				"assets": []map[string]string{
					{
						"name":                 "utils.json",
						"browser_download_url": "https://downloads.example/utils.json",
					},
					{
						"name":                 "sandstorm-utils_v0.1.0_linux_amd64.tar.gz",
						"browser_download_url": "https://downloads.example/sandstorm-utils_v0.1.0_linux_amd64.tar.gz",
					},
				},
			}), nil
		}
		if req.URL.String() == "https://downloads.example/utils.json" {
			return jsonResponse(t, http.StatusOK, map[string]any{
				"version": 1,
				"utilities": []map[string]string{{
					"name":        "foo",
					"summary":     "utility foo",
					"description": "utility foo description",
				}},
			}), nil
		}
		return tarballResponse(t, http.StatusOK, map[string]string{
			"release/bin/foo": "new\n",
		}), nil
	})}
	svc := newServiceWithHTTPClient(t, plugin, home, client)

	if _, err := svc.Add(context.Background(), workDir, "foo"); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(workDir, ".sandstorm", "utils", "foo"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(body); got != "new\n" {
		t.Fatalf("unexpected overwritten body: %q", got)
	}
}

func TestAddFailsWhenExpectedAssetIsMissing(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{name: domain.ProviderLima, detectInstance: "sandstorm-app-1234"}
	if err := os.MkdirAll(filepath.Join(workDir, ".sandstorm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".sandstorm", config.ProjectFile), config.InitialProject("node"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".sandstorm", config.LocalFile), config.InitialLocal(domain.ProviderLima), 0o644); err != nil {
		t.Fatal(err)
	}

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(t, http.StatusOK, map[string]any{
			"tag_name": "v0.1.0",
			"assets": []map[string]string{{
				"name":                 "different.tar.gz",
				"browser_download_url": "https://downloads.example/different.tar.gz",
			}},
		}), nil
	})}
	svc := newServiceWithHTTPClient(t, plugin, home, client)

	_, err := svc.Add(context.Background(), workDir, "foo")
	if err == nil {
		t.Fatal("expected missing asset error")
	}
	if !strings.Contains(err.Error(), `does not contain asset "utils.json"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddFailsWhenUtilityIsMissingFromManifest(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{name: domain.ProviderLima, detectInstance: "sandstorm-app-1234"}
	if err := os.MkdirAll(filepath.Join(workDir, ".sandstorm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".sandstorm", config.ProjectFile), config.InitialProject("node"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".sandstorm", config.LocalFile), config.InitialLocal(domain.ProviderLima), 0o644); err != nil {
		t.Fatal(err)
	}

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() == "https://api.github.com/repos/mnutt/sandstorm-utils/releases/latest" {
			return jsonResponse(t, http.StatusOK, map[string]any{
				"tag_name": "v0.1.0",
				"assets": []map[string]string{{
					"name":                 "utils.json",
					"browser_download_url": "https://downloads.example/utils.json",
				}},
			}), nil
		}
		return jsonResponse(t, http.StatusOK, map[string]any{
			"version": 1,
			"utilities": []map[string]string{{
				"name":        "bar",
				"summary":     "utility bar",
				"description": "utility bar description",
			}},
		}), nil
	})}
	svc := newServiceWithHTTPClient(t, plugin, home, client)

	_, err := svc.Add(context.Background(), workDir, "foo")
	if err == nil {
		t.Fatal("expected missing utility error")
	}
	if !strings.Contains(err.Error(), `utility "foo" was not found in utils.json`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(workDir, ".sandstorm", config.UtilsFile)); !os.IsNotExist(statErr) {
		t.Fatalf("expected utils.toml to be absent after failed install, got %v", statErr)
	}
}

func TestAddFailsWhenBinaryIsMissingFromArchive(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{name: domain.ProviderLima, detectInstance: "sandstorm-app-1234"}
	if err := os.MkdirAll(filepath.Join(workDir, ".sandstorm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".sandstorm", config.ProjectFile), config.InitialProject("node"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".sandstorm", config.LocalFile), config.InitialLocal(domain.ProviderLima), 0o644); err != nil {
		t.Fatal(err)
	}

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "https://api.github.com/repos/mnutt/sandstorm-utils/releases/latest":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"tag_name": "v0.1.0",
				"assets": []map[string]string{
					{
						"name":                 "utils.json",
						"browser_download_url": "https://downloads.example/utils.json",
					},
					{
						"name":                 "sandstorm-utils_v0.1.0_linux_amd64.tar.gz",
						"browser_download_url": "https://downloads.example/sandstorm-utils_v0.1.0_linux_amd64.tar.gz",
					},
				},
			}), nil
		case "https://downloads.example/utils.json":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"version": 1,
				"utilities": []map[string]string{{
					"name":        "foo",
					"summary":     "utility foo",
					"description": "utility foo description",
				}},
			}), nil
		default:
			return tarballResponse(t, http.StatusOK, map[string]string{
				"release/bin/bar": "bar\n",
			}), nil
		}
	})}
	svc := newServiceWithHTTPClient(t, plugin, home, client)

	_, err := svc.Add(context.Background(), workDir, "foo")
	if err == nil {
		t.Fatal("expected missing binary error")
	}
	if !strings.Contains(err.Error(), `binary "foo" was not found`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(workDir, ".sandstorm", config.UtilsFile)); !os.IsNotExist(statErr) {
		t.Fatalf("expected utils.toml to be absent after failed install, got %v", statErr)
	}
}

func TestListUtilitiesReturnsManifestCatalog(t *testing.T) {
	t.Parallel()

	plugin := &fakePlugin{name: domain.ProviderLima, detectInstance: "sandstorm-app-1234"}
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "https://api.github.com/repos/mnutt/sandstorm-utils/releases/latest":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"tag_name": "v0.1.0",
				"assets": []map[string]string{{
					"name":                 "utils.json",
					"browser_download_url": "https://downloads.example/utils.json",
				}},
			}), nil
		case "https://downloads.example/utils.json":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"version": 1,
				"utilities": []map[string]any{{
					"name":        "stay-awake",
					"summary":     "wake-lock helper",
					"description": "Acquire, renew, and release Sandstorm wake-lock leases.",
					"examples":    []string{"stay-awake acquire <sessionId>"},
				}},
			}), nil
		default:
			return nil, errors.New("unexpected request: " + req.URL.String())
		}
	})}
	svc := newServiceWithHTTPClient(t, plugin, t.TempDir(), client)

	catalog, err := svc.ListUtilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if catalog.ReleaseTag != "v0.1.0" || catalog.Version != 1 {
		t.Fatalf("unexpected catalog metadata: %+v", catalog)
	}
	if len(catalog.Utilities) != 1 || catalog.Utilities[0].Name != "stay-awake" || catalog.Utilities[0].Description == "" {
		t.Fatalf("unexpected catalog utilities: %+v", catalog.Utilities)
	}
}

func TestDescribeUtilityReturnsDetails(t *testing.T) {
	t.Parallel()

	plugin := &fakePlugin{name: domain.ProviderLima, detectInstance: "sandstorm-app-1234"}
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "https://api.github.com/repos/mnutt/sandstorm-utils/releases/latest":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"tag_name": "v0.1.0",
				"assets": []map[string]string{{
					"name":                 "utils.json",
					"browser_download_url": "https://downloads.example/utils.json",
				}},
			}), nil
		case "https://downloads.example/utils.json":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"version": 1,
				"utilities": []map[string]any{{
					"name":        "stay-awake",
					"summary":     "wake-lock helper",
					"description": "Acquire, renew, and release Sandstorm wake-lock leases.",
					"examples":    []string{"stay-awake acquire <sessionId>", "stay-awake release <lockId>"},
				}},
			}), nil
		default:
			return nil, errors.New("unexpected request: " + req.URL.String())
		}
	})}
	svc := newServiceWithHTTPClient(t, plugin, t.TempDir(), client)

	details, err := svc.DescribeUtility(context.Background(), "stay-awake")
	if err != nil {
		t.Fatal(err)
	}
	if details.ReleaseTag != "v0.1.0" || details.Version != 1 {
		t.Fatalf("unexpected details metadata: %+v", details)
	}
	if details.Utility.Name != "stay-awake" || len(details.Utility.Examples) != 2 {
		t.Fatalf("unexpected utility details: %+v", details.Utility)
	}
}

func TestListUtilitiesRejectsUnsupportedManifestVersion(t *testing.T) {
	t.Parallel()

	plugin := &fakePlugin{name: domain.ProviderLima, detectInstance: "sandstorm-app-1234"}
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "https://api.github.com/repos/mnutt/sandstorm-utils/releases/latest":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"tag_name": "v0.1.0",
				"assets": []map[string]string{{
					"name":                 "utils.json",
					"browser_download_url": "https://downloads.example/utils.json",
				}},
			}), nil
		case "https://downloads.example/utils.json":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"version": 99,
				"utilities": []map[string]any{{
					"name":    "stay-awake",
					"summary": "wake-lock helper",
				}},
			}), nil
		default:
			return nil, errors.New("unexpected request: " + req.URL.String())
		}
	})}
	svc := newServiceWithHTTPClient(t, plugin, t.TempDir(), client)

	_, err := svc.ListUtilities(context.Background())
	if err == nil {
		t.Fatal("expected unsupported schema error")
	}
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("expected domain error, got %T", err)
	}
	if domainErr.Code != domain.ErrUnsupported {
		t.Fatalf("unexpected error: %+v", domainErr)
	}
}

func TestSetupVMFailsIfProjectConfigAlreadyExistsWithoutForce(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{
		name:           domain.ProviderLima,
		detectInstance: "sandstorm-app-1234",
	}
	svc := newService(t, plugin, home)

	if err := os.MkdirAll(filepath.Join(workDir, ".sandstorm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".sandstorm", config.ProjectFile), []byte("stack = \"lemp\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "meteor", false)
	if err == nil {
		t.Fatal("expected conflict error")
	}
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("expected domain error, got %T", err)
	}
	if domainErr.Code != domain.ErrConflict {
		t.Fatalf("unexpected error: %+v", domainErr)
	}
}

func TestSetupVMForceOverwritesProjectAndLocalConfig(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{
		name:           domain.ProviderLima,
		detectInstance: "sandstorm-app-1234",
	}
	svc := newService(t, plugin, home)

	if err := os.MkdirAll(filepath.Join(workDir, ".sandstorm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".sandstorm", config.ProjectFile), []byte("stack = \"lemp\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".sandstorm", config.LocalFile), []byte("provider = \"vagrant\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "meteor", true); err != nil {
		t.Fatal(err)
	}

	projectData, err := os.ReadFile(filepath.Join(workDir, ".sandstorm", config.ProjectFile))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(projectData); !strings.Contains(got, `stack = "meteor"`) {
		t.Fatalf("unexpected project config: %q", got)
	}

	localData, err := os.ReadFile(filepath.Join(workDir, ".sandstorm", config.LocalFile))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(localData); !strings.Contains(got, `provider = "lima"`) {
		t.Fatalf("unexpected local config: %q", got)
	}
}

func TestSetupVMPreservesExistingLocalConfigWithoutForce(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{
		name:           domain.ProviderVagrant,
		detectInstance: "sandstorm-app-1234",
	}
	svc := newService(t, plugin, home)

	if err := os.MkdirAll(filepath.Join(workDir, ".sandstorm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".sandstorm", config.LocalFile), []byte("provider = \"vagrant\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderVagrant, "meteor", false); err != nil {
		t.Fatal(err)
	}

	localData, err := os.ReadFile(filepath.Join(workDir, ".sandstorm", config.LocalFile))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(localData); !strings.Contains(got, `provider = "vagrant"`) {
		t.Fatalf("expected local config to be preserved, got %q", got)
	}
}

func TestSetupVMUpdatesExistingLocalConfigWhenProviderChanges(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{
		name:           domain.ProviderLima,
		detectInstance: "sandstorm-app-1234",
	}
	svc := newService(t, plugin, home)

	if err := os.MkdirAll(filepath.Join(workDir, ".sandstorm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".sandstorm", config.LocalFile), []byte("provider = \"vagrant\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "meteor", false); err != nil {
		t.Fatal(err)
	}

	localData, err := os.ReadFile(filepath.Join(workDir, ".sandstorm", config.LocalFile))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(localData); !strings.Contains(got, `provider = "lima"`) {
		t.Fatalf("expected local config to be updated, got %q", got)
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

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "meteor", false); err != nil {
		t.Fatal(err)
	}

	st, err := svc.Init(context.Background(), workDir, "")
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

func TestRenderConfigReturnsGeneratedArtifacts(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{
		name:           domain.ProviderLima,
		detectInstance: "sandstorm-node-9999",
		bootstrapFiles: []providers.RenderedFile{
			{
				Path: filepath.Join(".sandstorm", ".generated", "lima.yaml"),
				Body: []byte("vmType: vz\n"),
				Mode: 0o644,
			},
			{
				Path: filepath.Join(".sandstorm", "provider-file.txt"),
				Body: []byte("ignore me\n"),
				Mode: 0o644,
			},
		},
	}
	svc := newService(t, plugin, home)

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "meteor", false); err != nil {
		t.Fatal(err)
	}

	rendered, err := svc.RenderConfig(context.Background(), workDir, "")
	if err != nil {
		t.Fatal(err)
	}

	if rendered.Provider != domain.ProviderLima {
		t.Fatalf("unexpected provider: %+v", rendered)
	}
	if len(rendered.Files) != 2 {
		t.Fatalf("expected runtime.env and lima.yaml, got %+v", rendered.Files)
	}
	if rendered.Files[0].Path != ".sandstorm/.generated/runtime.env" {
		t.Fatalf("expected runtime.env first, got %+v", rendered.Files)
	}
	if !strings.Contains(rendered.Files[0].Body, "SANDSTORM_EXTERNAL_PORT=6090") {
		t.Fatalf("unexpected runtime.env body: %q", rendered.Files[0].Body)
	}
	if rendered.Files[1].Path != ".sandstorm/.generated/lima.yaml" || rendered.Files[1].Body != "vmType: vz\n" {
		t.Fatalf("unexpected provider render: %+v", rendered.Files[1])
	}
}

func TestUpgradeVMRewritesBootstrapFiles(t *testing.T) {
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

	st, err := svc.SetupVM(context.Background(), workDir, domain.ProviderVagrant, "lemp", false)
	if err != nil {
		t.Fatal(err)
	}
	if st.Provider != domain.ProviderVagrant {
		t.Fatalf("unexpected initial state: %+v", st)
	}

	plugin.bootstrapFiles = []providers.RenderedFile{{
		Path: filepath.Join(".sandstorm", "provider-file.txt"),
		Body: []byte("v2\n"),
		Mode: 0o644,
	}}
	st, err = svc.UpgradeVM(context.Background(), workDir, "")
	if err != nil {
		t.Fatal(err)
	}
	if st.Provider != domain.ProviderVagrant {
		t.Fatalf("unexpected provider after upgrade: %+v", st)
	}

	data, err := os.ReadFile(filepath.Join(workDir, ".sandstorm", "provider-file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "v2\n" {
		t.Fatalf("unexpected provider file contents: %q", string(data))
	}

	runtimeEnv, err := os.ReadFile(filepath.Join(workDir, ".sandstorm", ".generated", "runtime.env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(runtimeEnv), "SANDSTORM_GUEST_PORT=6090") {
		t.Fatalf("unexpected runtime env: %q", string(runtimeEnv))
	}

}

func TestUpgradeVMLegacyProjectWritesRuntimeEnv(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{
		name:           domain.ProviderVagrant,
		detectInstance: "legacy-vagrant-instance",
		bootstrapFiles: []providers.RenderedFile{{
			Path: filepath.Join(".sandstorm", ".generated", "Vagrantfile"),
			Body: []byte("Vagrant.configure(\"2\") do |config|\nend\n"),
			Mode: 0o644,
		}},
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

	if _, err := svc.UpgradeVM(context.Background(), workDir, ""); err != nil {
		t.Fatal(err)
	}

	runtimeEnv, err := os.ReadFile(filepath.Join(workDir, ".sandstorm", ".generated", "runtime.env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(runtimeEnv), "SANDSTORM_BASE_URL=http://local.sandstorm.io:6090") {
		t.Fatalf("unexpected runtime env: %q", string(runtimeEnv))
	}
	if !strings.Contains(string(runtimeEnv), "SANDSTORM_WILDCARD_HOST=*.local.sandstorm.io:6090") {
		t.Fatalf("unexpected runtime env: %q", string(runtimeEnv))
	}

}

func TestInstallSkillsInstallsCodexSkillAndUpdatesGitignore(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	pathDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(pathDir, "codex"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", pathDir)

	plugin := &fakePlugin{name: domain.ProviderLima, detectInstance: "demo"}
	svc := newService(t, plugin, home)

	result, err := svc.InstallSkills(context.Background(), workDir, services.InstallSkillsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Targets) != 1 || result.Targets[0] != "codex" {
		t.Fatalf("unexpected targets: %+v", result.Targets)
	}
	if !result.GitignoreUpdated {
		t.Fatal("expected .gitignore to be updated")
	}

	skillData, err := os.ReadFile(filepath.Join(workDir, ".codex", "skills", "sandstorm-app-author", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(skillData), "Sandstorm App Author") {
		t.Fatalf("unexpected skill content: %q", string(skillData))
	}

	gitignoreData, err := os.ReadFile(filepath.Join(workDir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(gitignoreData); got != ".codex/skills/sandstorm-app-author/\n" {
		t.Fatalf("unexpected .gitignore contents: %q", got)
	}
}

func TestInstallSkillsAppendsGitignoreRuleOnce(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{name: domain.ProviderLima, detectInstance: "demo"}
	svc := newService(t, plugin, home)

	if err := os.WriteFile(filepath.Join(workDir, ".gitignore"), []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.InstallSkills(context.Background(), workDir, services.InstallSkillsRequest{Codex: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.InstallSkills(context.Background(), workDir, services.InstallSkillsRequest{Codex: true, Force: true}); err != nil {
		t.Fatal(err)
	}

	gitignoreData, err := os.ReadFile(filepath.Join(workDir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(gitignoreData); got != "node_modules/\n.codex/skills/sandstorm-app-author/\n" {
		t.Fatalf("unexpected .gitignore contents: %q", got)
	}
}

func TestInstallSkillsInstallsClaudeSkillAndUpdatesGitignore(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{name: domain.ProviderLima, detectInstance: "demo"}
	svc := newService(t, plugin, home)

	result, err := svc.InstallSkills(context.Background(), workDir, services.InstallSkillsRequest{Claude: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Targets) != 1 || result.Targets[0] != "claude" {
		t.Fatalf("unexpected targets: %+v", result.Targets)
	}

	skillData, err := os.ReadFile(filepath.Join(workDir, ".claude", "skills", "sandstorm-app-author", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(skillData), "Sandstorm App Author") {
		t.Fatalf("unexpected skill content: %q", string(skillData))
	}

	gitignoreData, err := os.ReadFile(filepath.Join(workDir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(gitignoreData); got != ".claude/skills/sandstorm-app-author/\n" {
		t.Fatalf("unexpected .gitignore contents: %q", got)
	}
}

func TestInstallSkillsAutoDetectsClaudeWhenCodexIsUnavailable(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	pathDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(pathDir, "claude"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", pathDir)

	plugin := &fakePlugin{name: domain.ProviderLima, detectInstance: "demo"}
	svc := newService(t, plugin, home)

	result, err := svc.InstallSkills(context.Background(), workDir, services.InstallSkillsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Targets) != 1 || result.Targets[0] != "claude" {
		t.Fatalf("unexpected targets: %+v", result.Targets)
	}
}

func TestInstallSkillsAutoDetectsBothTargets(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	pathDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(pathDir, "codex"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pathDir, "claude"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", pathDir)

	plugin := &fakePlugin{name: domain.ProviderLima, detectInstance: "demo"}
	svc := newService(t, plugin, home)

	result, err := svc.InstallSkills(context.Background(), workDir, services.InstallSkillsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Targets) != 2 || result.Targets[0] != "codex" || result.Targets[1] != "claude" {
		t.Fatalf("unexpected targets: %+v", result.Targets)
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

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "lemp", false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Dev(context.Background(), workDir, ""); err != nil {
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
	if !strings.Contains(interactive, "/opt/app/.sandstorm/build.sh &&") {
		t.Fatalf("missing build step: %q", interactive)
	}
	if !strings.Contains(interactive, "spk dev --pkg-def=/opt/app/.sandstorm/sandstorm-pkgdef.capnp:pkgdef") {
		t.Fatalf("missing spk dev command: %q", interactive)
	}
}

func TestVMLifecycleCommandsPassResolvedProjectContext(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	var upCalls int
	var provisionCalls int
	plugin := &fakePlugin{
		name:           domain.ProviderLima,
		detectInstance: "sandstorm-app-1234",
		statusResult: providers.Status{
			Provider:     domain.ProviderLima,
			InstanceName: "sandstorm-app-1234",
			State:        "unknown",
		},
		upHook: func(project providers.ProjectContext) error {
			upCalls++
			return nil
		},
		provisionHook: func(project providers.ProjectContext) error {
			provisionCalls++
			return nil
		},
	}
	svc := newService(t, plugin, home)

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "lemp", false); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.VMCreate(context.Background(), workDir, "", ""); err != nil {
		t.Fatal(err)
	}
	if plugin.lastActionCtx.State == nil || plugin.lastActionCtx.Config == nil {
		t.Fatalf("vm create missing project context: %+v", plugin.lastActionCtx)
	}
	if plugin.lastActionCtx.State.Provider != domain.ProviderLima || plugin.lastActionCtx.State.Stack != "lemp" {
		t.Fatalf("unexpected vm create state: %+v", plugin.lastActionCtx.State)
	}
	if plugin.lastActionCtx.Config.Provider != domain.ProviderLima || plugin.lastActionCtx.Config.Stack != "lemp" {
		t.Fatalf("unexpected vm create config: %+v", plugin.lastActionCtx.Config)
	}
	if upCalls != 1 || provisionCalls != 1 {
		t.Fatalf("expected vm create to call up and provision once each, got up=%d provision=%d", upCalls, provisionCalls)
	}

	if _, err := svc.VMUp(context.Background(), workDir, "", 0); err != nil {
		t.Fatal(err)
	}
	if plugin.lastActionCtx.State == nil || plugin.lastActionCtx.Config == nil {
		t.Fatalf("vm up missing project context: %+v", plugin.lastActionCtx)
	}
	if plugin.lastActionCtx.State.Provider != domain.ProviderLima || plugin.lastActionCtx.State.Stack != "lemp" {
		t.Fatalf("unexpected vm up state: %+v", plugin.lastActionCtx.State)
	}
	if plugin.lastActionCtx.Config.Provider != domain.ProviderLima || plugin.lastActionCtx.Config.Stack != "lemp" {
		t.Fatalf("unexpected vm up config: %+v", plugin.lastActionCtx.Config)
	}
	if upCalls != 2 || provisionCalls != 1 {
		t.Fatalf("expected vm up to call up only after vm create, got up=%d provision=%d", upCalls, provisionCalls)
	}

	if _, err := svc.VMHalt(context.Background(), workDir, ""); err != nil {
		t.Fatal(err)
	}
	if plugin.lastActionCtx.State == nil || plugin.lastActionCtx.State.VMInstance != "sandstorm-app-1234" {
		t.Fatalf("unexpected vm halt context: %+v", plugin.lastActionCtx.State)
	}

	if _, err := svc.VMProvision(context.Background(), workDir, "", ""); err != nil {
		t.Fatal(err)
	}
	if plugin.lastActionCtx.Config == nil || plugin.lastActionCtx.Config.Network.Sandstorm.ExternalPort != 6090 {
		t.Fatalf("unexpected vm provision context: %+v", plugin.lastActionCtx.Config)
	}

	if _, err := svc.VMDestroy(context.Background(), workDir, ""); err != nil {
		t.Fatal(err)
	}
	if plugin.lastActionCtx.WorkDir != workDir {
		t.Fatalf("unexpected vm destroy context: %+v", plugin.lastActionCtx)
	}
}

func TestVMCreateReturnsProvisionErrorAfterSuccessfulStart(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	expectedErr := errors.New("provision failed")
	var upCalls int
	var provisionCalls int
	plugin := &fakePlugin{
		name:           domain.ProviderLima,
		detectInstance: "sandstorm-app-1234",
		statusResult: providers.Status{
			Provider:     domain.ProviderLima,
			InstanceName: "sandstorm-app-1234",
			State:        "unknown",
		},
		upHook: func(project providers.ProjectContext) error {
			upCalls++
			return nil
		},
		provisionHook: func(project providers.ProjectContext) error {
			provisionCalls++
			return expectedErr
		},
	}
	svc := newService(t, plugin, home)

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "lemp", false); err != nil {
		t.Fatal(err)
	}

	_, err := svc.VMCreate(context.Background(), workDir, "", "")
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected provision error, got %v", err)
	}
	if upCalls != 1 || provisionCalls != 1 {
		t.Fatalf("expected vm create to call up and provision once each, got up=%d provision=%d", upCalls, provisionCalls)
	}
}

func TestVMProvisionPersistsSandstormDownloadURLAndRefreshesRuntimeEnv(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{
		name:           domain.ProviderLima,
		detectInstance: "sandstorm-app-1234",
		statusResult: providers.Status{
			Provider:     domain.ProviderLima,
			InstanceName: "sandstorm-app-1234",
			State:        "running",
		},
	}
	svc := newService(t, plugin, home)

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "lemp", false); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.VMProvision(context.Background(), workDir, "", "https://downloads.example.test/sandstorm-0-fast-1.tar.xz"); err != nil {
		t.Fatal(err)
	}

	localData, err := os.ReadFile(filepath.Join(workDir, ".sandstorm", config.LocalFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(localData), `download_url = "https://downloads.example.test/sandstorm-0-fast-1.tar.xz"`) {
		t.Fatalf("expected local config to persist sandstorm download url, got:\n%s", string(localData))
	}

	runtimeEnv, err := os.ReadFile(filepath.Join(workDir, ".sandstorm", ".generated", "runtime.env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(runtimeEnv), "SANDSTORM_DOWNLOAD_URL=https://downloads.example.test/sandstorm-0-fast-1.tar.xz") {
		t.Fatalf("expected runtime.env to include sandstorm download url, got:\n%s", string(runtimeEnv))
	}
	if plugin.lastActionCtx.Config == nil || plugin.lastActionCtx.Config.Sandstorm.DownloadURL != "https://downloads.example.test/sandstorm-0-fast-1.tar.xz" {
		t.Fatalf("unexpected runtime config: %+v", plugin.lastActionCtx.Config)
	}
}

func TestVMUpDoesNotProvision(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	var upCalls int
	var provisionCalls int
	plugin := &fakePlugin{
		name:           domain.ProviderLima,
		detectInstance: "sandstorm-app-1234",
		statusResult: providers.Status{
			Provider:     domain.ProviderLima,
			InstanceName: "sandstorm-app-1234",
			State:        "unknown",
		},
		upHook: func(project providers.ProjectContext) error {
			upCalls++
			return nil
		},
		provisionHook: func(project providers.ProjectContext) error {
			provisionCalls++
			return nil
		},
	}
	svc := newService(t, plugin, home)

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "lemp", false); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.VMUp(context.Background(), workDir, "", 0); err != nil {
		t.Fatal(err)
	}
	if upCalls != 1 || provisionCalls != 0 {
		t.Fatalf("expected vm up to call up only, got up=%d provision=%d", upCalls, provisionCalls)
	}
}

func TestVMUpPersistsSandstormPortOverrideAndRefreshesRuntimeEnv(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	var upCalls int
	plugin := &fakePlugin{
		name:           domain.ProviderLima,
		detectInstance: "sandstorm-app-1234",
		statusResult: providers.Status{
			Provider:     domain.ProviderLima,
			InstanceName: "sandstorm-app-1234",
			State:        "unknown",
		},
		upHook: func(project providers.ProjectContext) error {
			upCalls++
			return nil
		},
	}
	svc := newService(t, plugin, home)

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "lemp", false); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.VMUp(context.Background(), workDir, "", 7000); err != nil {
		t.Fatal(err)
	}
	if upCalls != 1 {
		t.Fatalf("expected vm up to call up once, got %d", upCalls)
	}

	localData, err := os.ReadFile(filepath.Join(workDir, ".sandstorm", config.LocalFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(localData), "guest_port = 7000") || !strings.Contains(string(localData), "external_port = 7000") {
		t.Fatalf("expected local config to persist both ports, got:\n%s", string(localData))
	}

	runtimeEnv, err := os.ReadFile(filepath.Join(workDir, ".sandstorm", ".generated", "runtime.env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(runtimeEnv), "SANDSTORM_EXTERNAL_PORT=7000") {
		t.Fatalf("expected runtime.env to include updated external port, got:\n%s", string(runtimeEnv))
	}
	if plugin.lastActionCtx.Config == nil || plugin.lastActionCtx.Config.Network.Sandstorm.GuestPort != 7000 || plugin.lastActionCtx.Config.Network.Sandstorm.ExternalPort != 7000 {
		t.Fatalf("unexpected vm up config: %+v", plugin.lastActionCtx.Config)
	}
}

func TestVMUpWithPortReturnsConflictWhenInstanceAlreadyExists(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	var upCalls int
	plugin := &fakePlugin{
		name:           domain.ProviderLima,
		detectInstance: "sandstorm-app-1234",
		statusResult: providers.Status{
			Provider:     domain.ProviderLima,
			InstanceName: "sandstorm-app-1234",
			State:        "running",
		},
		upHook: func(project providers.ProjectContext) error {
			upCalls++
			return nil
		},
	}
	svc := newService(t, plugin, home)

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "lemp", false); err != nil {
		t.Fatal(err)
	}

	_, err := svc.VMUp(context.Background(), workDir, "", 7000)
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != domain.ErrConflict {
		t.Fatalf("expected conflict error, got %v", err)
	}
	if upCalls != 0 {
		t.Fatalf("expected vm up not to be called, got %d", upCalls)
	}

	localData, err := os.ReadFile(filepath.Join(workDir, ".sandstorm", config.LocalFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(localData), "guest_port = 7000") || strings.Contains(string(localData), "external_port = 7000") {
		t.Fatalf("expected local config to remain unchanged, got:\n%s", string(localData))
	}
}

func TestVMCreateReturnsConflictWhenInstanceAlreadyExists(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	var upCalls int
	var provisionCalls int
	plugin := &fakePlugin{
		name:           domain.ProviderLima,
		detectInstance: "sandstorm-app-1234",
		statusResult: providers.Status{
			Provider:     domain.ProviderLima,
			InstanceName: "sandstorm-app-1234",
			State:        "stopped",
		},
		upHook: func(project providers.ProjectContext) error {
			upCalls++
			return nil
		},
		provisionHook: func(project providers.ProjectContext) error {
			provisionCalls++
			return nil
		},
	}
	svc := newService(t, plugin, home)

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "lemp", false); err != nil {
		t.Fatal(err)
	}

	_, err := svc.VMCreate(context.Background(), workDir, "", "")
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != domain.ErrConflict {
		t.Fatalf("expected conflict error, got %v", err)
	}
	if upCalls != 0 || provisionCalls != 0 {
		t.Fatalf("expected vm create conflict before up/provision, got up=%d provision=%d", upCalls, provisionCalls)
	}
}

func TestVMStatusUsesProviderOverrideInResolvedContext(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	vagrantPlugin := &fakePlugin{
		name:           domain.ProviderVagrant,
		detectInstance: "vagrant-app",
		statusResult: providers.Status{
			Provider:     domain.ProviderVagrant,
			InstanceName: "vagrant-app",
			State:        "running",
		},
	}
	svc := newService(t, vagrantPlugin, home)

	if err := os.MkdirAll(filepath.Join(workDir, ".sandstorm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".sandstorm", config.ProjectFile), config.InitialProject("lemp"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".sandstorm", config.LocalFile), config.InitialLocal(domain.ProviderLima), 0o644); err != nil {
		t.Fatal(err)
	}

	status, err := svc.VMStatus(context.Background(), workDir, domain.ProviderVagrant)
	if err != nil {
		t.Fatal(err)
	}
	if status.Provider != domain.ProviderVagrant || status.InstanceName != "vagrant-app" {
		t.Fatalf("unexpected status: %+v", status)
	}
	if vagrantPlugin.lastActionCtx.Config == nil || vagrantPlugin.lastActionCtx.Config.Provider != domain.ProviderVagrant {
		t.Fatalf("expected provider override to reach status context: %+v", vagrantPlugin.lastActionCtx.Config)
	}
}

func TestPackIncludesGuestCommandStderrInWorkflowError(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{
		name:           domain.ProviderLima,
		detectInstance: "sandstorm-app-1234",
		execErr: &runner.CommandError{
			Result: runner.Result{
				Command:  "limactl shell --workdir /opt/app sandstorm-app-1234 bash -lc ...",
				ExitCode: 1,
				Stderr:   "bad alwaysInclude paths\nCap'n Proto parse error",
			},
			Err: &exec.ExitError{},
		},
	}
	svc := newService(t, plugin, home)

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "node", false); err != nil {
		t.Fatal(err)
	}

	_, err := svc.Pack(context.Background(), workDir, filepath.Join(workDir, "out.spk"), "")
	if err == nil {
		t.Fatal("expected pack error")
	}
	if !strings.Contains(err.Error(), "bad alwaysInclude paths") {
		t.Fatalf("expected guest stderr in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "Cap'n Proto parse error") {
		t.Fatalf("expected guest stderr details in error, got %v", err)
	}
}

func TestVMStatusWrapsProviderLookupErrorsWithSandboxHint(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{
		name:           domain.ProviderLima,
		detectInstance: "sandstorm-app-1234",
		statusErr:      errors.New("permission denied"),
	}
	svc := newService(t, plugin, home)

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "node", false); err != nil {
		t.Fatal(err)
	}

	_, err := svc.VMStatus(context.Background(), workDir, "")
	if err == nil {
		t.Fatal("expected vm status error")
	}
	if !strings.Contains(err.Error(), "rerun elevated") {
		t.Fatalf("expected sandbox hint in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "provider status lookup failed") {
		t.Fatalf("expected status lookup context in error, got %v", err)
	}
}

func TestVMSSHForwardsArgsAndResolvedContext(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{
		name:           domain.ProviderVagrant,
		detectInstance: "vagrant-app",
	}
	svc := newService(t, plugin, home)

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderVagrant, "lemp", false); err != nil {
		t.Fatal(err)
	}

	st, err := svc.VMSSH(context.Background(), workDir, []string{"-c", "pwd"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if st.Provider != domain.ProviderVagrant || st.Stack != "lemp" {
		t.Fatalf("unexpected returned state: %+v", st)
	}
	if len(plugin.lastSSHArgs) != 2 || plugin.lastSSHArgs[0] != "-c" || plugin.lastSSHArgs[1] != "pwd" {
		t.Fatalf("unexpected ssh args: %#v", plugin.lastSSHArgs)
	}
	if plugin.lastActionCtx.Config == nil || plugin.lastActionCtx.Config.Provider != domain.ProviderVagrant {
		t.Fatalf("unexpected ssh context: %+v", plugin.lastActionCtx.Config)
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

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "lemp", false); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Pack(context.Background(), workDir, outputPath, ""); err != nil {
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

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderVagrant, "lemp", false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(input, []byte("verify-me"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Verify(context.Background(), workDir, input, ""); err != nil {
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

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "lemp", false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(input, []byte("publish-me"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Publish(context.Background(), workDir, input, ""); err != nil {
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

func TestVerifySkipsSelfCopyWhenPackageIsAlreadyStaged(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{
		name:           domain.ProviderVagrant,
		detectInstance: "app",
	}
	svc := newService(t, plugin, home)

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderVagrant, "lemp", false); err != nil {
		t.Fatal(err)
	}

	input := filepath.Join(workDir, ".sandstorm", "input.spk")
	if err := os.WriteFile(input, []byte("verify-me"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Verify(context.Background(), workDir, input, ""); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "verify-me" {
		t.Fatalf("expected staged file to remain intact, got %q", string(data))
	}
}

func TestPublishSkipsSelfCopyWhenPackageIsAlreadyStaged(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{
		name:           domain.ProviderLima,
		detectInstance: "sandstorm-app",
	}
	svc := newService(t, plugin, home)

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "lemp", false); err != nil {
		t.Fatal(err)
	}

	input := filepath.Join(workDir, ".sandstorm", "publish.spk")
	if err := os.WriteFile(input, []byte("publish-me"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Publish(context.Background(), workDir, input, ""); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "publish-me" {
		t.Fatalf("expected staged file to remain intact, got %q", string(data))
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

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "lemp", false); err != nil {
		t.Fatal(err)
	}
	result, err := svc.Keygen(context.Background(), workDir, []string{"--admin"}, "")
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

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderVagrant, "lemp", false); err != nil {
		t.Fatal(err)
	}
	result, err := svc.ListKeys(context.Background(), workDir, nil, "")
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

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "lemp", false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetKey(context.Background(), workDir, "", ""); err == nil {
		t.Fatal("expected missing key id error")
	}
	result, err := svc.GetKey(context.Background(), workDir, "kid123", "")
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

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "lemp", false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EnterGrain(context.Background(), workDir, "", false); err != nil {
		t.Fatal(err)
	}
	if plugin.attached == nil || plugin.attached.GrainID != "grain123" {
		t.Fatalf("unexpected attached grain: %+v", plugin.attached)
	}
	if plugin.attachChecksum == "" {
		t.Fatal("expected checksum to be passed to provider")
	}
}

func TestEnterGrainNoninteractiveFailsWhenMultipleGrainsExist(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{
		name:           domain.ProviderLima,
		detectInstance: "sandstorm-app",
		grains: []providers.Grain{
			{SupervisorPID: "100", GrainID: "grain123", ChildPID: 200},
			{SupervisorPID: "101", GrainID: "grain456", ChildPID: 201},
		},
	}
	svc := newService(t, plugin, home)

	if _, err := svc.SetupVM(context.Background(), workDir, domain.ProviderLima, "lemp", false); err != nil {
		t.Fatal(err)
	}

	_, err := svc.EnterGrain(context.Background(), workDir, "", true)
	if err == nil {
		t.Fatal("expected noninteractive mode to reject multiple grains")
	}
	if plugin.attached != nil {
		t.Fatalf("did not expect grain attachment, got %+v", plugin.attached)
	}
}

func TestListKeysDoesNotFallbackToLegacyVagrantLayout(t *testing.T) {
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

	_, err := svc.ListKeys(context.Background(), workDir, nil, "")
	if err == nil {
		t.Fatal("expected missing config error")
	}
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("expected domain error, got %T", err)
	}
	if domainErr.Code != domain.ErrNotFound {
		t.Fatalf("unexpected error: %+v", domainErr)
	}
}

func TestUpgradeVMMigratesLegacyLimaLayout(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	home := t.TempDir()
	plugin := &fakePlugin{
		name:           domain.ProviderLima,
		detectInstance: "legacy-lima-instance",
		bootstrapFiles: []providers.RenderedFile{{
			Path: filepath.Join(".sandstorm", ".generated", "lima.yaml"),
			Body: []byte("mounts: []\n"),
			Mode: 0o644,
		}},
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

	st, err := svc.UpgradeVM(context.Background(), workDir, "")
	if err != nil {
		t.Fatal(err)
	}
	if st.Provider != domain.ProviderLima || st.Stack != "node" || st.VMInstance != "legacy-lima-instance" {
		t.Fatalf("unexpected migrated state: %+v", st)
	}

	projectData, err := os.ReadFile(filepath.Join(workDir, ".sandstorm", config.ProjectFile))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(projectData); !strings.Contains(got, `stack = "node"`) {
		t.Fatalf("unexpected project config: %q", got)
	}

	localData, err := os.ReadFile(filepath.Join(workDir, ".sandstorm", config.LocalFile))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(localData); !strings.Contains(got, `provider = "lima"`) {
		t.Fatalf("unexpected local config: %q", got)
	}
}
