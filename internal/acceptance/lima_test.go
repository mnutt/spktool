//go:build acceptance

package acceptance_test

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
)

var acceptanceMu sync.Mutex

func TestLimaLifecycleAcceptance(t *testing.T) {
	acceptanceMu.Lock()
	t.Cleanup(acceptanceMu.Unlock)

	if os.Getenv("SPKTOOL_ACCEPTANCE_LIMA") != "1" {
		t.Skip("set SPKTOOL_ACCEPTANCE_LIMA=1 to enable real Lima acceptance tests")
	}
	if _, err := exec.LookPath("limactl"); err != nil {
		t.Skip("limactl is not installed")
	}

	workDir := mustMkdirTempInRepo(t, "acceptance-lima-")
	binPath := buildBinary(t)

	runSpktool(t, binPath, "--work-directory", workDir, "setupvm", "node", "--provider", "lima")

	runtimeEnvPath := filepath.Join(workDir, ".sandstorm", ".generated", "runtime.env")
	runtimeEnvBytes, err := os.ReadFile(runtimeEnvPath)
	if err != nil {
		t.Fatalf("expected runtime.env to exist after setupvm: %v", err)
	}
	runtimeEnv := string(runtimeEnvBytes)
	if !strings.Contains(runtimeEnv, "SANDSTORM_EXTERNAL_PORT=6090") {
		t.Fatalf("runtime.env missing external port:\n%s", runtimeEnv)
	}
	if !strings.Contains(runtimeEnv, "SANDSTORM_WILDCARD_HOST=*.local.sandstorm.io:6090") {
		t.Fatalf("runtime.env missing wildcard host port:\n%s", runtimeEnv)
	}

	render := runSpktool(t, binPath, "--work-directory", workDir, "config", "render", "--provider", "lima")
	if !strings.Contains(render, "mountType: reverse-sshfs") {
		t.Fatalf("config render missing expected Lima mount type:\n%s", render)
	}
	if !strings.Contains(render, "== .sandstorm/.generated/runtime.env ==") {
		t.Fatalf("config render missing runtime.env section:\n%s", render)
	}
	if !strings.Contains(render, "SANDSTORM_BASE_URL=http://local.sandstorm.io:6090") {
		t.Fatalf("config render missing runtime.env contents:\n%s", render)
	}
	if !strings.Contains(render, "SANDSTORM_WILDCARD_HOST=*.local.sandstorm.io:6090") {
		t.Fatalf("config render missing wildcard host port:\n%s", render)
	}

	instanceName := detectInstanceNameFromWorkDir(workDir)
	t.Logf("acceptance workdir: %s", workDir)
	t.Logf("acceptance lima instance: %s", instanceName)

	t.Cleanup(func() {
		runCommandBestEffort(t, repoRoot(t), nil, "limactl", "stop", "--force", instanceName)
		runCommandBestEffort(t, repoRoot(t), nil, "limactl", "delete", "--force", instanceName)
	})

	runSpktoolStream(t, binPath, "--work-directory", workDir, "vm", "create", "--provider", "lima")
	instanceName = detectInstanceName(t, binPath, workDir, "lima")

	mounted := runCommand(t, workDir, nil, "limactl", "shell", instanceName, "sh", "-lc", "test -d /opt/app/.sandstorm && echo mounted")
	if !strings.Contains(mounted, "mounted") {
		t.Fatalf("project mount missing inside Lima guest:\n%s", mounted)
	}

	guestRuntimeEnv := runCommand(t, workDir, nil, "limactl", "shell", instanceName, "sh", "-lc", "grep '^SANDSTORM_EXTERNAL_PORT=6090$' /opt/app/.sandstorm/.generated/runtime.env")
	if !strings.Contains(guestRuntimeEnv, "SANDSTORM_EXTERNAL_PORT=6090") {
		t.Fatalf("runtime.env not visible in guest:\n%s", guestRuntimeEnv)
	}

	serviceState := runCommand(t, workDir, nil, "limactl", "shell", instanceName, "sh", "-lc", "systemctl is-active sandstorm")
	if !strings.Contains(strings.TrimSpace(serviceState), "active") {
		t.Fatalf("sandstorm service is not active:\n%s", serviceState)
	}

	runSpktoolStream(t, binPath, "--work-directory", workDir, "init", "--provider", "lima")

	if _, err := os.Stat(filepath.Join(workDir, ".sandstorm", "sandstorm-pkgdef.capnp")); err != nil {
		t.Fatalf("expected sandstorm-pkgdef.capnp to exist: %v", err)
	}
}

