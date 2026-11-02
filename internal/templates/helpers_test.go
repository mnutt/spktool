//go:build linux

package templates

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Each helper gets its own process group. A successful shutdown must leave no
// workers in that group, including tails whose immediate parent has exited.
func TestHelperShutdown(t *testing.T) {
	for _, mode := range []string{"tailer-term", "wrapper-term", "wrapper-int", "wrapper-exit"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			grains := filepath.Join(dir, "grains")
			for _, id := range []string{"grain-one", "grain-two"} {
				path := filepath.Join(grains, id)
				if err := os.MkdirAll(path, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(path, "log"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			write := func(name, body string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0700); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range []string{"grain-log-tailer.sh", "dev-with-tail.sh"} {
				body, err := New().HelperFile(name)
				if err != nil {
					t.Fatal(err)
				}
				write(name, strings.ReplaceAll(string(body), "/opt/sandstorm/var/sandstorm/grains", grains))
			}
			// Replace sudo without needing a sandstorm account or elevated privileges.
			write("sudo", "#!/bin/bash\nshift 2\nexec \"$@\"\n")
			realTail, err := exec.LookPath("tail")
			if err != nil {
				t.Fatal(err)
			}
			write("tail", fmt.Sprintf("#!/bin/bash\necho ready >> %q\nexec %q \"$@\"\n", filepath.Join(dir, "ready"), realTail))
			// A FIFO lets the test choose exactly when the wrapped command exits.
			fifo := filepath.Join(dir, "finish")
			if err := syscall.Mkfifo(fifo, 0600); err != nil {
				t.Fatal(err)
			}
			write("command", "#!/bin/bash\nread -r line < \"$1\"\nexit 7\n")
			args := []string{filepath.Join(dir, "grain-log-tailer.sh")}
			if mode != "tailer-term" {
				args = []string{filepath.Join(dir, "dev-with-tail.sh"), "--", filepath.Join(dir, "command"), fifo}
			}
			cmd := exec.Command("bash", args...)
			cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"))
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			output, err := os.Create(filepath.Join(dir, "output"))
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			cmd.Stdout, cmd.Stderr = output, output
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			pid := cmd.Process.Pid
			defer syscall.Kill(-pid, syscall.SIGKILL)
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			deadline := time.Now().Add(5 * time.Second)
			for {
				ready, _ := os.ReadFile(filepath.Join(dir, "ready"))
				if strings.Count(string(ready), "ready") == 2 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("tails did not start")
				}
				time.Sleep(10 * time.Millisecond)
			}
			wantCode := 143
			if mode == "wrapper-exit" {
				f, err := os.OpenFile(fifo, os.O_WRONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				_, err = f.WriteString("finish\n")
				f.Close()
				if err != nil {
					t.Fatal(err)
				}
				wantCode = 7
			} else {
				sig := syscall.SIGTERM
				if mode == "wrapper-int" {
					sig = syscall.SIGINT
					wantCode = 130
				}
				if err := cmd.Process.Signal(sig); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case <-done:
				if cmd.ProcessState.ExitCode() != wantCode {
					t.Errorf("exit = %d, want %d", cmd.ProcessState.ExitCode(), wantCode)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("helper did not exit after shutdown")
			}
			if err := syscall.Kill(-pid, 0); err != syscall.ESRCH {
				data, _ := exec.Command("ps", "-eo", "pid,ppid,pgid,args").Output()
				var survivors []string
				for _, line := range strings.Split(string(data), "\n") {
					fields := strings.Fields(line)
					if len(fields) > 2 && fields[2] == fmt.Sprint(pid) {
						survivors = append(survivors, line)
					}
				}
				t.Fatalf("workers survived shutdown: %v\n%s", err, strings.Join(survivors, "\n"))
			}
		})
	}
}
