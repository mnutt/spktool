package lima

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// startWatch supplements serial output with Lima's lifecycle and forwarding
// events. Diagnostics are best-effort and must not prevent a VM from starting.
func startWatch(ctx context.Context, instance string, exists bool, output io.Writer) func() {
	probeCtx, cancelProbe := context.WithTimeout(ctx, 2*time.Second)
	err := exec.CommandContext(probeCtx, "limactl", "watch", "--help").Run()
	cancelProbe()
	if err != nil {
		// Lima 2.0 has no watch command. Probe capabilities rather than parsing
		// version strings, which can also be development commit hashes.
		return func() {}
	}

	watchCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		args := []string{"watch", "--tty=false"}
		if !exists {
			// A named watch cannot attach before the instance exists. Wait for
			// its event log, then replay history to include early boot events.
			dir, err := limaInstanceDir(instance)
			if err != nil {
				return
			}
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				if _, err := os.Stat(filepath.Join(dir, "ha.stdout.log")); err == nil {
					break
				}
				select {
				case <-watchCtx.Done():
					return
				case <-ticker.C:
				}
			}
			args = append(args, "--history")
		}
		args = append(args, instance)
		cmd := exec.CommandContext(watchCtx, "limactl", args...)
		cmd.Stdout = output
		cmd.Stderr = output
		cmd.WaitDelay = time.Second
		if err := cmd.Run(); err != nil && watchCtx.Err() == nil {
			fmt.Fprintf(output, "Lima event watch stopped: %v (serial diagnostics remain active)\n", err)
		}
	}()
	var once sync.Once
	return func() {
		once.Do(cancel)
		<-done
	}
}

func limaInstanceDir(instance string) (string, error) {
	if home := os.Getenv("LIMA_HOME"); home != "" {
		return filepath.Join(home, instance), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".lima", instance), nil
}
