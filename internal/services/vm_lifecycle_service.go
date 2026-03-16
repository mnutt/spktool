package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/mnutt/spktool/internal/domain"
	"github.com/mnutt/spktool/internal/providers"
)

func (s *VMLifecycleService) VMUp(ctx context.Context, workDir string, providerOverride domain.ProviderName) (*domain.ProjectState, error) {
	projectState, resolved, plugin, err := s.deps.loadRuntimeProject(ctx, workDir, providerOverride)
	if err != nil {
		return nil, err
	}
	if err := plugin.Up(ctx, s.deps.projectContext(workDir, projectState, resolved)); err != nil {
		return nil, err
	}
	return projectState, nil
}

func (s *VMLifecycleService) VMCreate(ctx context.Context, workDir string, providerOverride domain.ProviderName) (*domain.ProjectState, error) {
	projectState, resolved, plugin, err := s.deps.loadRuntimeProject(ctx, workDir, providerOverride)
	if err != nil {
		return nil, err
	}
	project := s.deps.projectContext(workDir, projectState, resolved)
	status, err := plugin.Status(ctx, project)
	if err != nil {
		return nil, err
	}
	if vmStateExists(status.State) {
		return nil, &domain.Error{
			Code:    domain.ErrConflict,
			Op:      "services.VMCreate",
			Message: fmt.Sprintf("vm instance %q already exists; use `vm up` or `vm provision`", status.InstanceName),
		}
	}
	if err := plugin.Up(ctx, project); err != nil {
		return nil, err
	}
	if err := plugin.Provision(ctx, project); err != nil {
		return nil, err
	}
	return projectState, nil
}

func (s *VMLifecycleService) VMHalt(ctx context.Context, workDir string, providerOverride domain.ProviderName) (*domain.ProjectState, error) {
	projectState, resolved, plugin, err := s.deps.loadRuntimeProject(ctx, workDir, providerOverride)
	if err != nil {
		return nil, err
	}
	if err := plugin.Halt(ctx, s.deps.projectContext(workDir, projectState, resolved)); err != nil {
		return nil, err
	}
	return projectState, nil
}

func (s *VMLifecycleService) VMDestroy(ctx context.Context, workDir string, providerOverride domain.ProviderName) (*domain.ProjectState, error) {
	projectState, resolved, plugin, err := s.deps.loadRuntimeProject(ctx, workDir, providerOverride)
	if err != nil {
		return nil, err
	}
	if err := plugin.Destroy(ctx, s.deps.projectContext(workDir, projectState, resolved)); err != nil {
		return nil, err
	}
	return projectState, nil
}

func (s *VMLifecycleService) VMStatus(ctx context.Context, workDir string, providerOverride domain.ProviderName) (providers.Status, error) {
	projectState, resolved, plugin, err := s.deps.loadRuntimeProject(ctx, workDir, providerOverride)
	if err != nil {
		return providers.Status{}, err
	}
	return plugin.Status(ctx, s.deps.projectContext(workDir, projectState, resolved))
}

func (s *VMLifecycleService) VMProvision(ctx context.Context, workDir string, providerOverride domain.ProviderName) (*domain.ProjectState, error) {
	projectState, resolved, plugin, err := s.deps.loadRuntimeProject(ctx, workDir, providerOverride)
	if err != nil {
		return nil, err
	}
	if err := plugin.Provision(ctx, s.deps.projectContext(workDir, projectState, resolved)); err != nil {
		return nil, err
	}
	return projectState, nil
}

func (s *VMLifecycleService) VMSSH(ctx context.Context, workDir string, args []string, providerOverride domain.ProviderName) (*domain.ProjectState, error) {
	projectState, resolved, plugin, err := s.deps.loadRuntimeProject(ctx, workDir, providerOverride)
	if err != nil {
		return nil, err
	}
	if err := plugin.SSH(ctx, s.deps.projectContext(workDir, projectState, resolved), args); err != nil {
		return nil, err
	}
	return projectState, nil
}

func vmStateExists(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "", "unknown", "not_created", "not created":
		return false
	default:
		return true
	}
}
