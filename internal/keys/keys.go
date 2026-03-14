package keys

import (
	"context"
	"os"
	"path/filepath"

	"github.com/mnutt/spktool/internal/domain"
)

type Manager interface {
	EnsureLayout(context.Context) error
	DefaultKeyringPath() string
}

type LocalKeyring struct {
	Home string
}

func NewLocalKeyring(home string) *LocalKeyring {
	return &LocalKeyring{Home: home}
}

func (k *LocalKeyring) EnsureLayout(_ context.Context) error {
	root := filepath.Join(k.Home, ".sandstorm")
	cacheDir := filepath.Join(root, "caches")
	keyring := filepath.Join(root, "sandstorm-keyring")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return domain.Wrap(domain.ErrExternal, "keys.EnsureLayout", "create host keyring directories", err)
	}
	if _, err := os.Stat(keyring); os.IsNotExist(err) {
		if err := os.WriteFile(keyring, nil, 0o644); err != nil {
			return domain.Wrap(domain.ErrExternal, "keys.EnsureLayout", "create host keyring file", err)
		}
	}
	return nil
}

func (k *LocalKeyring) DefaultKeyringPath() string {
	return filepath.Join(k.Home, ".sandstorm", "sandstorm-keyring")
}
