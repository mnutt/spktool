package vagrant

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/mnutt/spktool/internal/domain"
	"github.com/mnutt/spktool/internal/providers"
	"github.com/mnutt/spktool/internal/runner"
	"github.com/mnutt/spktool/internal/templates"
)

type Provider struct {
	runner    runner.Runner
	templates *templates.Repository
}

func New(r runner.Runner, repo *templates.Repository) *Provider {
	return &Provider{runner: r, templates: repo}
}

func (p *Provider) Name() domain.ProviderName { return domain.ProviderVagrant }

func (p *Provider) BootstrapFiles(project providers.ProjectContext) ([]providers.RenderedFile, error) {
	if project.Config == nil {
		return nil, &domain.Error{Code: domain.ErrInvalidArgument, Op: "vagrant.BootstrapFiles", Message: "resolved config is required"}
	}
	data, err := p.templates.BoxFile("Vagrantfile")
	if err != nil {
		return nil, err
	}
	rendered, err := renderTemplate(data, map[string]any{
		"Box":           project.Config.Vagrant.Box,
		"ExternalHost":  project.Config.Network.Sandstorm.Host,
		"GuestPort":     project.Config.Network.Sandstorm.GuestPort,
		"ExternalPort":  project.Config.Network.Sandstorm.ExternalPort,
		"LocalhostOnly": project.Config.Network.Sandstorm.LocalhostOnly,
	})
	if err != nil {
		return nil, err
	}
	return []providers.RenderedFile{{
		Path: filepath.Join(".sandstorm", ".generated", "Vagrantfile"),
		Body: rendered,
		Mode: 0o644,
	}}, nil
}

func (p *Provider) DetectInstanceName(workDir string) string {
	return filepath.Base(workDir)
}

func (p *Provider) Up(ctx context.Context, project providers.ProjectContext) error {
	_, err := p.runner.Run(ctx, runner.Spec{Name: "vagrant-up", Command: "vagrant", Args: []string{"up", "--no-provision"}, Dir: filepath.Join(project.WorkDir, ".sandstorm", ".generated"), Stream: true})
	return err
}

func (p *Provider) Halt(ctx context.Context, project providers.ProjectContext) error {
	_, err := p.runner.Run(ctx, runner.Spec{Name: "vagrant-halt", Command: "vagrant", Args: []string{"halt"}, Dir: filepath.Join(project.WorkDir, ".sandstorm", ".generated")})
	return err
}

func (p *Provider) Destroy(ctx context.Context, project providers.ProjectContext) error {
	_, err := p.runner.Run(ctx, runner.Spec{Name: "vagrant-destroy", Command: "vagrant", Args: []string{"destroy", "-f"}, Dir: filepath.Join(project.WorkDir, ".sandstorm", ".generated")})
	return err
}

func (p *Provider) SSH(ctx context.Context, project providers.ProjectContext, args []string) error {
	allArgs := append([]string{"ssh"}, args...)
	spec := runner.Spec{Name: "vagrant-ssh", Command: "vagrant", Args: allArgs, Dir: filepath.Join(project.WorkDir, ".sandstorm", ".generated")}
	if len(args) == 0 {
		spec.Interactive = true
	} else {
		spec.Stream = true
	}
	_, err := p.runner.Run(ctx, spec)
	return err
}

func (p *Provider) Exec(ctx context.Context, project providers.ProjectContext, command []string) (runner.Result, error) {
	argv := []string{"ssh", "-c", shellJoin(command)}
	return p.runner.Run(ctx, runner.Spec{Name: "vagrant-exec", Command: "vagrant", Args: argv, Dir: filepath.Join(project.WorkDir, ".sandstorm", ".generated")})
}

func (p *Provider) ExecInteractive(ctx context.Context, project providers.ProjectContext, command []string) error {
	_, err := p.runner.Run(ctx, runner.Spec{
		Name:        "vagrant-exec-interactive",
		Command:     "vagrant",
		Args:        []string{"ssh", "-c", shellJoin(command)},
		Dir:         filepath.Join(project.WorkDir, ".sandstorm", ".generated"),
		Interactive: true,
	})
	return err
}

func (p *Provider) WriteFile(ctx context.Context, project providers.ProjectContext, file providers.RenderedFile) error {
	targetDir := filepath.Dir(file.Path)
	command := []string{
		"mkdir", "-p", targetDir,
		"&&", "chmod", "755", targetDir,
		"&&", "cat", ">", file.Path,
		"&&", "chmod", strconv.FormatUint(uint64(file.Mode), 8), file.Path,
	}
	_, err := p.runner.Run(ctx, runner.Spec{
		Name:    "vagrant-write-file",
		Command: "vagrant",
		Args:    []string{"ssh", "-c", shellJoin(command)},
		Dir:     filepath.Join(project.WorkDir, ".sandstorm", ".generated"),
		Stdin:   file.Body,
	})
	return err
}

func (p *Provider) Provision(ctx context.Context, project providers.ProjectContext) error {
	_, err := p.runner.Run(ctx, runner.Spec{Name: "vagrant-provision", Command: "vagrant", Args: []string{"provision"}, Dir: filepath.Join(project.WorkDir, ".sandstorm", ".generated"), Stream: true})
	return err
}

