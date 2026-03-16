package services

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/mnutt/spktool/internal/config"
	"github.com/mnutt/spktool/internal/domain"
	"github.com/mnutt/spktool/internal/keys"
	"github.com/mnutt/spktool/internal/providers"
	"github.com/mnutt/spktool/internal/templates"
	"github.com/pelletier/go-toml/v2"
)

const ToolVersion = "0.1.0"

type ConfigRender struct {
	Provider domain.ProviderName `json:"provider"`
	Files    []ConfigRenderFile  `json:"files"`
}

type ConfigRenderFile struct {
	Path string `json:"path"`
	Body string `json:"body"`
}

type serviceDeps struct {
	logger    *slog.Logger
	templates *templates.Repository
	providers *providers.Registry
	keyring   keys.Manager
}

type ProjectBootstrapService struct {
	deps *serviceDeps
}

type PackageService struct {
	deps *serviceDeps
}

type GrainService struct {
	deps *serviceDeps
}

type KeyService struct {
	deps *serviceDeps
}

type VMLifecycleService struct {
	deps *serviceDeps
}

type Services struct {
	ProjectBootstrap *ProjectBootstrapService
	Package          *PackageService
	Grain            *GrainService
	Key              *KeyService
	VM               *VMLifecycleService
}

func NewServices(logger *slog.Logger, repo *templates.Repository, registry *providers.Registry, keyring keys.Manager) *Services {
	deps := &serviceDeps{
		logger:    logger,
		templates: repo,
		providers: registry,
		keyring:   keyring,
	}
	return &Services{
		ProjectBootstrap: &ProjectBootstrapService{deps: deps},
		Package:          &PackageService{deps: deps},
		Grain:            &GrainService{deps: deps},
		Key:              &KeyService{deps: deps},
		VM:               &VMLifecycleService{deps: deps},
	}
}

func (d *serviceDeps) loadResolvedProject(_ context.Context, workDir string, providerOverride domain.ProviderName) (*domain.ProjectState, *config.Resolved, error) {
	resolved, err := config.Load(workDir, providerOverride, "")
	if err != nil {
		return nil, nil, err
	}
	namer, err := d.providers.InstanceNamer(resolved.Provider)
	if err != nil {
		return nil, nil, err
	}
	return summarizeProject(namer, workDir, resolved), resolved, nil
}

func (d *serviceDeps) loadRuntimeProject(ctx context.Context, workDir string, providerOverride domain.ProviderName) (*domain.ProjectState, *config.Resolved, providers.RuntimeProvider, error) {
	projectState, resolved, err := d.loadResolvedProject(ctx, workDir, providerOverride)
	if err != nil {
		return nil, nil, nil, err
	}
	plugin, err := d.providers.RuntimeProvider(resolved.Provider)
	if err != nil {
		return nil, nil, nil, err
	}
	return projectState, resolved, plugin, nil
}

func (d *serviceDeps) loadBootstrapProject(ctx context.Context, workDir string, providerOverride domain.ProviderName) (*domain.ProjectState, *config.Resolved, providers.BootstrapRenderer, error) {
	projectState, resolved, err := d.loadResolvedProject(ctx, workDir, providerOverride)
	if err != nil {
		return nil, nil, nil, err
	}
	plugin, err := d.providers.BootstrapRenderer(resolved.Provider)
	if err != nil {
		return nil, nil, nil, err
	}
	return projectState, resolved, plugin, nil
}

func (d *serviceDeps) loadGrainProject(ctx context.Context, workDir string, providerOverride domain.ProviderName) (*domain.ProjectState, *config.Resolved, providers.GrainManager, error) {
	projectState, resolved, err := d.loadResolvedProject(ctx, workDir, providerOverride)
	if err != nil {
		return nil, nil, nil, err
	}
	plugin, err := d.providers.GrainManager(resolved.Provider)
	if err != nil {
		return nil, nil, nil, err
	}
	return projectState, resolved, plugin, nil
}

func (d *serviceDeps) migrateLegacyProject(workDir string, providerOverride domain.ProviderName) error {
	projectState, err := legacyProjectState(workDir, providerOverride)
	if err != nil {
		return err
	}
	files := []providers.RenderedFile{{
		Path: filepath.Join(".sandstorm", config.ProjectFile),
		Body: config.InitialProject(projectState.Stack),
		Mode: 0o644,
	}}
	localPath := filepath.Join(workDir, ".sandstorm", config.LocalFile)
	if _, err := os.Stat(localPath); err == nil {
		return d.writeFiles(workDir, files)
	} else if !errorsIsNotExist(err) {
		return domain.Wrap(domain.ErrExternal, "services.migrateLegacyProject", "read local config", err)
	}
	files = append(files, providers.RenderedFile{
		Path: filepath.Join(".sandstorm", config.LocalFile),
		Body: config.InitialLocal(projectState.Provider),
		Mode: 0o644,
	})
	return d.writeFiles(workDir, files)
}

