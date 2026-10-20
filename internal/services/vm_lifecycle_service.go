package services

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"syscall"

	"github.com/mnutt/spktool/internal/domain"
	"github.com/mnutt/spktool/internal/providers"
)

func (s *VMLifecycleService) VMUp(ctx context.Context, workDir string, providerOverride domain.ProviderName, port int, allowPortConflict bool) (*domain.ProjectState, error) {
	projectState, resolved, plugin, err := s.deps.loadRuntimeProject(ctx, workDir, providerOverride)
	if err != nil {
		return nil, err
	}
	status, err := plugin.Status(ctx, s.deps.projectContext(workDir, projectState, resolved))
	if err != nil {
		return nil, wrapProviderStatusError("services.VMUp", projectState.Provider, err)
	}
	if port != 0 {
		if vmStateExists(status.State) {
			return nil, &domain.Error{
				Code:    domain.ErrConflict,
				Op:      "services.VMUp",
				Message: fmt.Sprintf("vm instance %q already exists; rejecting `vm up --port`; destroy and recreate the VM to change forwarded ports", status.InstanceName),
			}
		}
	}
	if !strings.EqualFold(strings.TrimSpace(status.State), "running") {
		httpPort := resolved.Network.Sandstorm.ExternalPort
		if port != 0 {
			httpPort = port
		}
		if err := checkVMHTTPPort(httpPort, resolved.Network.Sandstorm.LocalhostOnly, vmStateExists(status.State)); err != nil {
			var domainErr *domain.Error
			if !allowPortConflict || !errors.As(err, &domainErr) || domainErr.Code != domain.ErrConflict {
				return nil, err
			}
			s.deps.logger.Warn("HTTP port availability check failed; continuing because --allow-port-conflict was specified", "error", err)
		}
	}
	if port != 0 {
		if err := s.deps.persistSandstormPortOverride(workDir, port); err != nil {
			return nil, err
		}
		projectState, resolved, plugin, err = s.deps.refreshGeneratedFiles(ctx, workDir, providerOverride)
		if err != nil {
			return nil, err
		}
	}
	if err := plugin.Up(ctx, s.deps.projectContext(workDir, projectState, resolved)); err != nil {
		return nil, err
	}
	return projectState, nil
}

func checkVMHTTPPort(port int, localhostOnly, instanceExists bool) error {
	if port < 1 || port > 65535 {
		return &domain.Error{Code: domain.ErrInvalidArgument, Op: "services.VMUp", Message: "HTTP port must be between 1 and 65535"}
	}
	host := "0.0.0.0"
	if localhostOnly {
		host = "127.0.0.1"
	}
	address := net.JoinHostPort(host, strconv.Itoa(port))
	listener, err := net.Listen("tcp4", address)
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			instructions := "choose a different port with `spktool vm up --port <port>`"
			if instanceExists {
				instructions = "to change forwarded ports, first destroy the existing VM with `spktool vm destroy`, then run `spktool vm up --port <port>`"
			}
			return domain.Wrap(domain.ErrConflict, "services.VMUp",
				fmt.Sprintf("HTTP port %d is already in use on %s; %s", port, host, instructions), err)
		}
		return domain.Wrap(domain.ErrExternal, "services.VMUp", fmt.Sprintf("check HTTP port availability on %s", address), err)
	}
	// Release the probe before the provider starts and binds its port forward.
	return listener.Close()
}

func (s *VMLifecycleService) VMCreate(ctx context.Context, workDir string, providerOverride domain.ProviderName, sandstormDownloadURL string) (*domain.ProjectState, error) {
	if err := s.deps.persistSandstormDownloadURLOverride(workDir, sandstormDownloadURL); err != nil {
		return nil, err
	}
	projectState, resolved, plugin, err := s.deps.refreshGeneratedFiles(ctx, workDir, providerOverride)
	if err != nil {
		return nil, err
	}
	project := s.deps.projectContext(workDir, projectState, resolved)
	status, err := plugin.Status(ctx, project)
	if err != nil {
		return nil, wrapProviderStatusError("services.VMCreate", projectState.Provider, err)
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
	status, err := plugin.Status(ctx, s.deps.projectContext(workDir, projectState, resolved))
	if err != nil {
		return providers.Status{}, wrapProviderStatusError("services.VMStatus", projectState.Provider, err)
	}
	return status, nil
}

func (s *VMLifecycleService) VMProvision(ctx context.Context, workDir string, providerOverride domain.ProviderName, sandstormDownloadURL string) (*domain.ProjectState, error) {
	if err := s.deps.persistSandstormDownloadURLOverride(workDir, sandstormDownloadURL); err != nil {
		return nil, err
	}
	projectState, resolved, plugin, err := s.deps.refreshGeneratedFiles(ctx, workDir, providerOverride)
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

func wrapProviderStatusError(op string, provider domain.ProviderName, err error) error {
	return domain.Wrap(domain.ErrExternal, op,
		fmt.Sprintf("provider status lookup failed for %s; if running inside a sandbox, rerun elevated before trusting VM state", provider),
		err,
	)
}
