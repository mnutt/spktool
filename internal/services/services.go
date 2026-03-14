package services

import (
	"bufio"
	"context"
	"crypto/sha1"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/mnutt/spktool/internal/domain"
	"github.com/mnutt/spktool/internal/keys"
	"github.com/mnutt/spktool/internal/providers"
	"github.com/mnutt/spktool/internal/runner"
	"github.com/mnutt/spktool/internal/state"
	"github.com/mnutt/spktool/internal/templates"
	"github.com/mnutt/spktool/internal/workflow"
)

const ToolVersion = "0.1.0"

type ProjectService struct {
	logger    *slog.Logger
	templates *templates.Repository
	providers *providers.Registry
	state     *state.Store
	keyring   keys.Manager
}

func NewProjectService(logger *slog.Logger, repo *templates.Repository, registry *providers.Registry, store *state.Store, keyring keys.Manager) *ProjectService {
	return &ProjectService{
		logger:    logger,
		templates: repo,
		providers: registry,
		state:     store,
		keyring:   keyring,
	}
}

func (s *ProjectService) SetupVM(ctx context.Context, workDir string, providerName domain.ProviderName, stack string) (*domain.ProjectState, error) {
	plugin, err := s.providers.Plugin(providerName)
	if err != nil {
		return nil, err
	}
	if stack == "" {
		return nil, &domain.Error{Code: domain.ErrInvalidArgument, Op: "services.SetupVM", Message: "stack is required"}
	}
	if _, err := s.templates.StackFile(stack, "setup.sh"); err != nil {
		return nil, &domain.Error{Code: domain.ErrNotFound, Op: "services.SetupVM", Message: fmt.Sprintf("unknown stack %q", stack), Cause: err}
	}

	project := providers.ProjectContext{WorkDir: workDir}
	stateFile := &domain.ProjectState{
		Provider:    providerName,
		VMInstance:  plugin.DetectInstanceName(workDir),
		Stack:       stack,
		ToolVersion: ToolVersion,
		Migration:   1,
	}

	globalSetup, err := s.templates.BoxFile("global-setup.sh")
	if err != nil {
		return nil, err
	}
	emptyBuild, err := s.templates.BoxFile("empty-build.sh")
	if err != nil {
		return nil, err
	}
	setup, err := s.templates.StackFile(stack, "setup.sh")
	if err != nil {
		return nil, err
	}
	launcher, err := s.templates.StackFile(stack, "launcher.sh")
	if err != nil {
		return nil, err
	}
	build, err := s.templates.StackFile(stack, "build.sh")
	if err != nil {
		build = emptyBuild
	}
	gitIgnore, err := s.templates.BoxFile("gitignore")
	if err != nil {
		return nil, err
	}
	gitAttributes, err := s.templates.BoxFile("gitattributes")
	if err != nil {
		return nil, err
	}
	bootstrapFiles, err := plugin.BootstrapFiles(project)
	if err != nil {
		return nil, err
	}

	files := []providers.RenderedFile{
		{Path: filepath.Join(".sandstorm", "global-setup.sh"), Body: globalSetup, Mode: 0o755},
		{Path: filepath.Join(".sandstorm", "setup.sh"), Body: setup, Mode: 0o755},
		{Path: filepath.Join(".sandstorm", "build.sh"), Body: build, Mode: 0o755},
		{Path: filepath.Join(".sandstorm", "launcher.sh"), Body: launcher, Mode: 0o755},
		{Path: filepath.Join(".sandstorm", ".gitignore"), Body: gitIgnore, Mode: 0o644},
		{Path: filepath.Join(".sandstorm", ".gitattributes"), Body: gitAttributes, Mode: 0o644},
		{Path: filepath.Join(".sandstorm", "stack"), Body: []byte(stack + "\n"), Mode: 0o644},
	}
	files = append(files, bootstrapFiles...)

	err = workflow.Run(ctx, "setupvm", []workflow.Step{
		{
			Name: "ensure-host-keyring",
			Do:   s.keyring.EnsureLayout,
		},
		{
			Name: "ensure-project-directory",
			Do: func(context.Context) error {
				return os.MkdirAll(filepath.Join(workDir, ".sandstorm"), 0o755)
			},
		},
		{
			Name: "write-project-assets",
			Do: func(context.Context) error {
				for _, file := range files {
					full := filepath.Join(workDir, file.Path)
					if err := os.WriteFile(full, file.Body, os.FileMode(file.Mode)); err != nil {
						return domain.Wrap(domain.ErrExternal, "services.SetupVM", "write project asset", err)
					}
				}
				return nil
			},
		},
		{
			Name: "save-project-state",
			Do: func(context.Context) error {
				return s.state.Save(ctx, workDir, stateFile)
			},
		},
	})
	if err != nil {
		return nil, err
	}

	s.logger.Info("project initialized", "provider", providerName, "stack", stack, "workdir", workDir)
	return stateFile, nil
}

