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
	SetupVM(context.Context, string, domain.ProviderName, string) (*domain.ProjectState, error)
	UpgradeVM(context.Context, string) (*domain.ProjectState, error)
	Init(context.Context, string) (*domain.ProjectState, error)
	Dev(context.Context, string) (*domain.ProjectState, error)
	Pack(context.Context, string, string) (*domain.ProjectState, error)
	Verify(context.Context, string, string) (*domain.ProjectState, error)
	Publish(context.Context, string, string) (*domain.ProjectState, error)
	Keygen(context.Context, string, []string) (runner.Result, error)
	ListKeys(context.Context, string, []string) (runner.Result, error)
	GetKey(context.Context, string, string) (runner.Result, error)
	EnterGrain(context.Context, string) (*domain.ProjectState, error)
	VMUp(context.Context, string) (*domain.ProjectState, error)
	VMHalt(context.Context, string) (*domain.ProjectState, error)
	VMDestroy(context.Context, string) (*domain.ProjectState, error)
	VMStatus(context.Context, string) (providers.Status, error)
	VMProvision(context.Context, string) (*domain.ProjectState, error)
	VMSSH(context.Context, string, []string) (*domain.ProjectState, error)
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
	flags := flag.NewFlagSet(cfg.Program, flag.ContinueOnError)
	flags.SetOutput(cfg.Stderr)

	var global GlobalFlags
	flags.StringVar(&global.WorkDir, "work-directory", mustGetwd(), "project working directory")
	flags.StringVar(&global.Provider, "provider", string(cfg.DefaultProvider), "provider to use (vagrant|lima)")
	flags.StringVar(&global.LogFormat, "log-format", "text", "log format (text|json)")
	flags.StringVar(&global.Output, "output", "text", "output format (text|json)")
	flags.BoolVar(&global.Verbose, "verbose", false, "enable verbose logging")
	flags.BoolVar(&global.NonInter, "noninteractive", false, "disable interactive prompts")
	flags.BoolVar(&global.ShowHelp, "help", false, "show help")
	flags.BoolVar(&global.ShowVer, "version", false, "show version")

	if err := flags.Parse(cfg.Args); err != nil {
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
	switch command {
	case "setupvm":
		if len(args) < 2 {
			stacks, _ := app.StackNames()
			return write(cfg.Stdout, global.Output, map[string]any{
				"error":  "stack is required",
				"stacks": stacks,
			})
		}
		state, err := app.SetupVM(ctx, global.WorkDir, resolveProvider(global.Provider, cfg.DefaultProvider), args[1])
		return respond(cfg.Stdout, global.Output, state, err)
	case "upgradevm":
		state, err := app.UpgradeVM(ctx, global.WorkDir)
		return respond(cfg.Stdout, global.Output, state, err)
	case "init":
		state, err := app.Init(ctx, global.WorkDir)
		return respond(cfg.Stdout, global.Output, state, err)
	case "dev":
		state, err := app.Dev(ctx, global.WorkDir)
		return respond(cfg.Stdout, global.Output, state, err)
	case "pack":
		if len(args) < 2 {
			return &domain.Error{Code: domain.ErrInvalidArgument, Op: "cli.Run", Message: "pack output path is required"}
		}
		state, err := app.Pack(ctx, global.WorkDir, args[1])
		return respond(cfg.Stdout, global.Output, state, err)
	case "verify":
		if len(args) < 2 {
			return &domain.Error{Code: domain.ErrInvalidArgument, Op: "cli.Run", Message: "verify spk path is required"}
		}
		state, err := app.Verify(ctx, global.WorkDir, args[1])
		return respond(cfg.Stdout, global.Output, state, err)
	case "publish":
		if len(args) < 2 {
			return &domain.Error{Code: domain.ErrInvalidArgument, Op: "cli.Run", Message: "publish spk path is required"}
		}
		state, err := app.Publish(ctx, global.WorkDir, args[1])
		return respond(cfg.Stdout, global.Output, state, err)
	case "keygen":
		result, err := app.Keygen(ctx, global.WorkDir, args[1:])
		return respond(cfg.Stdout, global.Output, result, err)
	case "listkeys":
		result, err := app.ListKeys(ctx, global.WorkDir, args[1:])
		return respond(cfg.Stdout, global.Output, result, err)
	case "getkey":
		if len(args) < 2 {
			return &domain.Error{Code: domain.ErrInvalidArgument, Op: "cli.Run", Message: "getkey key id is required"}
		}
		result, err := app.GetKey(ctx, global.WorkDir, args[1])
		return respond(cfg.Stdout, global.Output, result, err)
	case "enter-grain":
		state, err := app.EnterGrain(ctx, global.WorkDir)
		return respond(cfg.Stdout, global.Output, state, err)
	case "vm":
		if len(args) < 2 {
			return errors.New("vm subcommand is required")
		}
		return runVM(ctx, app, cfg.Stdout, global.Output, global.WorkDir, args[1:])
	default:
		return &domain.Error{Code: domain.ErrUnsupported, Op: "cli.Run", Message: fmt.Sprintf("command %q is not implemented yet", command)}
	}
}

func runVM(ctx context.Context, app App, out io.Writer, format, workDir string, args []string) error {
	switch args[0] {
	case "up":
		state, err := app.VMUp(ctx, workDir)
		return respond(out, format, state, err)
	case "halt":
		state, err := app.VMHalt(ctx, workDir)
		return respond(out, format, state, err)
	case "destroy":
		state, err := app.VMDestroy(ctx, workDir)
		return respond(out, format, state, err)
	case "status":
		status, err := app.VMStatus(ctx, workDir)
		return respond(out, format, status, err)
	case "provision":
		state, err := app.VMProvision(ctx, workDir)
		return respond(out, format, state, err)
	case "ssh":
		state, err := app.VMSSH(ctx, workDir, args[1:])
		return respond(out, format, state, err)
	default:
		return &domain.Error{Code: domain.ErrUnsupported, Op: "cli.runVM", Message: fmt.Sprintf("vm subcommand %q is not supported", args[0])}
	}
}

func printHelp(out io.Writer, app App, program string) error {
	stacks, _ := app.StackNames()
	_, err := fmt.Fprintf(out, `%s unifies the legacy vagrant-spk and lima-spk workflows.

Commands:
  setupvm <stack>
  upgradevm
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
		return fmt.Sprintf("provider=%s stack=%s vm=%s migration=%d", v.Provider, v.Stack, v.VMInstance, v.Migration)
	case providers.Status:
		return fmt.Sprintf("provider=%s instance=%s status=%s", v.Provider, v.InstanceName, v.State)
	case runner.Result:
		return strings.TrimRight(v.Stdout, "\n")
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
