package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mnutt/spktool/internal/domain"
	"github.com/mnutt/spktool/internal/providers"
	"github.com/mnutt/spktool/internal/runner"
	"github.com/mnutt/spktool/internal/services"
)

type ProjectBootstrapApp interface {
	SetupVM(context.Context, string, domain.ProviderName, string, bool) (*domain.ProjectState, error)
	UpgradeVM(context.Context, string, domain.ProviderName) (*domain.ProjectState, error)
	RenderConfig(context.Context, string, domain.ProviderName) (*services.ConfigRender, error)
	StackNames() ([]string, error)
}

type PackageApp interface {
	Init(context.Context, string, domain.ProviderName) (*domain.ProjectState, error)
	Dev(context.Context, string, domain.ProviderName) (*domain.ProjectState, error)
	Pack(context.Context, string, string, domain.ProviderName) (*domain.ProjectState, error)
	Verify(context.Context, string, string, domain.ProviderName) (*domain.ProjectState, error)
	Publish(context.Context, string, string, domain.ProviderName) (*domain.ProjectState, error)
}

type KeyApp interface {
	Keygen(context.Context, string, []string, domain.ProviderName) (runner.Result, error)
	ListKeys(context.Context, string, []string, domain.ProviderName) (runner.Result, error)
	GetKey(context.Context, string, string, domain.ProviderName) (runner.Result, error)
}

type GrainApp interface {
	EnterGrain(context.Context, string, domain.ProviderName, bool) (*domain.ProjectState, error)
}

type UtilityApp interface {
	Add(context.Context, string, string) (*domain.ProjectState, error)
	ListUtilities(context.Context) (*services.UtilityCatalog, error)
	DescribeUtility(context.Context, string) (*services.UtilityDetails, error)
}

type SkillApp interface {
	InstallSkills(context.Context, string, services.InstallSkillsRequest) (*services.InstallSkillsResult, error)
}

type VMApp interface {
	VMCreate(context.Context, string, domain.ProviderName) (*domain.ProjectState, error)
	VMUp(context.Context, string, domain.ProviderName) (*domain.ProjectState, error)
	VMHalt(context.Context, string, domain.ProviderName) (*domain.ProjectState, error)
	VMDestroy(context.Context, string, domain.ProviderName) (*domain.ProjectState, error)
	VMStatus(context.Context, string, domain.ProviderName) (providers.Status, error)
	VMProvision(context.Context, string, domain.ProviderName) (*domain.ProjectState, error)
	VMSSH(context.Context, string, []string, domain.ProviderName) (*domain.ProjectState, error)
}

type Applications struct {
	Bootstrap ProjectBootstrapApp
	Packages  PackageApp
	Keys      KeyApp
	Grains    GrainApp
	Utility   UtilityApp
	Skills    SkillApp
	VM        VMApp
}

type Config struct {
	Program         string
	Args            []string
	DefaultProvider domain.ProviderName
	Stdout          io.Writer
	Stderr          io.Writer
}

type GlobalFlags struct {
	WorkDir   string
	Provider  string
	LogFormat string
	Output    string
	Verbose   bool
	NonInter  bool
	ShowHelp  bool
	ShowVer   bool
}

