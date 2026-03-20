package lima

import (
	"bufio"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/mnutt/spktool/internal/domain"
	"github.com/mnutt/spktool/internal/providers"
	"github.com/mnutt/spktool/internal/runner"
	"github.com/mnutt/spktool/internal/templates"
)

type Provider struct {
	runner    runner.Runner
	templates *templates.Repository
}

const forcedVMType = "qemu"

var lookPath = exec.LookPath

func New(r runner.Runner, repo *templates.Repository) *Provider {
	return &Provider{runner: r, templates: repo}
}

func (p *Provider) Name() domain.ProviderName { return domain.ProviderLima }

func (p *Provider) BootstrapFiles(project providers.ProjectContext) ([]providers.RenderedFile, error) {
	if project.Config == nil {
		return nil, &domain.Error{Code: domain.ErrInvalidArgument, Op: "lima.BootstrapFiles", Message: "resolved config is required"}
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, domain.Wrap(domain.ErrExternal, "lima.BootstrapFiles", "resolve home directory", err)
	}
	hostIP := ""
	if project.Config.Network.Sandstorm.LocalhostOnly {
		hostIP = "\n    hostIP: 127.0.0.1"
	}
	mountType := defaultMountType()
	body := []byte(fmt.Sprintf(`arch: %s
vmType: %s
mountType: %s
images:
  - location: %q
    arch: %q
containerd:
  system: false
  user: false
mounts:
  - location: %q
    mountPoint: /opt/app
    writable: true
  - location: %q
    mountPoint: /host-dot-sandstorm
    writable: true
portForwards:
  - guestPort: %d
    hostPort: %d%s
  - guestIP: "127.0.0.1"
    proto: "any"
    guestPortRange: [1, 65535]
    ignore: true
`, project.Config.Lima.Arch, forcedVMType, mountType, project.Config.Lima.Image, project.Config.Lima.ImageArch, project.WorkDir, filepath.Join(homeDir, ".sandstorm"), project.Config.Network.Sandstorm.GuestPort, project.Config.Network.Sandstorm.ExternalPort, hostIP))
	return []providers.RenderedFile{{
		Path: filepath.Join(".sandstorm", ".generated", "lima.yaml"),
		Body: body,
		Mode: 0o644,
	}}, nil
}

func (p *Provider) DetectInstanceName(workDir string) string {
	sum := md5.Sum([]byte(filepath.Clean(workDir)))
	hash := hex.EncodeToString(sum[:])[:8]
	base := regexp.MustCompile(`[^a-zA-Z0-9-]`).ReplaceAllString(filepath.Base(workDir), "-")
	return "sandstorm-" + base + "-" + hash
}

func (p *Provider) Up(ctx context.Context, project providers.ProjectContext) error {
	instance := p.DetectInstanceName(project.WorkDir)
	status, err := p.Status(ctx, project)
	if err != nil {
		return err
	}
	stopTail := func() {}
	if project.Verbose {
		stopTail = startSerialTail(instance)
	}
	defer stopTail()
	args := []string{"start", "--name", instance, "--tty=false", "--progress", filepath.Join(project.WorkDir, ".sandstorm", ".generated", "lima.yaml")}
	if limaInstanceExists(status.State) {
		args = []string{"start", "--tty=false", instance}
	}
	_, err = p.runner.Run(ctx, runner.Spec{Name: "lima-start", Command: "limactl", Args: args, Stream: true})
	return err
}

func (p *Provider) Halt(ctx context.Context, project providers.ProjectContext) error {
	_, err := p.runner.Run(ctx, runner.Spec{Name: "lima-stop", Command: "limactl", Args: []string{"stop", p.DetectInstanceName(project.WorkDir)}})
	return err
}

func (p *Provider) Destroy(ctx context.Context, project providers.ProjectContext) error {
	_, err := p.runner.Run(ctx, runner.Spec{Name: "lima-delete", Command: "limactl", Args: []string{"delete", "--force", p.DetectInstanceName(project.WorkDir)}})
	return err
}

func (p *Provider) SSH(ctx context.Context, project providers.ProjectContext, args []string) error {
	argv := []string{"shell"}
	if len(args) == 0 {
		argv = append(argv, "--workdir", "/opt/app")
	}
	argv = append(argv, p.DetectInstanceName(project.WorkDir))
	argv = append(argv, args...)
	spec := runner.Spec{Name: "lima-shell", Command: "limactl", Args: argv}
	if len(args) == 0 {
		spec.Interactive = true
	} else {
		spec.Stream = true
	}
	_, err := p.runner.Run(ctx, spec)
	return err
}

func (p *Provider) Exec(ctx context.Context, project providers.ProjectContext, command []string) (runner.Result, error) {
	argv := append([]string{"shell", "--workdir", "/opt/app", p.DetectInstanceName(project.WorkDir), "bash", "-lc"}, shellJoin(command))
	return p.runner.Run(ctx, runner.Spec{Name: "lima-exec", Command: "limactl", Args: argv})
}

func (p *Provider) ExecInteractive(ctx context.Context, project providers.ProjectContext, command []string) error {
	_, err := p.runner.Run(ctx, runner.Spec{
		Name:        "lima-exec-interactive",
		Command:     "limactl",
		Args:        append([]string{"shell", "--workdir", "/opt/app", p.DetectInstanceName(project.WorkDir), "bash", "-lc"}, shellJoin(command)),
		Interactive: true,
	})
	return err
}