func TestVagrantLifecycleAcceptance(t *testing.T) {
	acceptanceMu.Lock()
	t.Cleanup(acceptanceMu.Unlock)

	if os.Getenv("SPKTOOL_ACCEPTANCE_VAGRANT") != "1" {
		t.Skip("set SPKTOOL_ACCEPTANCE_VAGRANT=1 to enable real Vagrant acceptance tests")
	}
	if _, err := exec.LookPath("vagrant"); err != nil {
		t.Skip("vagrant is not installed")
	}

	workDir := mustMkdirTempInRepo(t, "acceptance-vagrant-")
	binPath := buildBinary(t)

	runSpktool(t, binPath, "--work-directory", workDir, "setupvm", "node", "--provider", "vagrant")

	runtimeEnvPath := filepath.Join(workDir, ".sandstorm", ".generated", "runtime.env")
	runtimeEnvBytes, err := os.ReadFile(runtimeEnvPath)
	if err != nil {
		t.Fatalf("expected runtime.env to exist after setupvm: %v", err)
	}
	runtimeEnv := string(runtimeEnvBytes)
	if !strings.Contains(runtimeEnv, "SANDSTORM_EXTERNAL_PORT=6090") {
		t.Fatalf("runtime.env missing external port:\n%s", runtimeEnv)
	}

	render := runSpktool(t, binPath, "--work-directory", workDir, "config", "render", "--provider", "vagrant")
	if !strings.Contains(render, "== .sandstorm/.generated/Vagrantfile ==") {
		t.Fatalf("config render missing expected Vagrantfile section:\n%s", render)
	}
	if !strings.Contains(render, "== .sandstorm/.generated/runtime.env ==") {
		t.Fatalf("config render missing runtime.env section:\n%s", render)
	}
	if !strings.Contains(render, "SANDSTORM_WILDCARD_HOST=*.local.sandstorm.io:6090") {
		t.Fatalf("config render missing wildcard host port:\n%s", render)
	}
	if !strings.Contains(render, `"/host-dot-sandstorm"`) {
		t.Fatalf("config render missing host-dot-sandstorm mount:\n%s", render)
	}

	t.Cleanup(func() {
		runSpktoolBestEffort(t, binPath, "--work-directory", workDir, "vm", "destroy", "--provider", "vagrant")
	})

	runSpktoolStream(t, binPath, "--work-directory", workDir, "vm", "create", "--provider", "vagrant")

	instanceName := detectInstanceName(t, binPath, workDir, "vagrant")
	if instanceName == "" {
		t.Fatal("vm status did not return a Vagrant instance name")
	}

	guestRuntimeEnv := runSpktool(t, binPath, "--work-directory", workDir, "vm", "ssh", "--provider", "vagrant", "-c", "grep '^SANDSTORM_EXTERNAL_PORT=6090$' /opt/app/.sandstorm/.generated/runtime.env")
	if !strings.Contains(guestRuntimeEnv, "SANDSTORM_EXTERNAL_PORT=6090") {
		t.Fatalf("runtime.env not visible in guest:\n%s", guestRuntimeEnv)
	}

	hostMount := runSpktool(t, binPath, "--work-directory", workDir, "vm", "ssh", "--provider", "vagrant", "-c", "test -d /host-dot-sandstorm && echo mounted")
	if !strings.Contains(hostMount, "mounted") {
		t.Fatalf("host-dot-sandstorm mount missing inside guest:\n%s", hostMount)
	}

	runSpktoolStream(t, binPath, "--work-directory", workDir, "init", "--provider", "vagrant")

	if _, err := os.Stat(filepath.Join(workDir, ".sandstorm", "sandstorm-pkgdef.capnp")); err != nil {
		t.Fatalf("expected sandstorm-pkgdef.capnp to exist: %v", err)
	}
}

