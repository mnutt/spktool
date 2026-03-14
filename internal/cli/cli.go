package cli

import (
	"context"
	"encoding/json"
	"errors"
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

type App interface {
	SetupVM(context.Context, string, domain.ProviderName, string, bool) (*domain.ProjectState, error)
	UpgradeVM(context.Context, string, domain.ProviderName) (*domain.ProjectState, error)
	RenderConfig(context.Context, string, domain.ProviderName) (*services.ConfigRender, error)
	Init(context.Context, string, domain.ProviderName) (*domain.ProjectState, error)
	Dev(context.Context, string, domain.ProviderName) (*domain.ProjectState, error)
	Pack(context.Context, string, string, domain.ProviderName) (*domain.ProjectState, error)
	Verify(context.Context, string, string, domain.ProviderName) (*domain.ProjectState, error)
	Publish(context.Context, string, string, domain.ProviderName) (*domain.ProjectState, error)
	Keygen(context.Context, string, []string, domain.ProviderName) (runner.Result, error)
	ListKeys(context.Context, string, []string, domain.ProviderName) (runner.Result, error)
	GetKey(context.Context, string, string, domain.ProviderName) (runner.Result, error)
	EnterGrain(context.Context, string, domain.ProviderName) (*domain.ProjectState, error)
	VMUp(context.Context, string, domain.ProviderName) (*domain.ProjectState, error)
	VMHalt(context.Context, string, domain.ProviderName) (*domain.ProjectState, error)
	VMDestroy(context.Context, string, domain.ProviderName) (*domain.ProjectState, error)
	VMStatus(context.Context, string, domain.ProviderName) (providers.Status, error)
	VMProvision(context.Context, string, domain.ProviderName) (*domain.ProjectState, error)
	VMSSH(context.Context, string, []string, domain.ProviderName) (*domain.ProjectState, error)
	StackNames() ([]string, error)
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

func Run(ctx context.Context, app App, cfg Config) error {
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
		return printHelp(cfg.Stdout, app, cfg.Program)
	}

	command := args[0]
	providerOverride := resolveProvider(global.Provider, cfg.DefaultProvider)
	switch command {
	case "setupvm":
		return runSetupVM(ctx, app, cfg.Stdout, cfg.Stderr, global.Output, global.WorkDir, providerOverride, args[1:])
	case "upgradevm":
		state, err := app.UpgradeVM(ctx, global.WorkDir, providerOverride)
		return respond(cfg.Stdout, global.Output, state, err)
	case "config":
		return runConfig(ctx, app, cfg.Stdout, global.Output, global.WorkDir, providerOverride, args[1:])
	case "init":
		state, err := app.Init(ctx, global.WorkDir, providerOverride)
		return respond(cfg.Stdout, global.Output, state, err)
	case "dev":
		state, err := app.Dev(ctx, global.WorkDir, providerOverride)
		return respond(cfg.Stdout, global.Output, state, err)
	case "pack":
		if len(args) < 2 {
			return &domain.Error{Code: domain.ErrInvalidArgument, Op: "cli.Run", Message: "pack output path is required"}
		}
		state, err := app.Pack(ctx, global.WorkDir, args[1], providerOverride)
		return respond(cfg.Stdout, global.Output, state, err)
	case "verify":
		if len(args) < 2 {
			return &domain.Error{Code: domain.ErrInvalidArgument, Op: "cli.Run", Message: "verify spk path is required"}
		}
		state, err := app.Verify(ctx, global.WorkDir, args[1], providerOverride)
		return respond(cfg.Stdout, global.Output, state, err)
	case "publish":
		if len(args) < 2 {
			return &domain.Error{Code: domain.ErrInvalidArgument, Op: "cli.Run", Message: "publish spk path is required"}
		}
		state, err := app.Publish(ctx, global.WorkDir, args[1], providerOverride)
		return respond(cfg.Stdout, global.Output, state, err)
	case "keygen":
		result, err := app.Keygen(ctx, global.WorkDir, args[1:], providerOverride)
		return respond(cfg.Stdout, global.Output, result, err)
	case "listkeys":
		result, err := app.ListKeys(ctx, global.WorkDir, args[1:], providerOverride)
		return respond(cfg.Stdout, global.Output, result, err)
	case "getkey":
		if len(args) < 2 {
			return &domain.Error{Code: domain.ErrInvalidArgument, Op: "cli.Run", Message: "getkey key id is required"}
		}
		result, err := app.GetKey(ctx, global.WorkDir, args[1], providerOverride)
		return respond(cfg.Stdout, global.Output, result, err)
	case "enter-grain":
		state, err := app.EnterGrain(ctx, global.WorkDir, providerOverride)
		return respond(cfg.Stdout, global.Output, state, err)
	case "vm":
		if len(args) < 2 {
			return errors.New("vm subcommand is required")
		}
		return runVM(ctx, app, cfg.Stdout, global.Output, global.WorkDir, providerOverride, args[1:])
	default:
		return &domain.Error{Code: domain.ErrUnsupported, Op: "cli.Run", Message: fmt.Sprintf("command %q is not implemented yet", command)}
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

func runConfig(ctx context.Context, app App, out io.Writer, format, workDir string, providerOverride domain.ProviderName, args []string) error {
	if len(args) == 0 {
		return &domain.Error{Code: domain.ErrInvalidArgument, Op: "cli.runConfig", Message: "config subcommand is required"}
	}
	switch args[0] {
	case "render":
		rendered, err := app.RenderConfig(ctx, workDir, providerOverride)
		return respond(out, format, rendered, err)
	default:
		return &domain.Error{Code: domain.ErrUnsupported, Op: "cli.runConfig", Message: fmt.Sprintf("config subcommand %q is not supported", args[0])}
	}
}

func runSetupVM(ctx context.Context, app App, out, errOut io.Writer, format, workDir string, providerOverride domain.ProviderName, args []string) error {
	flags := flag.NewFlagSet("setupvm", flag.ContinueOnError)
	flags.SetOutput(errOut)
	force := flags.Bool("force", false, "overwrite existing setupvm-managed project files")
	if err := flags.Parse(args); err != nil {
		return err
	}
	rest := flags.Args()
	if len(rest) >= 1 && rest[0] == "--help" {
		return printHelp(out, app, "setupvm")
	}
	if len(rest) < 1 {
		stacks, _ := app.StackNames()
		return write(out, format, map[string]any{
			"error":  "stack is required",
			"stacks": stacks,
		})
	}
	state, err := app.SetupVM(ctx, workDir, providerOverride, rest[0], *force)
	return respond(out, format, state, err)
}

func runVM(ctx context.Context, app App, out io.Writer, format, workDir string, providerOverride domain.ProviderName, args []string) error {
	switch args[0] {
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
		return &domain.Error{Code: domain.ErrUnsupported, Op: "cli.runVM", Message: fmt.Sprintf("vm subcommand %q is not supported", args[0])}
	}
}

func printHelp(out io.Writer, app App, program string) error {
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
  vm up|halt|destroy|status|provision|ssh

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

func resolveProvider(raw string, fallback domain.ProviderName) domain.ProviderName {
	if raw != "" {
		return domain.ProviderName(raw)
	}
	return fallback
}

func respond(out io.Writer, format string, payload any, err error) error {
	if err != nil {
		if format == "json" {
			return write(out, format, map[string]any{"ok": false, "error": err.Error()})
		}
		return err
	}
	if format == "json" {
		return write(out, format, map[string]any{"ok": true, "result": payload})
	}
	_, writeErr := fmt.Fprintln(out, renderText(payload))
	return writeErr
}

func write(out io.Writer, format string, payload any) error {
	if format != "json" {
		_, err := fmt.Fprintln(out, renderText(payload))
		return err
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func renderText(payload any) string {
	switch v := payload.(type) {
	case *domain.ProjectState:
		if v == nil {
			return ""
		}
		return fmt.Sprintf("provider=%s stack=%s vm=%s", v.Provider, v.Stack, v.VMInstance)
	case providers.Status:
		return fmt.Sprintf("provider=%s instance=%s status=%s", v.Provider, v.InstanceName, v.State)
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
	case map[string]any:
		return fmt.Sprintf("%v", v)
	default:
		return fmt.Sprintf("%v", payload)
	}
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return filepath.Clean(".")
	}
	return wd
}
