package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mnutt/spktool/internal/domain"
)

func TestLoadAcceptsValidConfig(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	writeConfigFile(t, workDir, ProjectFile, `stack = "meteor"
provider = "lima"

[network.sandstorm]
host = "local.sandstorm.io"
guest_port = 6090
external_port = 6090
localhost_only = true

[providers.vagrant]
box = "debian/bookworm64"

[providers.lima]
vm_type = "qemu"
arch = "x86_64"
image = "https://example.test/debian-amd64.qcow2"
image_arch = "x86_64"
`)

	resolved, err := Load(workDir, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Provider != domain.ProviderLima || resolved.Stack != "meteor" {
		t.Fatalf("unexpected resolved config: %+v", resolved)
	}
	if resolved.Sandstorm.DownloadURL != "" {
		t.Fatalf("expected empty sandstorm download url by default, got %q", resolved.Sandstorm.DownloadURL)
	}
}

func TestLoadPrefersLocalSandstormDownloadURL(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	writeConfigFile(t, workDir, ProjectFile, validProjectConfig())
	writeConfigFile(t, workDir, LocalFile, `provider = "lima"

[sandstorm]
download_url = "https://downloads.example.test/sandstorm-0-fast-1.tar.xz"
`)

	resolved, err := Load(workDir, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Sandstorm.DownloadURL != "https://downloads.example.test/sandstorm-0-fast-1.tar.xz" {
		t.Fatalf("unexpected sandstorm download url: %+v", resolved.Sandstorm)
	}
}

func TestLoadRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		project     string
		local       string
		override    domain.ProviderName
		wantMessage string
	}{
		{
			name:        "invalid provider in project",
			project:     replaceInValidProjectConfig(`provider = "lima"`, `provider = "docker"`),
			wantMessage: "provider must be one of: lima, vagrant",
		},
		{
			name:    "invalid provider in local",
			project: validProjectConfig(),
			local: `provider = "docker"
`,
			wantMessage: "provider must be one of: lima, vagrant",
		},
		{
			name:        "invalid provider override",
			project:     validProjectConfig(),
			override:    domain.ProviderName("docker"),
			wantMessage: "provider must be one of: lima, vagrant",
		},
		{
			name:        "privileged external port",
			project:     replaceInValidProjectConfig(`external_port = 6090`, `external_port = 443`),
			wantMessage: "network.sandstorm.external_port must be 1024 or higher",
		},
		{
			name:        "external port too large",
			project:     replaceInValidProjectConfig(`external_port = 6090`, `external_port = 70000`),
			wantMessage: "network.sandstorm.external_port must be between 1 and 65535",
		},
		{
			name:        "guest port too large",
			project:     replaceInValidProjectConfig(`guest_port = 6090`, `guest_port = 70000`),
			wantMessage: "network.sandstorm.guest_port must be between 1 and 65535",
		},
		{
			name:        "missing host",
			project:     replaceInValidProjectConfig(`host = "local.sandstorm.io"`, `host = "   "`),
			wantMessage: "network.sandstorm.host is required",
		},
		{
			name:        "unspecified host ip",
			project:     replaceInValidProjectConfig(`host = "local.sandstorm.io"`, `host = "0.0.0.0"`),
			wantMessage: "network.sandstorm.host must not be an unspecified address",
		},
		{
			name: "invalid sandstorm download url",
			project: `stack = "meteor"
provider = "lima"

[sandstorm]
download_url = "not-a-url"

[network.sandstorm]
host = "local.sandstorm.io"
guest_port = 6090
external_port = 6090
localhost_only = true

[providers.vagrant]
box = "debian/bookworm64"

[providers.lima]
vm_type = "qemu"
arch = "x86_64"
image = "https://example.test/debian-amd64.qcow2"
image_arch = "x86_64"
`,
			wantMessage: "sandstorm.download_url must be an absolute URL",
		},
		{
			name: "empty vagrant box",
			project: strings.Replace(
				replaceInValidProjectConfig(`provider = "lima"`, `provider = "vagrant"`),
				`box = "debian/bookworm64"`,
				`box = "   "`,
				1,
			),
			wantMessage: "providers.vagrant.box is required",
		},
		{
			name:        "invalid lima vm type",
			project:     replaceInValidProjectConfig(`vm_type = "qemu"`, `vm_type = "hyperkit"`),
			wantMessage: "providers.lima.vm_type must be qemu",
		},
		{
			name:        "invalid lima arch",
			project:     replaceInValidProjectConfig(`arch = "x86_64"`, `arch = "arm64"`),
			wantMessage: "providers.lima.arch must be one of: x86_64, aarch64",
		},
		{
			name:        "missing lima image",
			project:     replaceInValidProjectConfig(`image = "https://example.test/debian-amd64.qcow2"`, `image = "   "`),
			wantMessage: "providers.lima.image is required",
		},
		{
			name:        "invalid lima image arch",
			project:     replaceInValidProjectConfig(`image_arch = "x86_64"`, `image_arch = "arm64"`),
			wantMessage: "providers.lima.image_arch must be one of: x86_64, aarch64",
		},
		{
			name:        "mismatched lima arch and image arch",
			project:     replaceInValidProjectConfig(`image_arch = "x86_64"`, `image_arch = "aarch64"`),
			wantMessage: "providers.lima.arch must match providers.lima.image_arch",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			workDir := t.TempDir()
			writeConfigFile(t, workDir, ProjectFile, tc.project)
			if tc.local != "" {
				writeConfigFile(t, workDir, LocalFile, tc.local)
			}

			_, err := Load(workDir, tc.override, "")
			if err == nil {
				t.Fatal("expected validation error")
			}
			var domainErr *domain.Error
			if !errors.As(err, &domainErr) {
				t.Fatalf("expected domain error, got %T", err)
			}
			if domainErr.Code != domain.ErrInvalidArgument {
				t.Fatalf("unexpected error: %+v", domainErr)
			}
			if !strings.Contains(domainErr.Message, tc.wantMessage) {
				t.Fatalf("expected message %q, got %q", tc.wantMessage, domainErr.Message)
			}
		})
	}
}

