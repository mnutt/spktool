package services

import (
	"context"
	"encoding/json"
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
	devTestPkgdefJSONPath  = "/tmp/spktool-dev-test-pkgdef.json"
	devTestPkgdefPath      = "/tmp/spktool-dev-test-pkgdef.capnp"
	defaultPkgdefGuestPath = "/opt/app/.sandstorm/sandstorm-pkgdef.capnp"
	capnpGuestImportPath   = "/opt/sandstorm/latest/usr/include"
	capnpPackageSchemaPath = "/opt/sandstorm/latest/usr/include/sandstorm/package.capnp"
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

	prepared, err := s.preparePackPkgdef(ctx, workDir, project, plugin, opts)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = prepared.cleanup(ctx)
	}()

	if err := os.Remove(hostArtifact); err != nil && !os.IsNotExist(err) {
		return nil, domain.Wrap(domain.ErrExternal, "services.Pack", "remove stale host artifact", err)
	}

	if err := s.buildPackageInGuest(ctx, project, plugin, prepared.pkgdefRef, guestArtifact); err != nil {
		return nil, err
	}
	if err := moveFile(hostArtifact, outputPath); err != nil {
		return nil, err
	}
	return &PackResult{
		OutputPath: outputPath,
		PackageID:  prepared.packageID,
		AppID:      prepared.appID,
		Dev:        opts.Dev,
		SetVersion: opts.SetVersion,
	}, nil
}

type preparedPkgdef struct {
	pkgdefRef string
	appID     string
	packageID string
	cleanup   func(context.Context) error
}

func (s *PackageService) preparePackPkgdef(ctx context.Context, workDir string, project providers.ProjectContext, plugin providers.RuntimeProvider, opts PackOptions) (preparedPkgdef, error) {
	prepared := preparedPkgdef{
		pkgdefRef: defaultPkgdefGuestPath + ":pkgdef",
		cleanup:   func(context.Context) error { return nil },
	}
	pkgdefJSON, err := s.guestPkgdefJSON(ctx, project, plugin, defaultPkgdefGuestPath)
	if err != nil {
		return preparedPkgdef{}, err
	}
	if !opts.Dev {
		packageID, err := packageIDFromPkgdefJSON(pkgdefJSON)
		if err != nil {
			return preparedPkgdef{}, err
		}
		prepared.packageID = packageID
		return prepared, nil
	}
	appID, err := s.ensureDevTestAppID(ctx, workDir, project, plugin)
	if err != nil {
		return preparedPkgdef{}, err
	}
	body, err := devPkgdefJSON(pkgdefJSON, appID, opts.SetVersion)
	if err != nil {
		return preparedPkgdef{}, err
	}
	if err := plugin.WriteFile(ctx, project, providers.RenderedFile{
		Path: devTestPkgdefJSONPath,
		Body: body,
		Mode: 0o644,
	}); err != nil {
		return preparedPkgdef{}, err
	}
	if err := s.writeGuestPkgdefFromJSON(ctx, project, plugin, devTestPkgdefJSONPath, devTestPkgdefPath); err != nil {
		_, _ = plugin.Exec(ctx, project, []string{"rm", "-f", devTestPkgdefJSONPath})
		return preparedPkgdef{}, err
	}
	return preparedPkgdef{
		pkgdefRef: devTestPkgdefPath + ":pkgdef",
		appID:     appID,
		packageID: appID,
		cleanup: func(context.Context) error {
			_, err := plugin.Exec(ctx, project, []string{"rm", "-f", devTestPkgdefJSONPath, devTestPkgdefPath})
			return err
		},
	}, nil
}

func (s *PackageService) guestPkgdefJSON(ctx context.Context, project providers.ProjectContext, plugin providers.RuntimeProvider, pkgdefPath string) ([]byte, error) {
	result, err := plugin.Exec(ctx, project, []string{"capnp", "-I", capnpGuestImportPath, "eval", "-ojson", "--short", pkgdefPath, "pkgdef"})
	if err != nil {
		return nil, domain.Wrap(domain.ErrExternal, "services.guestPkgdefJSON", "`capnp` is required in the VM; rerun `spktool vm provision` to install capnproto", err)
	}
	return []byte(result.Stdout), nil
}

func (s *PackageService) writeGuestPkgdefFromJSON(ctx context.Context, project providers.ProjectContext, plugin providers.RuntimeProvider, jsonPath, outputPath string) error {
	script := fmt.Sprintf(`set -euo pipefail
converted="$(capnp -I %s convert --short json:text %s PackageDefinition < %s)"
file_id="$(capnp id | sed -E 's/^@?([^;]+);?$/\1/')"
{
  printf '@%%s;\n\n' "$file_id"
  printf 'using Spk = import "/sandstorm/package.capnp";\n\n'
  printf 'const pkgdef :Spk.PackageDefinition = %%s;\n' "$converted"
} > %s`, capnpGuestImportPath, capnpPackageSchemaPath, jsonPath, outputPath)
	_, err := plugin.Exec(ctx, project, []string{"bash", "-lc", script})
	if err != nil {
		return domain.Wrap(domain.ErrExternal, "services.writeGuestPkgdefFromJSON", "write temporary dev package definition", err)
	}
	return nil
}

