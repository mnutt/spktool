package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mnutt/spktool/internal/config"
	"github.com/mnutt/spktool/internal/domain"
	"github.com/mnutt/spktool/internal/providers"
	"github.com/mnutt/spktool/internal/workflow"
)

func (s *ProjectBootstrapService) SetupVM(ctx context.Context, workDir string, providerName domain.ProviderName, stack string, force bool) (*domain.ProjectState, error) {
	if stack == "" {
		return nil, &domain.Error{Code: domain.ErrInvalidArgument, Op: "services.SetupVM", Message: "stack is required"}
	}
	if _, err := s.deps.templates.StackFile(stack, "setup.sh"); err != nil {
		return nil, &domain.Error{Code: domain.ErrNotFound, Op: "services.SetupVM", Message: fmt.Sprintf("unknown stack %q", stack), Cause: err}
	}
	if providerName == "" {
		providerName = config.DetectProvider("")
	}
	if providerName == "" {
		return nil, &domain.Error{Code: domain.ErrNotFound, Op: "services.SetupVM", Message: "provider is not configured and could not be autodetected"}
	}

	localConfigExists := false
	writeLocalConfig := force
	if !force {
		projectPath := filepath.Join(workDir, ".sandstorm", config.ProjectFile)
		if _, err := os.Stat(projectPath); err == nil {
			return nil, &domain.Error{
				Code:    domain.ErrConflict,
				Op:      "services.SetupVM",
				Message: fmt.Sprintf(".sandstorm/%s already exists; rerun with --force to overwrite", config.ProjectFile),
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, domain.Wrap(domain.ErrExternal, "services.SetupVM", "read existing project config", err)
		}
	}

	localPath := filepath.Join(workDir, ".sandstorm", config.LocalFile)
	if _, err := os.Stat(localPath); err == nil {
		localConfigExists = true
		sameProvider, err := localConfigUsesProvider(localPath, providerName)
		if err != nil {
			return nil, err
		}
		writeLocalConfig = !sameProvider
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, domain.Wrap(domain.ErrExternal, "services.SetupVM", "read existing local config", err)
	} else {
		writeLocalConfig = true
	}

	if _, err := s.deps.providers.BootstrapRenderer(providerName); err != nil {
		return nil, err
	}

	globalSetup, err := s.deps.templates.BoxFile("global-setup.sh")
	if err != nil {
		return nil, err
	}
	emptyBuild, err := s.deps.templates.BoxFile("empty-build.sh")
	if err != nil {
		return nil, err
	}
	setup, err := s.deps.templates.StackFile(stack, "setup.sh")
	if err != nil {
		return nil, err
	}
	launcher, err := s.deps.templates.StackFile(stack, "launcher.sh")
	if err != nil {
		return nil, err
	}
	build, err := s.deps.templates.StackFile(stack, "build.sh")
	if err != nil {
		build = emptyBuild
	}
	gitIgnore, err := s.deps.templates.BoxFile("gitignore")
	if err != nil {
		return nil, err
	}
	gitAttributes, err := s.deps.templates.BoxFile("gitattributes")
	if err != nil {
		return nil, err
	}

	projectFiles := []providers.RenderedFile{
		{Path: filepath.Join(".sandstorm", config.ProjectFile), Body: config.InitialProject(stack), Mode: 0o644},
		{Path: filepath.Join(".sandstorm", "global-setup.sh"), Body: globalSetup, Mode: 0o755},
		{Path: filepath.Join(".sandstorm", "setup.sh"), Body: setup, Mode: 0o755},
		{Path: filepath.Join(".sandstorm", "build.sh"), Body: build, Mode: 0o755},
		{Path: filepath.Join(".sandstorm", "launcher.sh"), Body: launcher, Mode: 0o755},
		{Path: filepath.Join(".sandstorm", ".gitignore"), Body: gitIgnore, Mode: 0o644},
		{Path: filepath.Join(".sandstorm", ".gitattributes"), Body: gitAttributes, Mode: 0o644},
	}
	if writeLocalConfig || !localConfigExists {
		projectFiles = append(projectFiles, providers.RenderedFile{
			Path: filepath.Join(".sandstorm", config.LocalFile),
			Body: config.InitialLocal(providerName),
			Mode: 0o644,
		})
	}

	err = workflow.Run(ctx, "setupvm", []workflow.Step{
		{
			Name: "ensure-host-keyring",
			Do:   s.deps.keyring.EnsureLayout,
		},
		{
			Name: "ensure-project-directory",
			Do: func(context.Context) error {
				return os.MkdirAll(filepath.Join(workDir, ".sandstorm"), 0o755)
			},
		},
		{
			Name: "write-project-config",
			Do: func(context.Context) error {
				for _, file := range projectFiles {
					full := filepath.Join(workDir, file.Path)
					if err := os.WriteFile(full, file.Body, os.FileMode(file.Mode)); err != nil {
						return domain.Wrap(domain.ErrExternal, "services.SetupVM", "write project asset", err)
					}
				}
				return nil
			},
		},
	})
	if err != nil {
		return nil, err
	}

	projectState, resolved, plugin, err := s.deps.loadBootstrapProject(ctx, workDir, "")
	if err != nil {
		return nil, err
	}
	files, err := s.deps.generatedFiles(workDir, projectState, resolved, plugin)
	if err != nil {
		return nil, err
	}
	if err := s.deps.writeFiles(workDir, files); err != nil {
		return nil, err
	}

	s.deps.logger.Info("project initialized", "provider", providerName, "stack", stack, "workdir", workDir)
	return projectState, nil
}

func (s *ProjectBootstrapService) UpgradeVM(ctx context.Context, workDir string, providerOverride domain.ProviderName) (*domain.ProjectState, error) {
	projectState, resolved, plugin, err := s.deps.loadBootstrapProject(ctx, workDir, providerOverride)
	if err != nil {
		var configErr *domain.Error
		if !errors.As(err, &configErr) || configErr.Code != domain.ErrNotFound {
			return nil, err
		}
		if err := s.deps.migrateLegacyProject(workDir, providerOverride); err != nil {
			return nil, err
		}
		projectState, resolved, plugin, err = s.deps.loadBootstrapProject(ctx, workDir, providerOverride)
		if err != nil {
			return nil, err
		}
	}
	files, err := s.deps.generatedFiles(workDir, projectState, resolved, plugin)
	if err != nil {
		return nil, err
	}
	if err := s.deps.writeFiles(workDir, files); err != nil {
		return nil, err
	}
	return projectState, nil
}

func (s *ProjectBootstrapService) RenderConfig(ctx context.Context, workDir string, providerOverride domain.ProviderName) (*ConfigRender, error) {
	projectState, resolved, plugin, err := s.deps.loadBootstrapProject(ctx, workDir, providerOverride)
	if err != nil {
		return nil, err
	}
	files, err := s.deps.generatedFiles(workDir, projectState, resolved, plugin)
	if err != nil {
		return nil, err
	}

	rendered := &ConfigRender{
		Provider: projectState.Provider,
		Files:    make([]ConfigRenderFile, 0, len(files)),
	}
	for _, file := range files {
		if !strings.HasPrefix(filepath.ToSlash(file.Path), ".sandstorm/.generated/") {
			continue
		}
		rendered.Files = append(rendered.Files, ConfigRenderFile{
			Path: filepath.ToSlash(file.Path),
			Body: string(file.Body),
		})
	}
	return rendered, nil
}

func (s *ProjectBootstrapService) StackNames() ([]string, error) {
	return s.deps.templates.StackNames()
}
