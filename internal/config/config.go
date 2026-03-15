package config

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pelletier/go-toml/v2"

	"github.com/mnutt/spktool/internal/domain"
	"github.com/mnutt/spktool/internal/templates"
)

const (
	ProjectFile = "box.toml"
	LocalFile   = "box.local.toml"
)

type ProjectFileConfig struct {
	Stack     string              `toml:"stack"`
	Provider  string              `toml:"provider"`
	Network   NetworkFileConfig   `toml:"network"`
	Providers ProvidersFileConfig `toml:"providers"`
}

type LocalFileConfig struct {
	Provider  string              `toml:"provider"`
	Network   NetworkFileConfig   `toml:"network"`
	Providers ProvidersFileConfig `toml:"providers"`
}

type NetworkFileConfig struct {
	Sandstorm SandstormFileConfig `toml:"sandstorm"`
}

type SandstormFileConfig struct {
	Host          string `toml:"host"`
	GuestPort     int    `toml:"guest_port"`
	ExternalPort  int    `toml:"external_port"`
	LocalhostOnly *bool  `toml:"localhost_only"`
}

type ProvidersFileConfig struct {
	Vagrant VagrantFileConfig `toml:"vagrant"`
	Lima    LimaFileConfig    `toml:"lima"`
}

type VagrantFileConfig struct {
	Box string `toml:"box"`
}

type LimaFileConfig struct {
	VMType    string `toml:"vm_type"`
	Arch      string `toml:"arch"`
	Image     string `toml:"image"`
	ImageArch string `toml:"image_arch"`
}

type Resolved struct {
	Stack    string
	Provider domain.ProviderName
	Network  NetworkResolved
	Vagrant  VagrantResolved
	Lima     LimaResolved
}

type NetworkResolved struct {
	Sandstorm SandstormResolved
}

type SandstormResolved struct {
	Host          string
	GuestPort     int
	ExternalPort  int
	LocalhostOnly bool
}

type VagrantResolved struct {
	Box string
}

type LimaResolved struct {
	VMType    string
	Arch      string
	Image     string
	ImageArch string
}

var (
	supportedProviders = map[domain.ProviderName]struct{}{
		domain.ProviderLima:    {},
		domain.ProviderVagrant: {},
	}
	supportedLimaVMTypes = map[string]struct{}{
		"qemu": {},
		"vz":   {},
	}
	supportedLimaArch = map[string]struct{}{
		"x86_64":  {},
		"aarch64": {},
	}
)

var (
	limaDefaultsOnce sync.Once
	limaDefaults     LimaResolved
	limaDefaultsErr  error
)

func Load(workDir string, providerOverride domain.ProviderName, defaultProvider domain.ProviderName) (*Resolved, error) {
	projectPath := filepath.Join(workDir, ".sandstorm", ProjectFile)
	projectData, err := os.ReadFile(projectPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, &domain.Error{Code: domain.ErrNotFound, Op: "config.Load", Message: "no .sandstorm/box.toml found; run setupvm first", Cause: err}
		}
		return nil, domain.Wrap(domain.ErrExternal, "config.Load", "read project config", err)
	}

	var project ProjectFileConfig
	if err := decode(projectData, &project); err != nil {
		return nil, &domain.Error{Code: domain.ErrInvalidArgument, Op: "config.Load", Message: fmt.Sprintf("parse .sandstorm/%s: %v", ProjectFile, err)}
	}

	var local LocalFileConfig
	localPath := filepath.Join(workDir, ".sandstorm", LocalFile)
	localData, err := os.ReadFile(localPath)
	if err == nil {
		if err := decode(localData, &local); err != nil {
			return nil, &domain.Error{Code: domain.ErrInvalidArgument, Op: "config.Load", Message: fmt.Sprintf("parse .sandstorm/%s: %v", LocalFile, err)}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, domain.Wrap(domain.ErrExternal, "config.Load", "read local config", err)
	}

	resolved := defaults()
	if project.Stack != "" {
		resolved.Stack = project.Stack
	}
	mergeNetwork(&resolved.Network, project.Network)
	mergeProviders(&resolved.Vagrant, &resolved.Lima, project.Providers)

	mergeNetwork(&resolved.Network, local.Network)
	mergeProviders(&resolved.Vagrant, &resolved.Lima, local.Providers)

	switch {
	case providerOverride != "":
		resolved.Provider = providerOverride
	case local.Provider != "":
		resolved.Provider = domain.ProviderName(local.Provider)
	case project.Provider != "":
		resolved.Provider = domain.ProviderName(project.Provider)
	default:
		resolved.Provider = DetectProvider(defaultProvider)
	}

	if resolved.Stack == "" {
		return nil, &domain.Error{Code: domain.ErrInvalidArgument, Op: "config.Load", Message: "stack is required in .sandstorm/box.toml"}
	}
	if resolved.Network.Sandstorm.Host == "" {
		return nil, &domain.Error{Code: domain.ErrInvalidArgument, Op: "config.Load", Message: "network.sandstorm.host is required"}
	}
	if resolved.Network.Sandstorm.GuestPort <= 0 {
		return nil, &domain.Error{Code: domain.ErrInvalidArgument, Op: "config.Load", Message: "network.sandstorm.guest_port must be positive"}
	}
	if resolved.Network.Sandstorm.ExternalPort <= 0 {
		return nil, &domain.Error{Code: domain.ErrInvalidArgument, Op: "config.Load", Message: "network.sandstorm.external_port must be positive"}
	}
	if resolved.Provider == "" {
		return nil, &domain.Error{Code: domain.ErrNotFound, Op: "config.Load", Message: "provider is not configured and could not be autodetected"}
	}
	if err := validateResolved(&resolved); err != nil {
		return nil, err
	}
	return &resolved, nil
}

