package mihomo

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xray-checker/models"
)

// The test executable doubles as a portable fake core so supervision can be
// exercised on Linux and Windows without shell scripts or a downloaded binary.
func init() {
	mode := os.Getenv("XRAY_CHECKER_MIHOMO_HELPER")
	if mode == "" {
		return
	}
	flags := flag.NewFlagSet("fake-mihomo", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	validate := flags.Bool("t", false, "")
	filename := flags.String("f", "", "")
	workDir := flags.String("d", "", "")
	if flags.Parse(os.Args[1:]) != nil {
		os.Exit(2)
	}
	data, err := os.ReadFile(*filename)
	if err != nil {
		os.Exit(3)
	}
	addresses, err := listenerAddresses(data)
	if err != nil {
		os.Exit(4)
	}
	if mode == "reject" {
		fmt.Fprintln(os.Stderr, "invalid fixture-secret-password")
		os.Exit(7)
	}
	if *validate {
		os.Exit(0)
	}
	if mode == "exit" {
		fmt.Fprintln(os.Stderr, "startup fixture-secret-password")
		os.Exit(8)
	}
	if mode == "hang" {
		for {
			time.Sleep(time.Hour)
		}
	}
	if mode == "supervisor" {
		os.Setenv("XRAY_CHECKER_MIHOMO_HELPER", "serve")
		binary, err := os.Executable()
		if err != nil {
			os.Exit(10)
		}
		runner := NewRunner(binary, *filename)
		if err := runner.Start(); err != nil {
			os.Exit(11)
		}
		info, _ := json.Marshal(struct {
			PID     int
			WorkDir string
		}{runner.process.cmd.Process.Pid, runner.workDir})
		if err := os.WriteFile(filepath.Join(*workDir, "child.json"), info, 0o600); err != nil {
			os.Exit(12)
		}
		for {
			time.Sleep(time.Hour)
		}
	}
	for _, address := range addresses {
		listener, err := net.Listen("tcp4", address)
		if err != nil {
			os.Exit(9)
		}
		go func(listener net.Listener) {
			for {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				go func() {
					defer conn.Close()
					var greeting [3]byte
					if _, err := io.ReadFull(conn, greeting[:]); err == nil && greeting == [3]byte{5, 1, 0} {
						conn.Write([]byte{5, 0})
					}
				}()
			}
		}(listener)
	}
	for {
		time.Sleep(time.Hour)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func runnerFixture(t *testing.T) (*Runner, string) {
	t.Helper()
	port := freePort(t)
	data, err := NewConfigGenerator("").GenerateConfig([]*models.ProxyConfig{nativeProxy()}, port, "none")
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	r := NewRunner(binary, filename)
	r.startTimeout, r.stopTimeout = 2*time.Second, time.Second
	t.Cleanup(func() { _ = r.Stop() })
	return r, fmt.Sprintf("127.0.0.1:%d", port)
}

func TestRunnerStartStopAndRestart(t *testing.T) {
	t.Setenv("XRAY_CHECKER_MIHOMO_HELPER", "serve")
	r, address := runnerFixture(t)
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	workDir := r.workDir
	if !socksReady(address) {
		t.Fatal("SOCKS listener was not ready")
	}
	if err := r.Start(); err == nil {
		t.Fatal("double Start must not spawn another child")
	}
	if err := r.Stop(); err != nil {
		t.Fatal(err)
	}
	if socksReady(address) {
		t.Fatal("child listener survived Stop")
	}
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Fatalf("private runtime directory survived Stop: %v", err)
	}
	if err := r.Stop(); err != nil {
		t.Fatal("Stop must be idempotent")
	}
	if err := r.Start(); err != nil {
		t.Fatalf("restart failed: %v", err)
	}
}

func TestRunnerRejectsOccupiedPort(t *testing.T) {
	t.Setenv("XRAY_CHECKER_MIHOMO_HELPER", "serve")
	r, address := runnerFixture(t)
	listener, err := net.Listen("tcp4", address)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := r.Start(); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("occupied listener accepted: %v", err)
	}
	if r.process != nil || r.workDir != "" {
		t.Fatal("failed preflight created a child or runtime directory")
	}
}

func TestRunnerFailedStartupReapsChild(t *testing.T) {
	for _, mode := range []string{"exit", "hang"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("XRAY_CHECKER_MIHOMO_HELPER", mode)
			r, address := runnerFixture(t)
			r.startTimeout = 150 * time.Millisecond
			if err := r.Start(); err == nil || strings.Contains(err.Error(), "fixture-secret-password") {
				t.Fatalf("invalid startup error: %v", err)
			}
			if r.process != nil || r.workDir != "" || socksReady(address) {
				t.Fatal("failed startup leaked child resources")
			}
		})
	}
}

func TestRunnerCanRestartAfterUnexpectedChildExit(t *testing.T) {
	t.Setenv("XRAY_CHECKER_MIHOMO_HELPER", "serve")
	r, _ := runnerFixture(t)
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	process := r.process
	if err := process.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-process.done
	if err := r.Start(); err != nil {
		t.Fatalf("restart after child exit failed: %v", err)
	}
}
