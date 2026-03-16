package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/mnutt/spktool/internal/domain"
	"github.com/mnutt/spktool/internal/providers"
	"github.com/mnutt/spktool/internal/workflow"
)

func (s *PackageService) Init(ctx context.Context, workDir string, providerOverride domain.ProviderName) (*domain.ProjectState, error) {
	projectState, resolved, plugin, err := s.deps.loadRuntimeProject(ctx, workDir, providerOverride)
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
	if _, err := plugin.Exec(ctx, s.deps.projectContext(workDir, projectState, resolved), command); err != nil {
		return nil, err
	}
	return projectState, nil
}

func (s *PackageService) Dev(ctx context.Context, workDir string, providerOverride domain.ProviderName) (*domain.ProjectState, error) {
	projectState, resolved, plugin, err := s.deps.loadRuntimeProject(ctx, workDir, providerOverride)
	if err != nil {
		return nil, err
	}

	helperDir := filepath.ToSlash(filepath.Join("/tmp", string(projectState.Provider)+"-spk-devhelpers"))
	tailerBody, err := s.deps.templates.HelperFile("grain-log-tailer.sh")
	if err != nil {
		return nil, err
	}
	wrapperBody, err := s.deps.templates.HelperFile("dev-with-tail.sh")
	if err != nil {
		return nil, err
	}

	project := s.deps.projectContext(workDir, projectState, resolved)
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

func (s *PackageService) Pack(ctx context.Context, workDir, outputPath string, providerOverride domain.ProviderName) (*domain.ProjectState, error) {
	projectState, resolved, plugin, err := s.deps.loadRuntimeProject(ctx, workDir, providerOverride)
	if err != nil {
		return nil, err
	}
	if outputPath == "" {
		return nil, &domain.Error{Code: domain.ErrInvalidArgument, Op: "services.Pack", Message: "output path is required"}
	}

	project := s.deps.projectContext(workDir, projectState, resolved)
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

func (s *PackageService) Verify(ctx context.Context, workDir, spkPath string, providerOverride domain.ProviderName) (*domain.ProjectState, error) {
	projectState, resolved, plugin, err := s.deps.loadRuntimeProject(ctx, workDir, providerOverride)
	if err != nil {
		return nil, err
	}
	if spkPath == "" {
		return nil, &domain.Error{Code: domain.ErrInvalidArgument, Op: "services.Verify", Message: "spk path is required"}
	}

	project := s.deps.projectContext(workDir, projectState, resolved)
	stagedName := filepath.Base(spkPath)
	stagedHostPath := filepath.Join(workDir, ".sandstorm", stagedName)
	stagedGuestPath := filepath.ToSlash(filepath.Join("/opt/app/.sandstorm", stagedName))
	stagedOnHost, err := samePath(spkPath, stagedHostPath)
	if err != nil {
		return nil, err
	}

	err = workflow.Run(ctx, "verify", []workflow.Step{
		{
			Name: "stage-package-on-host",
			Do: func(context.Context) error {
				if stagedOnHost {
					return nil
				}
				return copyFile(spkPath, stagedHostPath)
			},
			Rollback: func(context.Context) error {
				if stagedOnHost {
					return nil
				}
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
	if stagedOnHost {
		return projectState, nil
	}
	if err := os.Remove(stagedHostPath); err != nil && !os.IsNotExist(err) {
		return nil, domain.Wrap(domain.ErrExternal, "services.Verify", "remove staged package", err)
	}
	return projectState, nil
}

func (s *PackageService) Publish(ctx context.Context, workDir, spkPath string, providerOverride domain.ProviderName) (*domain.ProjectState, error) {
	projectState, resolved, plugin, err := s.deps.loadRuntimeProject(ctx, workDir, providerOverride)
	if err != nil {
		return nil, err
	}
	if spkPath == "" {
		return nil, &domain.Error{Code: domain.ErrInvalidArgument, Op: "services.Publish", Message: "spk path is required"}
	}

	project := s.deps.projectContext(workDir, projectState, resolved)
	stagedName := filepath.Base(spkPath)
	stagedHostPath := filepath.Join(workDir, ".sandstorm", stagedName)
	stagedGuestPath := filepath.ToSlash(filepath.Join("/opt/app/.sandstorm", stagedName))
	stagedOnHost, err := samePath(spkPath, stagedHostPath)
	if err != nil {
		return nil, err
	}

	err = workflow.Run(ctx, "publish", []workflow.Step{
		{
			Name: "stage-package-on-host",
			Do: func(context.Context) error {
				if stagedOnHost {
					return nil
				}
				return copyFile(spkPath, stagedHostPath)
			},
			Rollback: func(context.Context) error {
				if stagedOnHost {
					return nil
				}
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
	if stagedOnHost {
		return projectState, nil
	}
	if err := os.Remove(stagedHostPath); err != nil && !os.IsNotExist(err) {
		return nil, domain.Wrap(domain.ErrExternal, "services.Publish", "remove staged package", err)
	}
	return projectState, nil
}

func (s *PackageService) initArgs(stack string) string {
	data, err := s.deps.templates.StackFile(stack, "initargs")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (s *PackageService) devCommand(provider domain.ProviderName, wrapperPath string) []string {
	buildCmd := "/opt/app/.sandstorm/build.sh"
	devCmd := "cd /opt/app/.sandstorm && spk dev --pkg-def=/opt/app/.sandstorm/sandstorm-pkgdef.capnp:pkgdef"
	switch provider {
	case domain.ProviderLima:
		return []string{
			"bash", wrapperPath, "--",
			"bash", "-lc",
			buildCmd + " && sudo -u sandstorm -g sandstorm bash -lc " + devShellQuote(devCmd),
		}
	default:
		return []string{"bash", wrapperPath, "--", "bash", "-lc", buildCmd + " && " + devCmd}
	}
}

func devShellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