func InitialProject(stack string) []byte {
	lima := mustLimaDefaults()
	return []byte(fmt.Sprintf(`stack = %q

[network.sandstorm]
host = "local.sandstorm.io"
guest_port = 6090
external_port = 6090
localhost_only = true

[providers.vagrant]
box = "debian/bookworm64"

[providers.lima]
vm_type = %q
arch = %q
image = %q
image_arch = %q
`, stack, lima.VMType, lima.Arch, lima.Image, lima.ImageArch))
}

func InitialLocal(provider domain.ProviderName) []byte {
	return []byte(fmt.Sprintf("provider = %q\n", provider))
}

func LegacyResolved(stack string, provider domain.ProviderName) *Resolved {
	resolved := defaults()
	resolved.Stack = stack
	resolved.Provider = provider
	return &resolved
}

func decode(data []byte, target any) error {
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(target)
}

func defaults() Resolved {
	lima := mustLimaDefaults()
	return Resolved{
		Network: NetworkResolved{
			Sandstorm: SandstormResolved{
				Host:          "local.sandstorm.io",
				GuestPort:     6090,
				ExternalPort:  6090,
				LocalhostOnly: true,
			},
		},
		Vagrant: VagrantResolved{Box: "debian/bookworm64"},
		Lima:    lima,
	}
}

func mergeNetwork(dst *NetworkResolved, src NetworkFileConfig) {
	if src.Sandstorm.Host != "" {
		dst.Sandstorm.Host = src.Sandstorm.Host
	}
	if src.Sandstorm.GuestPort != 0 {
		dst.Sandstorm.GuestPort = src.Sandstorm.GuestPort
	}
	if src.Sandstorm.ExternalPort != 0 {
		dst.Sandstorm.ExternalPort = src.Sandstorm.ExternalPort
	}
	if src.Sandstorm.LocalhostOnly != nil {
		dst.Sandstorm.LocalhostOnly = *src.Sandstorm.LocalhostOnly
	}
}

func mergeProviders(vagrant *VagrantResolved, lima *LimaResolved, src ProvidersFileConfig) {
	if src.Vagrant.Box != "" {
		vagrant.Box = src.Vagrant.Box
	}
	if src.Lima.VMType != "" {
		lima.VMType = src.Lima.VMType
	}
	if src.Lima.Arch != "" {
		lima.Arch = src.Lima.Arch
	}
	if src.Lima.Image != "" {
		lima.Image = src.Lima.Image
	}
	if src.Lima.ImageArch != "" {
		lima.ImageArch = src.Lima.ImageArch
	}
}

func DetectProvider(defaultProvider domain.ProviderName) domain.ProviderName {
	available := make([]domain.ProviderName, 0, 2)
	if _, err := exec.LookPath("limactl"); err == nil {
		available = append(available, domain.ProviderLima)
	}
	if _, err := exec.LookPath("vagrant"); err == nil {
		available = append(available, domain.ProviderVagrant)
	}
	if len(available) == 1 {
		return available[0]
	}
	if defaultProvider != "" {
		for _, provider := range available {
			if provider == defaultProvider {
				return provider
			}
		}
	}
	if len(available) > 0 {
		return available[0]
	}
	return ""
}