func Run(ctx context.Context, apps Applications, cfg Config) error {
	normalizedArgs := normalizeGlobalArgs(cfg.Args)
	flags := flag.NewFlagSet(cfg.Program, flag.ContinueOnError)
	flags.SetOutput(cfg.Stderr)

	var global GlobalFlags
	flags.StringVar(&global.WorkDir, "work-directory", mustGetwd(), "project working directory")
	flags.StringVar(&global.Provider, "provider", "", "provider to use (vagrant|lima)")
	flags.StringVar(&global.LogFormat, "log-format", "text", "log format (text|json)")
	flags.StringVar(&global.Output, "output", "text", "output format (text|json)")
	flags.BoolVar(&global.Verbose, "verbose", false, "enable verbose logging")
	flags.BoolVar(&global.NonInter, "noninteractive", false, "disable interactive prompts")
	flags.BoolVar(&global.ShowHelp, "help", false, "show help")
	flags.BoolVar(&global.ShowVer, "version", false, "show version")

	if err := flags.Parse(normalizedArgs); err != nil {
		return err
	}
	if global.ShowVer {
		_, err := fmt.Fprintln(cfg.Stdout, services.ToolVersion)
		return err
	}

	args := flags.Args()
	if global.ShowHelp || len(args) == 0 {
		return printHelp(cfg.Stdout, apps.Bootstrap, cfg.Program)
	}

	command := args[0]
	providerOverride := resolveProvider(global.Provider, cfg.DefaultProvider)
	switch command {
	case "setupvm":
		return runSetupVM(ctx, apps.Bootstrap, cfg.Stdout, cfg.Stderr, global.Output, global.WorkDir, providerOverride, args[1:])
	case "upgradevm":
		state, err := apps.Bootstrap.UpgradeVM(ctx, global.WorkDir, providerOverride)
		return respond(cfg.Stdout, global.Output, state, err)
	case "config":
		return runConfig(ctx, apps.Bootstrap, cfg.Stdout, global.Output, global.WorkDir, providerOverride, args[1:])
	case "init":
		state, err := apps.Packages.Init(ctx, global.WorkDir, providerOverride)
		return respond(cfg.Stdout, global.Output, state, err)
	case "dev":
		state, err := apps.Packages.Dev(ctx, global.WorkDir, providerOverride)
		return respond(cfg.Stdout, global.Output, state, err)
	case "pack":
		if len(args) < 2 {
			return writeUsage(cfg.Stdout, global.Output, "pack output path is required", "pack <output.spk>")
		}
		state, err := apps.Packages.Pack(ctx, global.WorkDir, args[1], providerOverride)
		return respond(cfg.Stdout, global.Output, state, err)
	case "verify":
		if len(args) < 2 {
			return writeUsage(cfg.Stdout, global.Output, "verify spk path is required", "verify <input.spk>")
		}
		state, err := apps.Packages.Verify(ctx, global.WorkDir, args[1], providerOverride)
		return respond(cfg.Stdout, global.Output, state, err)
	case "publish":
		if len(args) < 2 {
			return writeUsage(cfg.Stdout, global.Output, "publish spk path is required", "publish <input.spk>")
		}
		state, err := apps.Packages.Publish(ctx, global.WorkDir, args[1], providerOverride)
		return respond(cfg.Stdout, global.Output, state, err)
	case "keygen":
		result, err := apps.Keys.Keygen(ctx, global.WorkDir, args[1:], providerOverride)
		return respond(cfg.Stdout, global.Output, result, err)
	case "listkeys":
		result, err := apps.Keys.ListKeys(ctx, global.WorkDir, args[1:], providerOverride)
		return respond(cfg.Stdout, global.Output, result, err)
	case "getkey":
		if len(args) < 2 {
			return writeUsage(cfg.Stdout, global.Output, "getkey key id is required", "getkey <key-id>")
		}
		result, err := apps.Keys.GetKey(ctx, global.WorkDir, args[1], providerOverride)
		return respond(cfg.Stdout, global.Output, result, err)
	case "enter-grain":
		state, err := apps.Grains.EnterGrain(ctx, global.WorkDir, providerOverride, global.NonInter)
		return respond(cfg.Stdout, global.Output, state, err)
	case "list-utils":
		if len(args) > 1 && args[1] == "--help" {
			return printListUtilsHelp(cfg.Stdout)
		}
		catalog, err := apps.Utility.ListUtilities(ctx)
		return respond(cfg.Stdout, global.Output, catalog, err)
	case "describe-util":
		if len(args) > 1 && args[1] == "--help" {
			return printDescribeUtilHelp(cfg.Stdout)
		}
		if len(args) < 2 {
			return writeUsage(cfg.Stdout, global.Output, "utility name is required", "describe-util <name>")
		}
		details, err := apps.Utility.DescribeUtility(ctx, args[1])
		return respond(cfg.Stdout, global.Output, details, err)
	case "add":
		if len(args) > 1 && args[1] == "--help" {
			return printAddHelp(cfg.Stdout)
		}
		if len(args) < 2 {
			return writeUsage(cfg.Stdout, global.Output, "utility name is required", "add <util>")
		}
		state, err := apps.Utility.Add(ctx, global.WorkDir, args[1])
		return respond(cfg.Stdout, global.Output, state, err)
	case "install-skills":
		return runInstallSkills(ctx, apps.Skills, cfg.Stdout, cfg.Stderr, global.Output, global.WorkDir, global.NonInter, args[1:])
	case "vm":
		if len(args) < 2 {
			if global.Output == "json" {
				return writePayload(cfg.Stdout, global.Output, map[string]any{
					"error": "vm subcommand is required",
					"usage": "vm create|up|halt|destroy|status|provision|ssh",
				})
			}
			return printVMHelp(cfg.Stdout)
		}
		return runVM(ctx, apps.VM, cfg.Stdout, global.Output, global.WorkDir, providerOverride, args[1:])
	default:
		return writeUsage(cfg.Stdout, global.Output, fmt.Sprintf("command %q is not implemented yet", command), "setupvm|upgradevm|config|init|dev|pack|verify|publish|keygen|listkeys|getkey|enter-grain|list-utils|describe-util|add|install-skills|vm")
	}
}

func normalizeGlobalArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	global := make([]string, 0, len(args))
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if isGlobalFlag(arg) {
			if (arg == "--help" || arg == "--version") && len(rest) > 0 {
				rest = append(rest, arg)
				continue
			}
			global = append(global, arg)
			if needsValue(arg) && i+1 < len(args) {
				i++
				global = append(global, args[i])
			}
			continue
		}
		rest = append(rest, arg)
	}
	return append(global, rest...)
}

func isGlobalFlag(arg string) bool {
	switch {
	case arg == "--work-directory", strings.HasPrefix(arg, "--work-directory="):
		return true
	case arg == "--provider", strings.HasPrefix(arg, "--provider="):
		return true
	case arg == "--log-format", strings.HasPrefix(arg, "--log-format="):
		return true
	case arg == "--output", strings.HasPrefix(arg, "--output="):
		return true
	case arg == "--verbose":
		return true
	case arg == "--noninteractive":
		return true
	case arg == "--help":
		return true
	case arg == "--version":
		return true
	default:
		return false
	}
}

func needsValue(arg string) bool {
	return arg == "--work-directory" || arg == "--provider" || arg == "--log-format" || arg == "--output"
}

func runConfig(ctx context.Context, app ProjectBootstrapApp, out io.Writer, format, workDir string, providerOverride domain.ProviderName, args []string) error {
	if len(args) == 0 || args[0] == "--help" {
		return printConfigHelp(out)
	}
	if args[0] == "render" && len(args) > 1 && args[1] == "--help" {
		return printConfigHelp(out)
	}
	switch args[0] {
	case "render":
		rendered, err := app.RenderConfig(ctx, workDir, providerOverride)
		return respond(out, format, rendered, err)
	default:
		return writeUsage(out, format, fmt.Sprintf("config subcommand %q is not supported", args[0]), "config render")
	}
}

func runSetupVM(ctx context.Context, app ProjectBootstrapApp, out, errOut io.Writer, format, workDir string, providerOverride domain.ProviderName, args []string) error {
	flags := flag.NewFlagSet("setupvm", flag.ContinueOnError)
	flags.SetOutput(errOut)
	force := flags.Bool("force", false, "overwrite existing setupvm-managed project files")
	showHelp := flags.Bool("help", false, "show help")
	if err := flags.Parse(args); err != nil {
		return err
	}
	rest := flags.Args()
	if *showHelp {
		return printSetupVMHelp(out, app)
	}
	if len(rest) < 1 {
		stacks, _ := app.StackNames()
		return writeUsage(out, format, "stack is required", fmt.Sprintf("setupvm [--force] <stack>\nknown stacks: %s", strings.Join(stacks, ", ")))
	}
	state, err := app.SetupVM(ctx, workDir, providerOverride, rest[0], *force)
	return respond(out, format, state, err)
}

func runInstallSkills(ctx context.Context, app SkillApp, out, errOut io.Writer, format, workDir string, nonInteractive bool, args []string) error {
	flags := flag.NewFlagSet("install-skills", flag.ContinueOnError)
	flags.SetOutput(errOut)
	codex := flags.Bool("codex", false, "install Codex skill files into .codex/skills")
	claude := flags.Bool("claude", false, "install Claude Code skill files into .claude/skills")
	force := flags.Bool("force", false, "overwrite existing installed skill files")
	showHelp := flags.Bool("help", false, "show help")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *showHelp {
		return printInstallSkillsHelp(out)
	}
	result, err := app.InstallSkills(ctx, workDir, services.InstallSkillsRequest{
		Codex:          *codex,
		Claude:         *claude,
		Force:          *force,
		NonInteractive: nonInteractive,
	})
	return respond(out, format, result, err)
}