func (s *ProjectService) UpgradeVM(ctx context.Context, workDir string) (*domain.ProjectState, error) {
	projectState, plugin, err := s.loadProject(ctx, workDir)
	if err != nil {
		return nil, err
	}
	bootstrapFiles, err := plugin.BootstrapFiles(providers.ProjectContext{WorkDir: workDir, State: projectState})
	if err != nil {
		return nil, err
	}
	globalSetup, err := s.templates.BoxFile("global-setup.sh")
	if err != nil {
		return nil, err
	}

	files := []providers.RenderedFile{
		{Path: filepath.Join(".sandstorm", "global-setup.sh"), Body: globalSetup, Mode: 0o755},
	}
	files = append(files, bootstrapFiles...)

	if err := workflow.Run(ctx, "upgradevm", []workflow.Step{{
		Name: "refresh-provider-assets",
		Do: func(context.Context) error {
			for _, file := range files {
				full := filepath.Join(workDir, file.Path)
				if err := os.WriteFile(full, file.Body, os.FileMode(file.Mode)); err != nil {
					return err
				}
			}
			return nil
		},
	}, {
		Name: "save-project-state",
		Do: func(context.Context) error {
			projectState.ToolVersion = ToolVersion
			projectState.Migration++
			return s.state.Save(ctx, workDir, projectState)
		},
	}}); err != nil {
		return nil, err
	}
	return projectState, nil
}

func (s *ProjectService) Init(ctx context.Context, workDir string) (*domain.ProjectState, error) {
	projectState, plugin, err := s.loadProject(ctx, workDir)
	if err != nil {
		return nil, err
	}
	initArgs := s.initArgs(projectState.Stack)
	command := []string{
		"spk", "init", "-p", "8000",
		"--keyring=/host-dot-sandstorm/sandstorm-keyring",
		"--output=/opt/app/.sandstorm/sandstorm-pkgdef.capnp",
	}
	if initArgs != "" {
		command = append(command, strings.Fields(initArgs)...)
	}
	command = append(command, "--", "/bin/bash", "/opt/app/.sandstorm/launcher.sh")
	if _, err := plugin.Exec(ctx, providers.ProjectContext{WorkDir: workDir, State: projectState}, command); err != nil {
		return nil, err
	}
	return projectState, nil
}