func WildcardHost(host string, externalPort int) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	wildcard := "*." + host
	if externalPort != 80 && externalPort > 0 {
		wildcard += ":" + fmt.Sprintf("%d", externalPort)
	}
	return wildcard
}

func mustLimaDefaults() LimaResolved {
	limaDefaultsOnce.Do(func() {
		data, err := templates.New().BoxFile("lima-defaults.toml")
		if err != nil {
			limaDefaultsErr = err
			return
		}

		var file LimaFileConfig
		if err := decode(data, &file); err != nil {
			limaDefaultsErr = err
			return
		}

		limaDefaults = LimaResolved{
			VMType:    file.VMType,
			Arch:      file.Arch,
			Image:     file.Image,
			ImageArch: file.ImageArch,
		}
	})
	if limaDefaultsErr != nil {
		panic(fmt.Sprintf("load lima defaults: %v", limaDefaultsErr))
	}
	return limaDefaults
}

func validateResolved(resolved *Resolved) error {
	if _, ok := supportedProviders[resolved.Provider]; !ok {
		return &domain.Error{Code: domain.ErrInvalidArgument, Op: "config.Load", Message: fmt.Sprintf("provider must be one of: %s, %s", domain.ProviderLima, domain.ProviderVagrant)}
	}
	if resolved.Stack == "" {
		return &domain.Error{Code: domain.ErrInvalidArgument, Op: "config.Load", Message: "stack is required in .sandstorm/box.toml"}
	}
	if strings.TrimSpace(resolved.Network.Sandstorm.Host) == "" {
		return &domain.Error{Code: domain.ErrInvalidArgument, Op: "config.Load", Message: "network.sandstorm.host is required"}
	}
	if resolved.Network.Sandstorm.GuestPort <= 0 || resolved.Network.Sandstorm.GuestPort > 65535 {
		return &domain.Error{Code: domain.ErrInvalidArgument, Op: "config.Load", Message: "network.sandstorm.guest_port must be between 1 and 65535"}
	}
	if resolved.Network.Sandstorm.ExternalPort <= 0 || resolved.Network.Sandstorm.ExternalPort > 65535 {
		return &domain.Error{Code: domain.ErrInvalidArgument, Op: "config.Load", Message: "network.sandstorm.external_port must be between 1 and 65535"}
	}
	if resolved.Network.Sandstorm.ExternalPort < 1024 {
		return &domain.Error{Code: domain.ErrInvalidArgument, Op: "config.Load", Message: "network.sandstorm.external_port must be 1024 or higher"}
	}
	if ip := net.ParseIP(resolved.Network.Sandstorm.Host); ip != nil && ip.IsUnspecified() {
		return &domain.Error{Code: domain.ErrInvalidArgument, Op: "config.Load", Message: "network.sandstorm.host must not be an unspecified address"}
	}
	if strings.TrimSpace(resolved.Vagrant.Box) == "" {
		return &domain.Error{Code: domain.ErrInvalidArgument, Op: "config.Load", Message: "providers.vagrant.box is required"}
	}
	if _, ok := supportedLimaVMTypes[resolved.Lima.VMType]; !ok {
		return &domain.Error{Code: domain.ErrInvalidArgument, Op: "config.Load", Message: "providers.lima.vm_type must be one of: qemu, vz"}
	}
	if _, ok := supportedLimaArch[resolved.Lima.Arch]; !ok {
		return &domain.Error{Code: domain.ErrInvalidArgument, Op: "config.Load", Message: "providers.lima.arch must be one of: x86_64, aarch64"}
	}
	if strings.TrimSpace(resolved.Lima.Image) == "" {
		return &domain.Error{Code: domain.ErrInvalidArgument, Op: "config.Load", Message: "providers.lima.image is required"}
	}
	if _, ok := supportedLimaArch[resolved.Lima.ImageArch]; !ok {
		return &domain.Error{Code: domain.ErrInvalidArgument, Op: "config.Load", Message: "providers.lima.image_arch must be one of: x86_64, aarch64"}
	}
	if resolved.Lima.Arch != resolved.Lima.ImageArch {
		return &domain.Error{Code: domain.ErrInvalidArgument, Op: "config.Load", Message: "providers.lima.arch must match providers.lima.image_arch"}
	}
	return nil
}