func (p *Provider) WriteFile(ctx context.Context, project providers.ProjectContext, file providers.RenderedFile) error {
	command := []string{
		"mkdir", "-p", filepath.Dir(file.Path),
		"&&", "chmod", "755", filepath.Dir(file.Path),
		"&&", "cat", ">", file.Path,
		"&&", "chmod", strconv.FormatUint(uint64(file.Mode), 8), file.Path,
	}
	_, err := p.runner.Run(ctx, runner.Spec{
		Name:    "lima-write-file",
		Command: "limactl",
		Args:    append([]string{"shell", "--workdir", "/opt/app", p.DetectInstanceName(project.WorkDir), "bash", "-lc"}, shellJoin(command)),
		Stdin:   file.Body,
	})
	return err
}

func (p *Provider) Provision(ctx context.Context, project providers.ProjectContext) error {
	_, err := p.runner.Run(ctx, runner.Spec{
		Name:    "lima-provision",
		Command: "limactl",
		Args: []string{
			"shell",
			"--workdir",
			"/opt/app/.sandstorm",
			p.DetectInstanceName(project.WorkDir),
			"sudo",
			"bash",
			"-lc",
			"./global-setup.sh && ./setup.sh",
		},
		Stream: true,
	})
	return err
}

func (p *Provider) ListGrains(ctx context.Context, project providers.ProjectContext) ([]providers.Grain, error) {
	result, err := p.runner.Run(ctx, runner.Spec{
		Name:    "lima-list-grains",
		Command: "limactl",
		Args:    []string{"shell", "--workdir", "/opt/app", p.DetectInstanceName(project.WorkDir), "bash", "-lc", `pidof spk || echo no-spk;SANDSTORM_UID=$(id sandstorm | sed -r s,[^0-9],_,g | sed -r s,_+,_,g | cut -d _ -f 2 ) ; for pid in $(pidof supervisor); do echo $pid $(cat /proc/$pid/status | grep -q 'Uid:.*'${SANDSTORM_UID} && echo ownership-correct || echo ownership-wrong) $(xargs -0 -n1 echo < /proc/$pid/cmdline  | grep -v -- - | head -n2 | tail -n1) $(grep -E -l ^PPid:[[:blank:]]*${pid}$ /proc/*/status | head -n1  | sed -r s,/proc/,,g | sed -r s,/status,,) ; done`},
	})
	if err != nil {
		return nil, err
	}
	return parseGrains(result.Stdout)
}

func (p *Provider) AttachGrain(ctx context.Context, project providers.ProjectContext, grain providers.Grain, helper []byte, checksum string) error {
	targetDir := "/proc/" + strconv.Itoa(grain.ChildPID) + "/root/var/tmp/lima-spk"
	targetBin := targetDir + "/enter-grain"
	inject := "sudo mkdir -p " + shellQuote(targetDir) +
		" && if which sha1sum >/dev/null && sudo sha1sum " + shellQuote(targetBin) + " 2>/dev/null | grep -q ^" + checksum + " ; then sudo chmod 755 " + shellQuote(targetBin) + " && exit 0 ; fi" +
		" && sudo dd of=" + shellQuote(targetBin) + " 2>/dev/null" +
		" && sudo chmod 755 " + shellQuote(targetBin)
	if _, err := p.runner.Run(ctx, runner.Spec{
		Name:    "lima-inject-enter-grain",
		Command: "limactl",
		Args:    []string{"shell", "--workdir", "/opt/app", p.DetectInstanceName(project.WorkDir), "bash", "-lc", inject},
		Stdin:   helper,
	}); err != nil {
		return err
	}
	_, err := p.runner.Run(ctx, runner.Spec{
		Name:        "lima-attach-grain",
		Command:     "limactl",
		Args:        []string{"shell", "--workdir", "/opt/app", p.DetectInstanceName(project.WorkDir), "bash", "-lc", "cd /opt/app/.sandstorm && sudo " + shellQuote(targetBin) + " " + strconv.Itoa(grain.ChildPID)},
		Interactive: true,
	})
	return err
}

func (p *Provider) Status(ctx context.Context, project providers.ProjectContext) (providers.Status, error) {
	result, err := p.runner.Run(ctx, runner.Spec{Name: "lima-list", Command: "limactl", Args: []string{"list", "--json"}})
	status := providers.Status{Provider: p.Name(), InstanceName: p.DetectInstanceName(project.WorkDir), State: "unknown"}
	if err != nil {
		return status, err
	}
	scanner := bufio.NewScanner(strings.NewReader(result.Stdout))
	for scanner.Scan() {
		var item struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		}
		if json.Unmarshal(scanner.Bytes(), &item) != nil {
			continue
		}
		if item.Name != status.InstanceName {
			continue
		}
		if item.Status != "" {
			status.State = strings.ToLower(item.Status)
		}
		return status, nil
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return status, scanErr
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

func defaultMountType() string {
	if runtime.GOOS == "darwin" {
		return "9p"
	}
	if _, err := lookPath("virtiofsd"); err == nil {
		return "virtiofs"
	}
	return "9p"
}

func limaInstanceExists(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "", "unknown", "not_created", "not created":
		return false
	default:
		return true
	}
}

var _ providers.Plugin = (*Provider)(nil)

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