func legacyProjectState(workDir string, providerOverride domain.ProviderName) (*domain.ProjectState, error) {
	sandstormDir := filepath.Join(workDir, ".sandstorm")
	if stat, err := os.Stat(sandstormDir); err != nil || !stat.IsDir() {
		return nil, &domain.Error{
			Code:    domain.ErrNotFound,
			Op:      "services.legacyProjectState",
			Message: "no .sandstorm project config found; run setupvm first",
			Cause:   err,
		}
	}

	stackData, err := os.ReadFile(filepath.Join(sandstormDir, "stack"))
	if err != nil {
		return nil, &domain.Error{
			Code:    domain.ErrNotFound,
			Op:      "services.legacyProjectState",
			Message: "legacy .sandstorm project is missing stack metadata",
			Cause:   err,
		}
	}
	stack := strings.TrimSpace(string(stackData))
	if stack == "" {
		return nil, &domain.Error{
			Code:    domain.ErrNotFound,
			Op:      "services.legacyProjectState",
			Message: "legacy .sandstorm project has an empty stack marker",
		}
	}

	providerName := inferProviderFromLegacyFiles(sandstormDir)
	if providerOverride != "" {
		providerName = providerOverride
	}
	if providerName == "" {
		return nil, &domain.Error{
			Code:    domain.ErrNotFound,
			Op:      "services.legacyProjectState",
			Message: "legacy .sandstorm project is missing provider metadata",
		}
	}
	return &domain.ProjectState{
		Provider:    providerName,
		Stack:       stack,
		ToolVersion: ToolVersion,
	}, nil
}

func (d *serviceDeps) generatedFiles(workDir string, projectState *domain.ProjectState, resolved *config.Resolved, plugin providers.BootstrapRenderer) ([]providers.RenderedFile, error) {
	globalSetup, err := d.templates.BoxFile("global-setup.sh")
	if err != nil {
		return nil, err
	}
	gitIgnore, err := d.templates.BoxFile("gitignore")
	if err != nil {
		return nil, err
	}
	gitAttributes, err := d.templates.BoxFile("gitattributes")
	if err != nil {
		return nil, err
	}
	bootstrapFiles, err := plugin.BootstrapFiles(d.projectContext(workDir, projectState, resolved))
	if err != nil {
		return nil, err
	}

	files := []providers.RenderedFile{
		{Path: filepath.Join(".sandstorm", "global-setup.sh"), Body: globalSetup, Mode: 0o755},
		{Path: filepath.Join(".sandstorm", ".gitignore"), Body: gitIgnore, Mode: 0o644},
		{Path: filepath.Join(".sandstorm", ".gitattributes"), Body: gitAttributes, Mode: 0o644},
	}
	if resolved != nil {
		files = append(files, providers.RenderedFile{
			Path: filepath.Join(".sandstorm", ".generated", "runtime.env"),
			Body: renderRuntimeEnv(resolved),
			Mode: 0o644,
		})
	}
	files = append(files, bootstrapFiles...)
	return files, nil
}

func (d *serviceDeps) writeFiles(workDir string, files []providers.RenderedFile) error {
	for _, file := range files {
		full := filepath.Join(workDir, file.Path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return domain.Wrap(domain.ErrExternal, "services.writeFiles", "create generated file directory", err)
		}
		if err := os.WriteFile(full, file.Body, os.FileMode(file.Mode)); err != nil {
			return domain.Wrap(domain.ErrExternal, "services.writeFiles", "write generated file", err)
		}
	}
	return nil
}

func summarizeProject(plugin providers.InstanceNamer, workDir string, resolved *config.Resolved) *domain.ProjectState {
	return &domain.ProjectState{
		Provider:    resolved.Provider,
		VMInstance:  plugin.DetectInstanceName(workDir),
		Stack:       resolved.Stack,
		ToolVersion: ToolVersion,
	}
}

func (d *serviceDeps) projectContext(workDir string, state *domain.ProjectState, resolved *config.Resolved) providers.ProjectContext {
	return providers.ProjectContext{
		WorkDir: workDir,
		State:   state,
		Config:  resolved,
		Verbose: d.logger.Enabled(context.Background(), slog.LevelDebug),
	}
}

