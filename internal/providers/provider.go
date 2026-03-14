package providers

import (
	"context"
	"errors"
	"fmt"

	"github.com/mnutt/spktool/internal/domain"
	"github.com/mnutt/spktool/internal/runner"
)

type ProjectContext struct {
	WorkDir string
	State   *domain.ProjectState
}

type Status struct {
	Provider     domain.ProviderName `json:"provider"`
	InstanceName string              `json:"instanceName"`
	State        string              `json:"state"`
}

type Grain struct {
	SupervisorPID string `json:"supervisorPid"`
	GrainID       string `json:"grainId"`
	ChildPID      int    `json:"childPid"`
}

type Provider interface {
	Name() domain.ProviderName
	Up(context.Context, ProjectContext) error
	Halt(context.Context, ProjectContext) error
	Destroy(context.Context, ProjectContext) error
	SSH(context.Context, ProjectContext, []string) error
	Exec(context.Context, ProjectContext, []string) (runner.Result, error)
	ExecInteractive(context.Context, ProjectContext, []string) error
	WriteFile(context.Context, ProjectContext, RenderedFile) error
	ListGrains(context.Context, ProjectContext) ([]Grain, error)
	AttachGrain(context.Context, ProjectContext, Grain, []byte, string) error
	Provision(context.Context, ProjectContext) error
	Status(context.Context, ProjectContext) (Status, error)
}

type Plugin interface {
	Provider
	BootstrapFiles(ProjectContext) ([]RenderedFile, error)
	DetectInstanceName(workDir string) string
}

type RenderedFile struct {
	Path string
	Body []byte
	Mode uint32
}

type Registry struct {
	plugins map[domain.ProviderName]Plugin
}

func NewRegistry(plugins ...Plugin) *Registry {
	items := make(map[domain.ProviderName]Plugin, len(plugins))
	for _, plugin := range plugins {
		items[plugin.Name()] = plugin
	}
	return &Registry{plugins: items}
}

func (r *Registry) Plugin(name domain.ProviderName) (Plugin, error) {
	plugin, ok := r.plugins[name]
	if !ok {
		return nil, &domain.Error{Code: domain.ErrUnsupported, Op: "providers.Plugin", Message: fmt.Sprintf("unknown provider %q", name)}
	}
	return plugin, nil
}

func (r *Registry) DetectDefault(name string) domain.ProviderName {
	switch name {
	case "vagrant-spk":
		return domain.ProviderVagrant
	case "lima-spk":
		return domain.ProviderLima
	default:
		return ""
	}
}

func RequireState(ctx ProjectContext) error {
	if ctx.State == nil {
		return errors.New("project state is required")
	}
	return nil
}