func (p *Provider) ListGrains(ctx context.Context, project providers.ProjectContext) ([]providers.Grain, error) {
	result, err := p.runner.Run(ctx, runner.Spec{
		Name:    "vagrant-list-grains",
		Command: "vagrant",
		Args:    []string{"ssh", "-c", `pidof spk || echo no-spk;SANDSTORM_UID=$(id sandstorm | sed -r s,[^0-9],_,g | sed -r s,_+,_,g | cut -d _ -f 2 ) ; for pid in $(pidof supervisor); do echo $pid $(cat /proc/$pid/status | grep -q 'Uid:.*'${SANDSTORM_UID} && echo ownership-correct || echo ownership-wrong) $(xargs -0 -n1 echo < /proc/$pid/cmdline  | grep -v -- - | head -n2 | tail -n1) $(grep -E -l ^PPid:[[:blank:]]*${pid}$ /proc/*/status | head -n1  | sed -r s,/proc/,,g | sed -r s,/status,,) ; done`},
		Dir:     filepath.Join(project.WorkDir, ".sandstorm", ".generated"),
	})
	if err != nil {
		return nil, err
	}
	return parseGrains(result.Stdout)
}

func (p *Provider) AttachGrain(ctx context.Context, project providers.ProjectContext, grain providers.Grain, helper []byte, checksum string) error {
	targetDir := "/proc/" + strconv.Itoa(grain.ChildPID) + "/root/var/tmp/vagrant-spk"
	targetBin := targetDir + "/enter-grain"
	inject := "sudo mkdir -p " + shellQuote(targetDir) +
		" && if which sha1sum >/dev/null && sudo sha1sum " + shellQuote(targetBin) + " 2>/dev/null | grep -q ^" + checksum + " ; then sudo chmod 755 " + shellQuote(targetBin) + " && exit 0 ; fi" +
		" && sudo dd of=" + shellQuote(targetBin) + " 2>/dev/null" +
		" && sudo chmod 755 " + shellQuote(targetBin)
	if _, err := p.runner.Run(ctx, runner.Spec{
		Name:    "vagrant-inject-enter-grain",
		Command: "vagrant",
		Args:    []string{"ssh", "-c", inject},
		Dir:     filepath.Join(project.WorkDir, ".sandstorm", ".generated"),
		Stdin:   helper,
	}); err != nil {
		return err
	}
	_, err := p.runner.Run(ctx, runner.Spec{
		Name:        "vagrant-attach-grain",
		Command:     "vagrant",
		Args:        []string{"ssh", "-c", "cd /opt/app/.sandstorm && sudo " + shellQuote(targetBin) + " " + strconv.Itoa(grain.ChildPID)},
		Dir:         filepath.Join(project.WorkDir, ".sandstorm", ".generated"),
		Interactive: true,
	})
	return err
}

func (p *Provider) Status(ctx context.Context, project providers.ProjectContext) (providers.Status, error) {
	result, err := p.runner.Run(ctx, runner.Spec{Name: "vagrant-status", Command: "vagrant", Args: []string{"status", "--machine-readable"}, Dir: filepath.Join(project.WorkDir, ".sandstorm", ".generated")})
	status := providers.Status{Provider: p.Name(), InstanceName: p.DetectInstanceName(project.WorkDir), State: "unknown"}
	if err != nil {
		return status, err
	}
	for _, line := range strings.Split(strings.TrimSpace(result.Stdout), "\n") {
		fields := strings.Split(line, ",")
		if len(fields) < 4 {
			continue
		}
		if fields[2] != "state" {
			continue
		}
		if fields[len(fields)-1] != "" {
			status.State = fields[len(fields)-1]
		}
	}
	return status, nil
}

func shellJoin(parts []string) string {
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "&&", "||", ";", "|", ">", ">>", "<":
			quoted = append(quoted, part)
		default:
			quoted = append(quoted, shellQuote(part))
		}
	}
	return strings.Join(quoted, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func renderTemplate(data []byte, values map[string]any) ([]byte, error) {
	tpl, err := template.New("vagrant").Parse(string(data))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := tpl.Execute(&out, values); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

var _ providers.Plugin = (*Provider)(nil)
var _ = os.ModePerm

func parseGrains(stdout string) ([]providers.Grain, error) {
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) == 0 || lines[0] == "" || lines[0] == "no-spk" {
		return nil, &domain.Error{Code: domain.ErrNotFound, Op: "providers.parseGrains", Message: "no Sandstorm supervisor processes found; try `dev` first"}
	}
	var grains []providers.Grain
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) != 4 || fields[1] != "ownership-correct" {
			continue
		}
		childPID, err := strconv.Atoi(fields[3])
		if err != nil {
			return nil, &domain.Error{Code: domain.ErrExternal, Op: "providers.parseGrains", Message: "parse grain child pid", Cause: err}
		}
		grains = append(grains, providers.Grain{SupervisorPID: fields[0], GrainID: fields[2], ChildPID: childPID})
	}
	if len(grains) == 0 {
		return nil, &domain.Error{Code: domain.ErrNotFound, Op: "providers.parseGrains", Message: "no grains found; open a grain in the browser first"}
	}
	return grains, nil
}
