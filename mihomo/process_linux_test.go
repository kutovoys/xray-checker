//go:build linux

package mihomo

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestCallerThreadExitDoesNotKillCore(t *testing.T) {
	t.Setenv("XRAY_CHECKER_MIHOMO_HELPER", "serve")
	r, address := runnerFixture(t)
	type startResult struct {
		threadID int
		err      error
	}
	started := make(chan startResult, 1)
	go func() {
		// Intentionally do not unlock: Go terminates this OS thread when the
		// goroutine exits. The core must belong to its own durable thread.
		runtime.LockOSThread()
		threadID := syscall.Gettid()
		started <- startResult{threadID: threadID, err: r.Start()}
	}()
	result := <-started
	if result.err != nil {
		t.Fatal(result.err)
	}
	threadPath := filepath.Join("/proc/self/task", strconv.Itoa(result.threadID))
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, err := os.Stat(threadPath)
		if os.IsNotExist(err) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("test caller OS thread did not exit")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !socksReady(address) {
		t.Fatal("core was killed by its caller's OS thread exit")
	}
	select {
	case <-r.process.done:
		t.Fatal("core exited after its caller's OS thread terminated")
	default:
	}
}