func runVM(ctx context.Context, app VMApp, out io.Writer, format, workDir string, providerOverride domain.ProviderName, args []string) error {
	if len(args) == 0 || args[0] == "--help" {
		return printVMHelp(out)
	}
	switch args[0] {
	case "create", "up", "halt", "destroy", "status", "provision", "ssh":
		if len(args) > 1 && args[1] == "--help" {
			return printVMHelp(out)
		}
	}
	switch args[0] {
	case "create":
		state, err := app.VMCreate(ctx, workDir, providerOverride)
		return respond(out, format, state, err)
	case "up":
		state, err := app.VMUp(ctx, workDir, providerOverride)
		return respond(out, format, state, err)
	case "halt":
		state, err := app.VMHalt(ctx, workDir, providerOverride)
		return respond(out, format, state, err)
	case "destroy":
		state, err := app.VMDestroy(ctx, workDir, providerOverride)
		return respond(out, format, state, err)
	case "status":
		status, err := app.VMStatus(ctx, workDir, providerOverride)
		return respond(out, format, status, err)
	case "provision":
		state, err := app.VMProvision(ctx, workDir, providerOverride)
		return respond(out, format, state, err)
	case "ssh":
		state, err := app.VMSSH(ctx, workDir, args[1:], providerOverride)
		return respond(out, format, state, err)
	default:
		return writeUsage(out, format, fmt.Sprintf("vm subcommand %q is not supported", args[0]), "vm create|up|halt|destroy|status|provision|ssh")
	}
}

func printHelp(out io.Writer, app ProjectBootstrapApp, program string) error {
	stacks, _ := app.StackNames()
	_, err := fmt.Fprintf(out, `%s unifies the legacy vagrant-spk and lima-spk workflows.

Commands:
  setupvm [--force] <stack>
  upgradevm
  config render
  init
  dev
  pack <output.spk>
  verify <input.spk>
  publish <input.spk>
  keygen [args...]
  listkeys [args...]
  getkey <key-id>
  enter-grain
  list-utils
  describe-util <name>
  add <util>
  install-skills [--codex] [--claude] [--force]
  vm create|up|halt|destroy|status|provision|ssh

Flags:
  --provider vagrant|lima
  --work-directory <dir>
  --log-format text|json
  --output text|json
  --verbose

Known stacks: %v
`, program, stacks)
	return err
}

func printSetupVMHelp(out io.Writer, app ProjectBootstrapApp) error {
	stacks, _ := app.StackNames()
	_, err := fmt.Fprintf(out, `setupvm initializes a project workspace for a stack and provider.

Usage:
  setupvm [--force] <stack>

Flags:
  --force   overwrite setupvm-managed project files

Known stacks: %v
`, stacks)
	return err
}

func printConfigHelp(out io.Writer) error {
	_, err := fmt.Fprint(out, `config exposes generated configuration artifacts.

Usage:
  config render
`)
	return err
}

func printVMHelp(out io.Writer) error {
	_, err := fmt.Fprint(out, `vm manages provider instances for the current project.

Usage:
  vm create
  vm up
  vm halt
  vm destroy
  vm status
  vm provision
  vm ssh [args...]
`)
	return err
}

func printInstallSkillsHelp(out io.Writer) error {
	_, err := fmt.Fprint(out, `install-skills exports agent skill files into the current project.

Usage:
  install-skills [--codex] [--claude] [--force]

Flags:
  --codex    install Codex skill files into .codex/skills
  --claude   install Claude Code skill files into .claude/skills
  --force    overwrite existing installed skill files
`)
	return err
}

func printListUtilsHelp(out io.Writer) error {
	_, err := fmt.Fprint(out, `list-utils lists installable Sandstorm utilities.

Usage:
  list-utils
`)
	return err
}

func printDescribeUtilHelp(out io.Writer) error {
	_, err := fmt.Fprint(out, `describe-util shows details for an installable Sandstorm utility.

Usage:
  describe-util <name>
`)
	return err
}

func printAddHelp(out io.Writer) error {
	_, err := fmt.Fprint(out, `add installs a Sandstorm utility into the current project.

Usage:
  add <util>
`)
	return err
}

func resolveProvider(raw string, fallback domain.ProviderName) domain.ProviderName {
	if raw != "" {
		return domain.ProviderName(raw)
	}
	return fallback
}

func respond(out io.Writer, format string, payload any, err error) error {
	if err != nil {
		if format == "json" {
			return writePayload(out, format, map[string]any{"ok": false, "error": err.Error()})
		}
		return err
	}
	if format == "json" {
		return writePayload(out, format, map[string]any{"ok": true, "result": payload})
	}
	return writePayload(out, format, payload)
}

