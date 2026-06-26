package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mnutt/spktool/internal/domain"
	"github.com/mnutt/spktool/internal/providers"
	"github.com/mnutt/spktool/internal/workflow"
)

const (
	keyringGuestPath       = "/host-dot-sandstorm/sandstorm-keyring"
	devTestAppIDHostPath   = ".sandstorm/sandstorm-test-app-id"
	devTestPkgdefPath      = "/tmp/spktool-dev-test-pkgdef.capnp"
	pkgdefUtilGuestPath    = "/tmp/spktool-pkgdef-util.py"
	capnpGuestImportPath   = "/opt/sandstorm/latest/usr/include"
	defaultPkgdefGuestPath = "/opt/app/.sandstorm/sandstorm-pkgdef.capnp"
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

func (s *PackageService) Build(ctx context.Context, workDir string, providerOverride domain.ProviderName) (*domain.ProjectState, error) {
	projectState, resolved, plugin, err := s.deps.loadRuntimeProject(ctx, workDir, providerOverride)
	if err != nil {
		return nil, err
	}
	project := s.deps.projectContext(workDir, projectState, resolved)
	if err := plugin.ExecStream(ctx, project, []string{"cd", "/opt/app", "&&", "sg", "sandstorm", "-c", "/opt/app/.sandstorm/build.sh"}); err != nil {
		return nil, err
	}
	return projectState, nil
}

func (s *PackageService) Pack(ctx context.Context, workDir, outputPath string, opts PackOptions, providerOverride domain.ProviderName) (*PackResult, error) {
	projectState, resolved, plugin, err := s.deps.loadRuntimeProject(ctx, workDir, providerOverride)
	if err != nil {
		return nil, err
	}
	if outputPath == "" {
		return nil, &domain.Error{Code: domain.ErrInvalidArgument, Op: "services.Pack", Message: "output path is required"}
	}
	if opts.SetVersion != "" && !opts.Dev {
		return nil, &domain.Error{Code: domain.ErrInvalidArgument, Op: "services.Pack", Message: "--set-version may only be used with --dev"}
	}

	project := s.deps.projectContext(workDir, projectState, resolved)
	hostArtifact := filepath.Join(workDir, "sandstorm-package.spk")
	guestArtifact := "/tmp/sandstorm-package.spk"
	if projectState.Provider == domain.ProviderVagrant {
		guestArtifact = "/home/vagrant/sandstorm-package.spk"
	}
	var verifyOutput string
	var appID string
	var packageID string
	pkgdefPath := defaultPkgdefGuestPath
	pkgdefRef := pkgdefPath + ":pkgdef"

	err = workflow.Run(ctx, "pack", []workflow.Step{
		{
			Name: "prepare-dev-pkgdef",
			Do: func(context.Context) error {
				if !opts.Dev {
					return nil
				}
				id, err := s.ensureDevTestAppID(ctx, workDir, project, plugin)
				if err != nil {
					return err
				}
				appID = id
				if err := s.writePkgdefUtil(ctx, project, plugin); err != nil {
					return err
				}
				if _, err := plugin.Exec(ctx, project, []string{"command", "-v", "capnp", ">", "/dev/null"}); err != nil {
					return domain.Wrap(domain.ErrExternal, "services.Pack", "`capnp` is required in the VM for `pack --dev`; rerun `spktool vm provision` to install capnproto", err)
				}
				command := []string{
					"python3", pkgdefUtilGuestPath,
					"--import-path", capnpGuestImportPath,
					"/opt/app/.sandstorm/sandstorm-pkgdef.capnp",
					appID,
					"--append-title-suffix", " Test",
					"--strip-signing",
				}
				if opts.SetVersion != "" {
					command = append(command, "--set-version", opts.SetVersion)
				}
				command = append(command, "--output", devTestPkgdefPath)
				if _, err := plugin.Exec(ctx, project, command); err != nil {
					return err
				}
				pkgdefPath = devTestPkgdefPath
				pkgdefRef = devTestPkgdefPath + ":pkgdef"
				return nil
			},
			Rollback: func(context.Context) error {
				if !opts.Dev {
					return nil
				}
				_, err := plugin.Exec(ctx, project, []string{"rm", "-f", devTestPkgdefPath})
				return err
			},
		},
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
					"cd", "/opt/app/.sandstorm",
					"&&", "spk", "pack",
					"--keyring=" + keyringGuestPath,
					"--pkg-def=" + pkgdefRef,
					guestArtifact,
					"&&", "spk", "verify", "--details", guestArtifact,
					"&&", "mv", guestArtifact, "/opt/app/sandstorm-package.spk",
				}
				result, err := plugin.Exec(ctx, project, command)
				verifyOutput = result.Stdout
				return err
			},
		},
		{
			Name: "resolve-package-id",
			Do: func(context.Context) error {
				packageID = s.packageIDFromPkgdef(ctx, project, plugin, pkgdefPath, verifyOutput)
				return nil
			},
		},
		{
			Name: "move-package-to-output",
			Do: func(context.Context) error {
				return moveFile(hostArtifact, outputPath)
			},
		},
		{
			Name: "cleanup-dev-pkgdef",
			Do: func(context.Context) error {
				if !opts.Dev {
					return nil
				}
				_, err := plugin.Exec(ctx, project, []string{"rm", "-f", devTestPkgdefPath})
				return err
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return &PackResult{
		Project:      projectState,
		OutputPath:   outputPath,
		PackageID:    packageID,
		AppID:        appID,
		Dev:          opts.Dev,
		SetVersion:   opts.SetVersion,
		VerifyOutput: verifyOutput,
	}, nil
}

func (s *PackageService) packageIDFromPkgdef(ctx context.Context, project providers.ProjectContext, plugin providers.RuntimeProvider, pkgdefPath, verifyOutput string) string {
	if packageID, err := packageIDFromPkgdefWithCapnp(ctx, project, plugin, pkgdefPath); err == nil && packageID != "" {
		return packageID
	}
	return parsePackageIDFallback(verifyOutput)
}

func packageIDFromPkgdefWithCapnp(ctx context.Context, project providers.ProjectContext, plugin providers.RuntimeProvider, pkgdefPath string) (string, error) {
	code := strings.Join([]string{
		"import json, subprocess, sys",
		"cmd = ['capnp', '-I', sys.argv[2], 'eval', '-ojson', '--short', sys.argv[1], 'pkgdef']",
		"print(json.loads(subprocess.check_output(cmd, text=True))['id'])",
	}, "\n")
	result, err := plugin.Exec(ctx, project, []string{"python3", "-c", code, pkgdefPath, capnpGuestImportPath})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Stdout), nil
}

func (s *PackageService) ensureDevTestAppID(ctx context.Context, workDir string, project providers.ProjectContext, plugin providers.RuntimeProvider) (string, error) {
	path := filepath.Join(workDir, devTestAppIDHostPath)
	data, err := os.ReadFile(path)
	if err == nil {
		appID := strings.TrimSpace(string(data))
		if appID != "" {
			return appID, nil
		}
	} else if !os.IsNotExist(err) {
		return "", domain.Wrap(domain.ErrExternal, "services.Pack", "read dev test app id", err)
	}

	result, err := plugin.Exec(ctx, project, []string{"spk", "keygen", "--keyring=" + keyringGuestPath})
	if err != nil {
		return "", err
	}
	lines := nonEmptyLines(result.Stdout)
	if len(lines) != 1 || !validAppID(lines[0]) {
		return "", &domain.Error{
			Code:    domain.ErrExternal,
			Op:      "services.Pack",
			Message: fmt.Sprintf("unexpected `spk keygen` output while creating dev test app id: %q", result.Stdout),
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", domain.Wrap(domain.ErrExternal, "services.Pack", "create .sandstorm directory for dev test app id", err)
	}
	if err := os.WriteFile(path, []byte(lines[0]+"\n"), 0o644); err != nil {
		return "", domain.Wrap(domain.ErrExternal, "services.Pack", "write dev test app id", err)
	}
	return lines[0], nil
}

func (s *PackageService) writePkgdefUtil(ctx context.Context, project providers.ProjectContext, plugin providers.RuntimeProvider) error {
	body, err := s.deps.templates.HelperFile("pkgdef-util.py")
	if err != nil {
		return err
	}
	return plugin.WriteFile(ctx, project, providers.RenderedFile{
		Path: pkgdefUtilGuestPath,
		Body: body,
		Mode: 0o755,
	})
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
	if err := s.withStagedPackage(ctx, "verify", "services.Verify", workDir, spkPath, func(stagedGuestPath string) error {
		_, err := plugin.Exec(ctx, project, []string{"spk", "verify", "--details", stagedGuestPath})
		return err
	}); err != nil {
		return nil, err
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
	if err := s.withStagedPackage(ctx, "publish", "services.Publish", workDir, spkPath, func(stagedGuestPath string) error {
		_, err := plugin.Exec(ctx, project, []string{
			"spk", "publish",
			"--keyring=/host-dot-sandstorm/sandstorm-keyring",
			stagedGuestPath,
		})
		return err
	}); err != nil {
		return nil, err
	}
	return projectState, nil
}

func (s *PackageService) withStagedPackage(ctx context.Context, workflowName, op, workDir, spkPath string, fn func(string) error) error {
	stagedName := filepath.Base(spkPath)
	stagedHostPath := filepath.Join(workDir, ".sandstorm", stagedName)
	stagedGuestPath := filepath.ToSlash(filepath.Join("/opt/app/.sandstorm", stagedName))
	stagedOnHost, err := samePath(spkPath, stagedHostPath)
	if err != nil {
		return err
	}

	cleanup := func() error {
		if stagedOnHost {
			return nil
		}
		if err := os.Remove(stagedHostPath); err != nil && !os.IsNotExist(err) {
			return domain.Wrap(domain.ErrExternal, op, "remove staged package", err)
		}
		return nil
	}

	err = workflow.Run(ctx, workflowName, []workflow.Step{
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
			Name: workflowName + "-package-in-guest",
			Do: func(context.Context) error {
				return fn(stagedGuestPath)
			},
			Rollback: func(context.Context) error {
				return cleanup()
			},
		},
	})
	if err != nil {
		return err
	}
	return cleanup()
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
		// Lima's rootless virtiofs mount only accepts creates using the host-mapped
		// primary uid/gid. Refresh supplementary groups without switching the primary gid.
		limaDevCmd := `sudo -n -u "$(id -un)" bash -lc ` + devShellQuote(buildCmd+" && "+devCmd)
		return []string{
			"bash", wrapperPath, "--",
			"bash", "-lc",
			limaDevCmd,
		}
	default:
		return []string{"bash", wrapperPath, "--", "bash", "-lc", buildCmd + " && " + devCmd}
	}
}

var (
	packageIDFallbackPattern = regexp.MustCompile(`"packageId"\s*:\s*"([^"]+)"`)
	appIDPattern             = regexp.MustCompile(`^[a-z0-9]{20,}$`)
)

func parsePackageIDFallback(output string) string {
	match := packageIDFallbackPattern.FindStringSubmatch(output)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func nonEmptyLines(output string) []string {
	lines := strings.Split(output, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			filtered = append(filtered, line)
		}
	}
	return filtered
}

func validAppID(id string) bool {
	return appIDPattern.MatchString(id)
}

func devShellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
