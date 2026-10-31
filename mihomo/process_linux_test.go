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
	startOnLockedThread := func() {
		// Intentionally do not unlock: Go terminates this worker OS thread when
		// the goroutine exits. The core must belong to its own durable thread.
		threadID := syscall.Gettid()
		started <- startResult{threadID: threadID, err: r.Start()}
	}
	go func() {
		runtime.LockOSThread()
		if syscall.Gettid() == os.Getpid() {
			// Go parks its main OS thread instead of terminating it. Keep that
			// thread occupied until another goroutine has locked a worker thread.
			defer runtime.UnlockOSThread()
			locked := make(chan struct{})
			go func() {
				runtime.LockOSThread()
				close(locked)
				startOnLockedThread()
			}()
			<-locked
			return
		}
		startOnLockedThread()
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