func mustMkdirTempInRepo(t *testing.T, prefix string) string {
	t.Helper()

	repoRoot := repoRoot(t)
	workDir, err := os.MkdirTemp(repoRoot, prefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(workDir)
	})
	return workDir
}

func buildBinary(t *testing.T) string {
	t.Helper()

	repoRoot := repoRoot(t)
	binPath := filepath.Join(t.TempDir(), "spktool")
	runCommand(t, repoRoot, append(os.Environ(), "GOCACHE=/tmp/go-build"), "go", "build", "-o", binPath, "./cmd/spktool")
	return binPath
}

func detectInstanceName(t *testing.T, binPath, workDir, provider string) string {
	t.Helper()

	result := runSpktool(t, binPath, "--work-directory", workDir, "--output", "json", "vm", "status", "--provider", provider)
	var payload struct {
		OK     bool `json:"ok"`
		Result struct {
			InstanceName string `json:"instanceName"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("failed to parse vm status json: %v\n%s", err, result)
	}
	if !payload.OK || payload.Result.InstanceName == "" {
		t.Fatalf("vm status missing instanceName:\n%s", result)
	}
	return payload.Result.InstanceName
}

func runSpktool(t *testing.T, binPath string, args ...string) string {
	t.Helper()

	return runCommand(t, repoRoot(t), nil, binPath, args...)
}

func runSpktoolStream(t *testing.T, binPath string, args ...string) string {
	t.Helper()

	return runCommandStreaming(t, repoRoot(t), nil, binPath, args...)
}

func runSpktoolBestEffort(t *testing.T, binPath string, args ...string) {
	t.Helper()

	runCommandBestEffort(t, repoRoot(t), nil, binPath, args...)
}

func runCommandBestEffort(t *testing.T, dir string, extraEnv []string, name string, args ...string) {
	t.Helper()

	t.Logf("cleanup: %s %s", name, strings.Join(args, " "))

	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extraEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("cleanup command failed: %v\n%s", err, out)
	}
}

func runCommand(t *testing.T, dir string, extraEnv []string, name string, args ...string) string {
	t.Helper()

	t.Logf("running: %s %s", name, strings.Join(args, " "))

	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extraEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

func runCommandStreaming(t *testing.T, dir string, extraEnv []string, name string, args ...string) string {
	t.Helper()

	t.Logf("running: %s %s", name, strings.Join(args, " "))

	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extraEnv...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe failed for %s %s: %v", name, strings.Join(args, " "), err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe failed for %s %s: %v", name, strings.Join(args, " "), err)
	}

	var buf bytes.Buffer
	if err := cmd.Start(); err != nil {
		t.Fatalf("%s %s failed to start: %v", name, strings.Join(args, " "), err)
	}

	done := make(chan struct{}, 2)
	stream := func(label string, r io.Reader) {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			line := scanner.Text()
			buf.WriteString(line)
			buf.WriteByte('\n')
			t.Logf("[%s] %s", label, line)
		}
		if err := scanner.Err(); err != nil {
			t.Logf("[%s] stream error: %v", label, err)
		}
		done <- struct{}{}
	}

	go stream("stdout", stdout)
	go stream("stderr", stderr)

	<-done
	<-done

	if err := cmd.Wait(); err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, buf.String())
	}
	return buf.String()
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve test file location")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func detectInstanceNameFromWorkDir(workDir string) string {
	sum := md5.Sum([]byte(filepath.Clean(workDir)))
	hash := hex.EncodeToString(sum[:])[:8]
	base := regexp.MustCompile(`[^a-zA-Z0-9-]`).ReplaceAllString(filepath.Base(workDir), "-")
	return "sandstorm-" + base + "-" + hash
}