func validProjectConfig() string {
	return `stack = "meteor"
provider = "lima"

[network.sandstorm]
host = "local.sandstorm.io"
guest_port = 6090
external_port = 6090
localhost_only = true

[providers.vagrant]
box = "debian/bookworm64"

[providers.lima]
vm_type = "qemu"
arch = "x86_64"
image = "https://example.test/debian-amd64.qcow2"
image_arch = "x86_64"
`
}

func replaceInValidProjectConfig(old, new string) string {
	return strings.Replace(validProjectConfig(), old, new, 1)
}

func writeConfigFile(t *testing.T, workDir, name, body string) {
	t.Helper()
	dir := filepath.Join(workDir, ".sandstorm")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateLocalSandstormDownloadURL(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	writeConfigFile(t, workDir, LocalFile, `provider = "lima"
`)

	if err := UpdateLocalSandstormDownloadURL(workDir, "https://downloads.example.test/sandstorm-0-fast-1.tar.xz"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(workDir, ".sandstorm", LocalFile))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, `provider = "lima"`) {
		t.Fatalf("expected provider to be preserved, got:\n%s", body)
	}
	if !strings.Contains(body, `download_url = "https://downloads.example.test/sandstorm-0-fast-1.tar.xz"`) {
		t.Fatalf("expected download url to be written, got:\n%s", body)
	}
}

func TestUpdateLocalSandstormPorts(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	writeConfigFile(t, workDir, LocalFile, `provider = "lima"

[sandstorm]
download_url = "https://downloads.example.test/sandstorm-0-fast-1.tar.xz"
`)

	if err := UpdateLocalSandstormPorts(workDir, 7000); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(workDir, ".sandstorm", LocalFile))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, `provider = "lima"`) {
		t.Fatalf("expected provider to be preserved, got:\n%s", body)
	}
	if !strings.Contains(body, `download_url = "https://downloads.example.test/sandstorm-0-fast-1.tar.xz"`) {
		t.Fatalf("expected download url to be preserved, got:\n%s", body)
	}
	if !strings.Contains(body, "guest_port = 7000") {
		t.Fatalf("expected guest port to be written, got:\n%s", body)
	}
	if !strings.Contains(body, "external_port = 7000") {
		t.Fatalf("expected external port to be written, got:\n%s", body)
	}
}