func (s *PackageService) buildPackageInGuest(ctx context.Context, project providers.ProjectContext, plugin providers.RuntimeProvider, pkgdefRef, guestArtifact string) error {
	command := []string{
		"cd", "/opt/app/.sandstorm",
		"&&", "spk", "pack",
		"--keyring=" + keyringGuestPath,
		"--pkg-def=" + pkgdefRef,
		guestArtifact,
		"&&", "spk", "verify", "--details", guestArtifact,
		"&&", "mv", guestArtifact, "/opt/app/sandstorm-package.spk",
	}
	_, err := plugin.Exec(ctx, project, command)
	return err
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
	appIDPattern  = regexp.MustCompile(`^[a-z0-9]{20,}$`)
	signingFields = map[string]struct{}{
		"pgpSignature":            {},
		"pgpKeyring":              {},
		"authorPgpSignature":      {},
		"authorPgpKeyring":        {},
		"authorPgpKeyFingerprint": {},
	}
)

func packageIDFromPkgdefJSON(input []byte) (string, error) {
	var pkgdef map[string]any
	if err := json.Unmarshal(input, &pkgdef); err != nil {
		return "", domain.Wrap(domain.ErrInvalidArgument, "services.packageIDFromPkgdefJSON", "parse package definition JSON", err)
	}
	id, ok := pkgdef["id"].(string)
	if !ok || !validAppID(id) {
		return "", &domain.Error{Code: domain.ErrInvalidArgument, Op: "services.packageIDFromPkgdefJSON", Message: "package definition JSON is missing a valid id"}
	}
	return id, nil
}

func devPkgdefJSON(input []byte, appID, setVersion string) ([]byte, error) {
	if !validAppID(appID) {
		return nil, &domain.Error{Code: domain.ErrInvalidArgument, Op: "services.devPkgdefJSON", Message: "invalid app id"}
	}
	var pkgdef map[string]any
	if err := json.Unmarshal(input, &pkgdef); err != nil {
		return nil, domain.Wrap(domain.ErrInvalidArgument, "services.devPkgdefJSON", "parse package definition JSON", err)
	}
	pkgdef["id"] = appID
	if err := appendJSONDefaultText(pkgdef, "manifest", "appTitle", " Test"); err != nil {
		return nil, err
	}
	if setVersion != "" {
		if err := setJSONDefaultText(pkgdef, setVersion, "manifest", "appMarketingVersion"); err != nil {
			return nil, err
		}
	}
	stripSigningJSON(pkgdef)
	output, err := json.Marshal(pkgdef)
	if err != nil {
		return nil, domain.Wrap(domain.ErrExternal, "services.devPkgdefJSON", "encode package definition JSON", err)
	}
	return output, nil
}

func appendJSONDefaultText(root map[string]any, pathA, pathB, suffix string) error {
	current, err := jsonObjectAt(root, pathA, pathB)
	if err != nil {
		return err
	}
	text, ok := current["defaultText"].(string)
	if !ok {
		return &domain.Error{Code: domain.ErrInvalidArgument, Op: "services.appendJSONDefaultText", Message: strings.Join([]string{pathA, pathB, "defaultText"}, ".") + " is missing or not a string"}
	}
	if !strings.HasSuffix(text, suffix) {
		current["defaultText"] = text + suffix
	}
	return nil
}

func setJSONDefaultText(root map[string]any, value string, path ...string) error {
	current, err := jsonObjectAt(root, path...)
	if err != nil {
		return err
	}
	if _, ok := current["defaultText"].(string); !ok {
		return &domain.Error{Code: domain.ErrInvalidArgument, Op: "services.setJSONDefaultText", Message: strings.Join(append(path, "defaultText"), ".") + " is missing or not a string"}
	}
	current["defaultText"] = value
	return nil
}

func jsonObjectAt(root map[string]any, path ...string) (map[string]any, error) {
	current := root
	for _, key := range path {
		next, ok := current[key].(map[string]any)
		if !ok {
			return nil, &domain.Error{Code: domain.ErrInvalidArgument, Op: "services.jsonObjectAt", Message: strings.Join(path, ".") + " is missing or not an object"}
		}
		current = next
	}
	return current, nil
}

func stripSigningJSON(value any) {
	switch current := value.(type) {
	case map[string]any:
		for key, item := range current {
			if _, ok := signingFields[key]; ok {
				delete(current, key)
				continue
			}
			stripSigningJSON(item)
		}
	case []any:
		for _, item := range current {
			stripSigningJSON(item)
		}
	}
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
