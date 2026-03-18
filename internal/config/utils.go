package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"

	"github.com/mnutt/spktool/internal/domain"
)

const UtilsFile = "utils.toml"

type UtilsFileConfig struct {
	Installed map[string]string `toml:"installed"`
}

func LoadUtils(workDir string) (*UtilsFileConfig, error) {
	path := filepath.Join(workDir, ".sandstorm", UtilsFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &UtilsFileConfig{Installed: map[string]string{}}, nil
		}
		return nil, domain.Wrap(domain.ErrExternal, "config.LoadUtils", "read utils config", err)
	}

	var cfg UtilsFileConfig
	if err := decode(data, &cfg); err != nil {
		return nil, &domain.Error{Code: domain.ErrInvalidArgument, Op: "config.LoadUtils", Message: "parse .sandstorm/utils.toml: " + err.Error()}
	}
	if cfg.Installed == nil {
		cfg.Installed = map[string]string{}
	}
	return &cfg, nil
}

func WriteUtils(workDir string, cfg *UtilsFileConfig) error {
	if cfg == nil {
		cfg = &UtilsFileConfig{}
	}
	if cfg.Installed == nil {
		cfg.Installed = map[string]string{}
	}

	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	if err := enc.Encode(cfg); err != nil {
		return domain.Wrap(domain.ErrExternal, "config.WriteUtils", "encode utils config", err)
	}

	path := filepath.Join(workDir, ".sandstorm", UtilsFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return domain.Wrap(domain.ErrExternal, "config.WriteUtils", "create utils config directory", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return domain.Wrap(domain.ErrExternal, "config.WriteUtils", "write utils config", err)
	}
	return nil
}
