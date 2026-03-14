package lima

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

func startSerialTail(instance string) func() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return func() {}
	}
	instanceDir := filepath.Join(homeDir, ".lima", instance)
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		offsets := map[string]int64{}
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				matches, err := filepath.Glob(filepath.Join(instanceDir, "serial*.log"))
				if err != nil {
					continue
				}
				for _, match := range matches {
					offsets[match] = tailSerialFile(match, offsets[match])
				}
			}
		}
	}()
	return func() {
		close(done)
		wg.Wait()
	}
}

func tailSerialFile(path string, offset int64) int64 {
	file, err := os.Open(path)
	if err != nil {
		return offset
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return offset
	}
	if stat.Size() < offset {
		offset = 0
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return offset
	}
	data, err := io.ReadAll(file)
	if err != nil || len(data) == 0 {
		return offset
	}
	fmt.Fprint(os.Stdout, string(data))
	return offset + int64(len(data))
}