func renderRuntimeEnv(resolved *config.Resolved) []byte {
	return []byte(fmt.Sprintf("SANDSTORM_HOST=%s\nSANDSTORM_EXTERNAL_PORT=%d\nSANDSTORM_GUEST_PORT=%d\nSANDSTORM_BASE_URL=http://%s:%d\nSANDSTORM_WILDCARD_HOST=%s\n",
		resolved.Network.Sandstorm.Host,
		resolved.Network.Sandstorm.ExternalPort,
		resolved.Network.Sandstorm.GuestPort,
		resolved.Network.Sandstorm.Host,
		resolved.Network.Sandstorm.ExternalPort,
		config.WildcardHost(resolved.Network.Sandstorm.Host, resolved.Network.Sandstorm.ExternalPort),
	))
}

func copyFile(src, dst string) error {
	same, err := samePath(src, dst)
	if err != nil {
		return err
	}
	if same {
		return nil
	}

	in, err := os.Open(src)
	if err != nil {
		return domain.Wrap(domain.ErrExternal, "services.copyFile", "open source file", err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return domain.Wrap(domain.ErrExternal, "services.copyFile", "create destination directory", err)
	}
	out, err := os.Create(dst)
	if err != nil {
		return domain.Wrap(domain.ErrExternal, "services.copyFile", "create destination file", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return domain.Wrap(domain.ErrExternal, "services.copyFile", "copy file contents", err)
	}
	if err := out.Close(); err != nil {
		return domain.Wrap(domain.ErrExternal, "services.copyFile", "close destination file", err)
	}
	return nil
}

func moveFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return domain.Wrap(domain.ErrExternal, "services.moveFile", "create destination directory", err)
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	if err := os.Remove(src); err != nil {
		return domain.Wrap(domain.ErrExternal, "services.moveFile", "remove source file", err)
	}
	return nil
}

func chooseGrain(grains []providers.Grain, noninteractive bool) (providers.Grain, error) {
	if len(grains) == 0 {
		return providers.Grain{}, &domain.Error{Code: domain.ErrNotFound, Op: "services.chooseGrain", Message: "no grains available"}
	}
	if len(grains) == 1 {
		return grains[0], nil
	}
	if noninteractive {
		return providers.Grain{}, &domain.Error{Code: domain.ErrInvalidArgument, Op: "services.chooseGrain", Message: "multiple grains available; rerun without --noninteractive"}
	}
	fmt.Fprintln(os.Stderr, "Running grains:")
	for i, grain := range grains {
		fmt.Fprintf(os.Stderr, "%d. %s\n", i+1, grain.GrainID)
	}
	fmt.Fprint(os.Stderr, "Choose grain [1]: ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return providers.Grain{}, &domain.Error{Code: domain.ErrExternal, Op: "services.chooseGrain", Message: "read grain selection", Cause: err}
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return grains[0], nil
	}
	var index int
	if _, err := fmt.Sscanf(line, "%d", &index); err != nil || index < 1 || index > len(grains) {
		return providers.Grain{}, &domain.Error{Code: domain.ErrInvalidArgument, Op: "services.chooseGrain", Message: "invalid grain selection"}
	}
	return grains[index-1], nil
}

func samePath(src, dst string) (bool, error) {
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return false, domain.Wrap(domain.ErrExternal, "services.samePath", "resolve source path", err)
	}
	dstAbs, err := filepath.Abs(dst)
	if err != nil {
		return false, domain.Wrap(domain.ErrExternal, "services.samePath", "resolve destination path", err)
	}
	return filepath.Clean(srcAbs) == filepath.Clean(dstAbs), nil
}

func localConfigUsesProvider(path string, provider domain.ProviderName) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, domain.Wrap(domain.ErrExternal, "services.SetupVM", "read existing local config", err)
	}
	var local struct {
		Provider string `toml:"provider"`
	}
	if err := toml.Unmarshal(data, &local); err != nil {
		return false, &domain.Error{Code: domain.ErrInvalidArgument, Op: "services.SetupVM", Message: fmt.Sprintf("parse .sandstorm/%s: %v", config.LocalFile, err)}
	}
	return domain.ProviderName(local.Provider) == provider, nil
}

func inferProviderFromLegacyFiles(sandstormDir string) domain.ProviderName {
	vagrantPaths := []string{
		filepath.Join(sandstormDir, "Vagrantfile"),
		filepath.Join(sandstormDir, ".generated", "Vagrantfile"),
	}
	for _, vagrantfile := range vagrantPaths {
		if _, err := os.Stat(vagrantfile); err == nil {
			return domain.ProviderVagrant
		}
	}
	limaPaths := []string{
		filepath.Join(sandstormDir, "lima.yaml"),
		filepath.Join(sandstormDir, ".generated", "lima.yaml"),
	}
	for _, limaYAML := range limaPaths {
		if _, err := os.Stat(limaYAML); err == nil {
			return domain.ProviderLima
		}
	}
	return ""
}

func errorsIsNotExist(err error) bool {
	return err != nil && os.IsNotExist(err)
}