func writePayload(out io.Writer, format string, payload any) error {
	if format != "json" {
		_, err := fmt.Fprintln(out, renderText(out, payload))
		return err
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func writeUsage(out io.Writer, format, message, usage string) error {
	if format == "json" {
		return writePayload(out, format, map[string]any{
			"error": message,
			"usage": usage,
		})
	}
	_, err := fmt.Fprintf(out, "error: %s\nusage: %s\n", message, usage)
	return err
}

func renderText(out io.Writer, payload any) string {
	color := supportsColor(out)
	switch v := payload.(type) {
	case *domain.ProjectState:
		if v == nil {
			return ""
		}
		return formatProjectState(v, color)
	case providers.Status:
		return formatProviderStatus(v, color)
	case runner.Result:
		return strings.TrimRight(v.Stdout, "\n")
	case *services.ConfigRender:
		if v == nil {
			return ""
		}
		parts := make([]string, 0, len(v.Files))
		for _, file := range v.Files {
			parts = append(parts, fmt.Sprintf("== %s ==\n%s", file.Path, strings.TrimRight(file.Body, "\n")))
		}
		return strings.Join(parts, "\n\n")
	case *services.UtilityCatalog:
		if v == nil {
			return ""
		}
		lines := make([]string, 0, len(v.Utilities))
		for _, utility := range v.Utilities {
			if strings.TrimSpace(utility.Summary) == "" {
				lines = append(lines, utility.Name)
				continue
			}
			lines = append(lines, fmt.Sprintf("%s - %s", utility.Name, utility.Summary))
		}
		return strings.Join(lines, "\n")
	case *services.UtilityDetails:
		if v == nil {
			return ""
		}
		parts := []string{v.Utility.Name}
		if strings.TrimSpace(v.Utility.Description) != "" {
			parts = append(parts, v.Utility.Description)
		} else if strings.TrimSpace(v.Utility.Summary) != "" {
			parts = append(parts, v.Utility.Summary)
		}
		if len(v.Utility.Examples) > 0 {
			parts = append(parts, "Examples:")
			for _, example := range v.Utility.Examples {
				parts = append(parts, "  "+example)
			}
		}
		if len(parts) <= 2 {
			return strings.Join(parts, "\n\n")
		}
		body := append([]string{}, parts[:2]...)
		body = append(body, "")
		body = append(body, parts[2:]...)
		return strings.Join(body, "\n")
	case *services.InstallSkillsResult:
		if v == nil {
			return ""
		}
		lines := make([]string, 0, len(v.Targets)+len(v.Files)+1)
		if len(v.Targets) > 0 {
			lines = append(lines, "targets: "+strings.Join(v.Targets, ", "))
		}
		lines = append(lines, v.Files...)
		if v.GitignoreUpdated {
			lines = append(lines, ".gitignore updated")
		}
		return strings.Join(lines, "\n")
	case map[string]any:
		return fmt.Sprintf("%v", v)
	default:
		return fmt.Sprintf("%v", payload)
	}
}

func formatProjectState(state *domain.ProjectState, color bool) string {
	return formatKeyValues(color,
		"provider", string(state.Provider),
		"stack", state.Stack,
		"vm", state.VMInstance,
	)
}

func formatProviderStatus(status providers.Status, color bool) string {
	return formatKeyValues(color,
		"provider", string(status.Provider),
		"instance", status.InstanceName,
		"status", status.State,
	)
}

func formatKeyValues(color bool, pairs ...string) string {
	if len(pairs)%2 != 0 {
		return strings.Join(pairs, " ")
	}
	parts := make([]string, 0, len(pairs)+len(pairs)/2-1)
	for i := 0; i < len(pairs); i += 2 {
		if i > 0 {
			if color {
				parts = append(parts, colorize("·", ansiDim))
			}
		}
		key := pairs[i]
		value := pairs[i+1]
		if color {
			parts = append(parts, colorize(key, ansiDim), colorize(value, ansiSoft))
			continue
		}
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, " ")
}

func supportsColor(out io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if term := os.Getenv("TERM"); term == "" || term == "dumb" {
		return false
	}
	file, ok := out.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func colorize(value, code string) string {
	return "\x1b[" + code + "m" + value + "\x1b[0m"
}

const (
	ansiDim  = "90"
	ansiSoft = "37"
)

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return filepath.Clean(".")
	}
	return wd
}