func (s *ProjectService) Dev(ctx context.Context, workDir string) (*domain.ProjectState, error) {
	projectState, plugin, err := s.loadProject(ctx, workDir)
	if err != nil {
		return nil, err
	}

	helperDir := filepath.ToSlash(filepath.Join("/tmp", string(projectState.Provider)+"-spk-devhelpers"))
	tailerBody, err := s.templates.HelperFile("grain-log-tailer.sh")
	if err != nil {
		return nil, err
	}
	wrapperBody, err := s.templates.HelperFile("dev-with-tail.sh")
	if err != nil {
		return nil, err
	}

	project := providers.ProjectContext{WorkDir: workDir, State: projectState}
	err = workflow.Run(ctx, "dev", []workflow.Step{
		{
			Name: "upload-grain-log-tailer",
			Do: func(context.Context) error {
				return plugin.WriteFile(ctx, project, providers.RenderedFile{
					Path: filepath.ToSlash(filepath.Join(helperDir, "grain-log-tailer.sh")),
					Body: tailerBody,
					Mode: 0o755,
				})
			},
		},
		{
			Name: "upload-dev-wrapper",
			Do: func(context.Context) error {
				return plugin.WriteFile(ctx, project, providers.RenderedFile{
					Path: filepath.ToSlash(filepath.Join(helperDir, "dev-with-tail.sh")),
					Body: wrapperBody,
					Mode: 0o755,
				})
			},
		},
		{
			Name: "start-dev-session",
			Do: func(context.Context) error {
				return plugin.ExecInteractive(ctx, project, s.devCommand(projectState.Provider, filepath.ToSlash(filepath.Join(helperDir, "dev-with-tail.sh"))))
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return projectState, nil
}

func (s *ProjectService) Pack(ctx context.Context, workDir, outputPath string) (*domain.ProjectState, error) {
	projectState, plugin, err := s.loadProject(ctx, workDir)
	if err != nil {
		return nil, err
	}
	if outputPath == "" {
		return nil, &domain.Error{Code: domain.ErrInvalidArgument, Op: "services.Pack", Message: "output path is required"}
	}

	project := providers.ProjectContext{WorkDir: workDir, State: projectState}
	hostArtifact := filepath.Join(workDir, "sandstorm-package.spk")
	guestArtifact := "/tmp/sandstorm-package.spk"
	if projectState.Provider == domain.ProviderVagrant {
		guestArtifact = "/home/vagrant/sandstorm-package.spk"
	}

	err = workflow.Run(ctx, "pack", []workflow.Step{
		{
			Name: "remove-stale-host-artifact",
			Do: func(context.Context) error {
				if err := os.Remove(hostArtifact); err != nil && !os.IsNotExist(err) {
					return domain.Wrap(domain.ErrExternal, "services.Pack", "remove stale host artifact", err)
				}
				return nil
			},
		},
		{
			Name: "build-package-in-guest",
			Do: func(context.Context) error {
				command := []string{
					"cd", "/opt/app/.sandstorm/",
					"&&", "spk", "pack",
					"--keyring=/host-dot-sandstorm/sandstorm-keyring",
					"--pkg-def=/opt/app/.sandstorm/sandstorm-pkgdef.capnp:pkgdef",
					guestArtifact,
					"&&", "spk", "verify", "--details", guestArtifact,
					"&&", "mv", guestArtifact, "/opt/app/sandstorm-package.spk",
				}
				_, err := plugin.Exec(ctx, project, command)
				return err
			},
		},
		{
			Name: "move-package-to-output",
			Do: func(context.Context) error {
				return moveFile(hostArtifact, outputPath)
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return projectState, nil
}

func (s *ProjectService) Verify(ctx context.Context, workDir, spkPath string) (*domain.ProjectState, error) {
	projectState, plugin, err := s.loadProject(ctx, workDir)
	if err != nil {
		return nil, err
	}
	if spkPath == "" {
		return nil, &domain.Error{Code: domain.ErrInvalidArgument, Op: "services.Verify", Message: "spk path is required"}
	}

	project := providers.ProjectContext{WorkDir: workDir, State: projectState}
	stagedName := filepath.Base(spkPath)
	stagedHostPath := filepath.Join(workDir, ".sandstorm", stagedName)
	stagedGuestPath := filepath.ToSlash(filepath.Join("/opt/app/.sandstorm", stagedName))

	err = workflow.Run(ctx, "verify", []workflow.Step{
		{
			Name: "stage-package-on-host",
			Do: func(context.Context) error {
				return copyFile(spkPath, stagedHostPath)
			},
			Rollback: func(context.Context) error {
				if err := os.Remove(stagedHostPath); err != nil && !os.IsNotExist(err) {
					return err
				}
				return nil
			},
		},
		{
			Name: "verify-package-in-guest",
			Do: func(context.Context) error {
				_, err := plugin.Exec(ctx, project, []string{"spk", "verify", "--details", stagedGuestPath})
				return err
			},
			Rollback: func(context.Context) error {
				if err := os.Remove(stagedHostPath); err != nil && !os.IsNotExist(err) {
					return err
				}
				return nil
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if err := os.Remove(stagedHostPath); err != nil && !os.IsNotExist(err) {
		return nil, domain.Wrap(domain.ErrExternal, "services.Verify", "remove staged package", err)
	}
	return projectState, nil
}

func (s *ProjectService) Publish(ctx context.Context, workDir, spkPath string) (*domain.ProjectState, error) {
	projectState, plugin, err := s.loadProject(ctx, workDir)
	if err != nil {
		return nil, err
	}
	if spkPath == "" {
		return nil, &domain.Error{Code: domain.ErrInvalidArgument, Op: "services.Publish", Message: "spk path is required"}
	}

	project := providers.ProjectContext{WorkDir: workDir, State: projectState}
	stagedName := filepath.Base(spkPath)
	stagedHostPath := filepath.Join(workDir, ".sandstorm", stagedName)
	stagedGuestPath := filepath.ToSlash(filepath.Join("/opt/app/.sandstorm", stagedName))

	err = workflow.Run(ctx, "publish", []workflow.Step{
		{
			Name: "stage-package-on-host",
			Do: func(context.Context) error {
				return copyFile(spkPath, stagedHostPath)
			},
			Rollback: func(context.Context) error {
				if err := os.Remove(stagedHostPath); err != nil && !os.IsNotExist(err) {
					return err
				}
				return nil
			},
		},
		{
			Name: "publish-package-in-guest",
			Do: func(context.Context) error {
				_, err := plugin.Exec(ctx, project, []string{
					"spk", "publish",
					"--keyring=/host-dot-sandstorm/sandstorm-keyring",
					stagedGuestPath,
				})
				return err
			},
			Rollback: func(context.Context) error {
				if err := os.Remove(stagedHostPath); err != nil && !os.IsNotExist(err) {
					return err
				}
				return nil
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if err := os.Remove(stagedHostPath); err != nil && !os.IsNotExist(err) {
		return nil, domain.Wrap(domain.ErrExternal, "services.Publish", "remove staged package", err)
	}
	return projectState, nil
}

func (s *ProjectService) EnterGrain(ctx context.Context, workDir string) (*domain.ProjectState, error) {
	projectState, plugin, err := s.loadProject(ctx, workDir)
	if err != nil {
		return nil, err
	}
	helper, err := s.templates.HelperFile("enter_grain")
	if err != nil {
		return nil, err
	}
	checksumFile, err := s.templates.HelperFile("enter_grain.sha1")
	if err != nil {
		return nil, err
	}
	desiredChecksum := ""
	if fields := strings.Fields(string(checksumFile)); len(fields) > 0 {
		desiredChecksum = strings.TrimSpace(fields[0])
	}
	if desiredChecksum != "" {
		found := fmt.Sprintf("%x", sha1.Sum(helper))
		if found != desiredChecksum {
			return nil, &domain.Error{Code: domain.ErrConflict, Op: "services.EnterGrain", Message: "embedded enter_grain helper checksum mismatch"}
		}
	}
	project := providers.ProjectContext{WorkDir: workDir, State: projectState}
	grains, err := plugin.ListGrains(ctx, project)
	if err != nil {
		return nil, err
	}
	chosen, err := chooseGrain(grains)
	if err != nil {
		return nil, err
	}
	if err := plugin.AttachGrain(ctx, project, chosen, helper, desiredChecksum); err != nil {
		return nil, err
	}
	return projectState, nil
}

func (s *ProjectService) Keygen(ctx context.Context, workDir string, args []string) (runner.Result, error) {
	return s.runKeyCommand(ctx, workDir, append([]string{"spk", "keygen", "--keyring=/host-dot-sandstorm/sandstorm-keyring"}, args...))
}

func (s *ProjectService) ListKeys(ctx context.Context, workDir string, args []string) (runner.Result, error) {
	return s.runKeyCommand(ctx, workDir, append([]string{"spk", "listkeys", "--keyring=/host-dot-sandstorm/sandstorm-keyring"}, args...))
}

func (s *ProjectService) GetKey(ctx context.Context, workDir, keyID string) (runner.Result, error) {
	if keyID == "" {
		return runner.Result{}, &domain.Error{Code: domain.ErrInvalidArgument, Op: "services.GetKey", Message: "key id is required"}
	}
	return s.runKeyCommand(ctx, workDir, []string{"spk", "getkey", "--keyring=/host-dot-sandstorm/sandstorm-keyring", keyID})
}

func (s *ProjectService) VMUp(ctx context.Context, workDir string) (*domain.ProjectState, error) {
	projectState, plugin, err := s.loadProject(ctx, workDir)
	if err != nil {
		return nil, err
	}
	if err := plugin.Up(ctx, providers.ProjectContext{WorkDir: workDir, State: projectState}); err != nil {
		return nil, err
	}
	return projectState, nil
}

func (s *ProjectService) devCommand(provider domain.ProviderName, wrapperPath string) []string {
	buildCmd := "/opt/app/.sandstorm/build.sh && cd /opt/app/.sandstorm && spk dev --pkg-def=/opt/app/.sandstorm/sandstorm-pkgdef.capnp:pkgdef"
	switch provider {
	case domain.ProviderLima:
		return []string{"bash", wrapperPath, "--", "sg", "sandstorm", "-c", buildCmd}
	default:
		return []string{"bash", wrapperPath, "--", "bash", "-lc", buildCmd}
	}
}

func (s *ProjectService) VMHalt(ctx context.Context, workDir string) (*domain.ProjectState, error) {
	projectState, plugin, err := s.loadProject(ctx, workDir)
	if err != nil {
		return nil, err
	}
	if err := plugin.Halt(ctx, providers.ProjectContext{WorkDir: workDir, State: projectState}); err != nil {
		return nil, err
	}
	return projectState, nil
}

func (s *ProjectService) VMDestroy(ctx context.Context, workDir string) (*domain.ProjectState, error) {
	projectState, plugin, err := s.loadProject(ctx, workDir)
	if err != nil {
		return nil, err
	}
	if err := plugin.Destroy(ctx, providers.ProjectContext{WorkDir: workDir, State: projectState}); err != nil {
		return nil, err
	}
	return projectState, nil
}

func (s *ProjectService) VMStatus(ctx context.Context, workDir string) (providers.Status, error) {
	projectState, plugin, err := s.loadProject(ctx, workDir)
	if err != nil {
		return providers.Status{}, err
	}
	return plugin.Status(ctx, providers.ProjectContext{WorkDir: workDir, State: projectState})
}

func (s *ProjectService) VMProvision(ctx context.Context, workDir string) (*domain.ProjectState, error) {
	projectState, plugin, err := s.loadProject(ctx, workDir)
	if err != nil {
		return nil, err
	}
	if err := plugin.Provision(ctx, providers.ProjectContext{WorkDir: workDir, State: projectState}); err != nil {
		return nil, err
	}
	return projectState, nil
}

func (s *ProjectService) VMSSH(ctx context.Context, workDir string, args []string) (*domain.ProjectState, error) {
	projectState, plugin, err := s.loadProject(ctx, workDir)
	if err != nil {
		return nil, err
	}
	if err := plugin.SSH(ctx, providers.ProjectContext{WorkDir: workDir, State: projectState}, args); err != nil {
		return nil, err
	}
	return projectState, nil
}

func (s *ProjectService) StackNames() ([]string, error) {
	return s.templates.StackNames()
}

func (s *ProjectService) loadProject(ctx context.Context, workDir string) (*domain.ProjectState, providers.Plugin, error) {
	projectState, err := s.state.Load(ctx, workDir)
	if err != nil {
		return nil, nil, err
	}
	if projectState == nil {
		projectState, err = s.inferLegacyProject(ctx, workDir)
		if err != nil {
			return nil, nil, err
		}
	}
	plugin, err := s.providers.Plugin(projectState.Provider)
	if err != nil {
		return nil, nil, err
	}
	return projectState, plugin, nil
}

func (s *ProjectService) inferLegacyProject(ctx context.Context, workDir string) (*domain.ProjectState, error) {
	sandstormDir := filepath.Join(workDir, ".sandstorm")
	if stat, err := os.Stat(sandstormDir); err != nil || !stat.IsDir() {
		return nil, &domain.Error{
			Code:    domain.ErrNotFound,
			Op:      "services.inferLegacyProject",
			Message: "no .sandstorm project state found; run setupvm first",
			Cause:   err,
		}
	}

	stackData, err := os.ReadFile(filepath.Join(sandstormDir, "stack"))
	if err != nil {
		return nil, &domain.Error{
			Code:    domain.ErrNotFound,
			Op:      "services.inferLegacyProject",
			Message: "legacy .sandstorm project is missing stack metadata",
			Cause:   err,
		}
	}
	stack := strings.TrimSpace(string(stackData))
	if stack == "" {
		return nil, &domain.Error{
			Code:    domain.ErrNotFound,
			Op:      "services.inferLegacyProject",
			Message: "legacy .sandstorm project has an empty stack marker",
		}
	}

	providerName := inferProviderFromLegacyFiles(sandstormDir)
	if providerName == "" {
		return nil, &domain.Error{
			Code:    domain.ErrNotFound,
			Op:      "services.inferLegacyProject",
			Message: "legacy .sandstorm project is missing provider metadata",
		}
	}
	plugin, err := s.providers.Plugin(providerName)
	if err != nil {
		return nil, err
	}

	projectState := &domain.ProjectState{
		Provider:    providerName,
		VMInstance:  plugin.DetectInstanceName(workDir),
		Stack:       stack,
		ToolVersion: ToolVersion,
		Migration:   1,
	}
	if err := s.state.Save(ctx, workDir, projectState); err != nil {
		return nil, err
	}
	s.logger.Info("migrated legacy project state", "provider", providerName, "stack", stack, "workdir", workDir)
	return projectState, nil
}

func (s *ProjectService) runKeyCommand(ctx context.Context, workDir string, command []string) (runner.Result, error) {
	projectState, plugin, err := s.loadProject(ctx, workDir)
	if err != nil {
		return runner.Result{}, err
	}
	return plugin.Exec(ctx, providers.ProjectContext{WorkDir: workDir, State: projectState}, command)
}

func (s *ProjectService) initArgs(stack string) string {
	data, err := s.templates.StackFile(stack, "initargs")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func copyFile(src, dst string) error {
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

func chooseGrain(grains []providers.Grain) (providers.Grain, error) {
	if len(grains) == 0 {
		return providers.Grain{}, &domain.Error{Code: domain.ErrNotFound, Op: "services.chooseGrain", Message: "no grains available"}
	}
	if len(grains) == 1 {
		return grains[0], nil
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

func inferProviderFromLegacyFiles(sandstormDir string) domain.ProviderName {
	vagrantfile := filepath.Join(sandstormDir, "Vagrantfile")
	limaYAML := filepath.Join(sandstormDir, "lima.yaml")
	if _, err := os.Stat(vagrantfile); err == nil {
		return domain.ProviderVagrant
	}
	if _, err := os.Stat(limaYAML); err == nil {
		return domain.ProviderLima
	}
	return ""
}
