package services

import (
	"context"

	"github.com/mnutt/spktool/internal/domain"
	"github.com/mnutt/spktool/internal/runner"
)

func (s *KeyService) Keygen(ctx context.Context, workDir string, args []string, providerOverride domain.ProviderName) (runner.Result, error) {
	return s.runKeyCommand(ctx, workDir, providerOverride, append([]string{"spk", "keygen", "--keyring=/host-dot-sandstorm/sandstorm-keyring"}, args...))
}

func (s *KeyService) ListKeys(ctx context.Context, workDir string, args []string, providerOverride domain.ProviderName) (runner.Result, error) {
	return s.runKeyCommand(ctx, workDir, providerOverride, append([]string{"spk", "listkeys", "--keyring=/host-dot-sandstorm/sandstorm-keyring"}, args...))
}

func (s *KeyService) GetKey(ctx context.Context, workDir, keyID string, providerOverride domain.ProviderName) (runner.Result, error) {
	if keyID == "" {
		return runner.Result{}, &domain.Error{Code: domain.ErrInvalidArgument, Op: "services.GetKey", Message: "key id is required"}
	}
	return s.runKeyCommand(ctx, workDir, providerOverride, []string{"spk", "getkey", "--keyring=/host-dot-sandstorm/sandstorm-keyring", keyID})
}

func (s *KeyService) runKeyCommand(ctx context.Context, workDir string, providerOverride domain.ProviderName, command []string) (runner.Result, error) {
	projectState, resolved, plugin, err := s.deps.loadRuntimeProject(ctx, workDir, providerOverride)
	if err != nil {
		return runner.Result{}, err
	}
	return plugin.Exec(ctx, s.deps.projectContext(workDir, projectState, resolved), command)
}
