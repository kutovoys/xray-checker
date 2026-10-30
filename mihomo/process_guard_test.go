//go:build linux || windows

package mihomo

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestAbruptSupervisorExitKillsCore(t *testing.T) {
	t.Setenv("XRAY_CHECKER_MIHOMO_HELPER", "supervisor")
	r, address := runnerFixture(t)
	workDir := t.TempDir()
	cmd := exec.Command(r.binary, "-f", r.configFile, "-d", workDir)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-done
	})
	var info struct {
		PID     int
		WorkDir string
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, err := os.ReadFile(filepath.Join(workDir, "child.json"))
		if err == nil && json.Unmarshal(data, &info) == nil && info.PID > 0 {
			break
		}
		select {
		case <-done:
			t.Fatal("supervisor exited before its core was ready")
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("supervisor did not start its core")
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Cleanup(func() {
		if socksReady(address) {
			if process, err := os.FindProcess(info.PID); err == nil {
				_ = process.Kill()
				_ = process.Release()
			}
		}
		// SIGKILL cannot run the supervisor's ordinary private-directory cleanup.
		_ = os.RemoveAll(info.WorkDir)
	})
	if !socksReady(address) {
		t.Fatal("supervised core never became ready")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-done
	deadline = time.Now().Add(5 * time.Second)
	for socksReady(address) {
		if time.Now().After(deadline) {
			t.Fatal("core survived the abrupt exit of its supervisor")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
