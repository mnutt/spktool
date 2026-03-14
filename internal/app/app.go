package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mnutt/spktool/internal/cli"
	"github.com/mnutt/spktool/internal/domain"
	"github.com/mnutt/spktool/internal/keys"
	"github.com/mnutt/spktool/internal/logging"
	"github.com/mnutt/spktool/internal/providers"
	"github.com/mnutt/spktool/internal/providers/lima"
	"github.com/mnutt/spktool/internal/providers/vagrant"
	"github.com/mnutt/spktool/internal/runner"
	"github.com/mnutt/spktool/internal/services"
	"github.com/mnutt/spktool/internal/templates"
)

func Run(ctx context.Context, argv []string) int {
	program := filepath.Base(argv[0])
	defaultProvider := detectDefaultProvider(program)
	logCfg := detectLogConfig(argv[1:])

	logger := logging.New(logCfg)
	repo := templates.New()
	cmdRunner := runner.New(logger)
	registry := providers.NewRegistry(
		vagrant.New(cmdRunner, repo),
		lima.New(cmdRunner, repo),
	)

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	service := services.NewProjectService(
		logger,
		repo,
		registry,
		keys.NewLocalKeyring(home),
	)

	if err := cli.Run(ctx, service, cli.Config{
		Program:         program,
		Args:            argv[1:],
		DefaultProvider: defaultProvider,
		Stdout:          os.Stdout,
		Stderr:          os.Stderr,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func detectDefaultProvider(program string) domain.ProviderName {
	switch program {
	case "vagrant-spk":
		return domain.ProviderVagrant
	case "lima-spk":
		return domain.ProviderLima
	default:
		return ""
	}
}

func detectLogConfig(args []string) logging.Config {
	cfg := logging.Config{Format: logging.FormatText}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--verbose":
			cfg.Verbose = true
		case "--log-format":
			if i+1 < len(args) && strings.EqualFold(args[i+1], "json") {
				cfg.Format = logging.FormatJSON
				i++
			}
		default:
			if strings.HasPrefix(args[i], "--log-format=") {
				if strings.EqualFold(strings.TrimPrefix(args[i], "--log-format="), "json") {
					cfg.Format = logging.FormatJSON
				}
			}
		}
	}
	return cfg
}
