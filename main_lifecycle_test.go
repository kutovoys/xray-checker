package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// The test executable acts as both checker and core, allowing the real main
// lifecycle (including its exit status) to run without a platform shell or an
// external Mihomo download.
func init() {
	if os.Getenv("XRAY_CHECKER_LIFECYCLE_HELPER") != "1" {
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--run-once" {
		main()
		os.Exit(0)
	}
	flags := flag.NewFlagSet("fake-core", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	validate := flags.Bool("t", false, "")
	filename := flags.String("f", "", "")
	flags.String("d", "", "")
	if flags.Parse(os.Args[1:]) != nil {
		os.Exit(2)
	}
	if *validate {
		os.Exit(0)
	}
	data, err := os.ReadFile(*filename)
	if err != nil {
		os.Exit(3)
	}
	var cfg struct {
		Listeners []struct {
			Listen string `yaml:"listen"`
			Port   int    `yaml:"port"`
		} `yaml:"listeners"`
	}
	if yaml.Unmarshal(data, &cfg) != nil || len(cfg.Listeners) == 0 {
		os.Exit(4)
	}
	for _, address := range cfg.Listeners {
		listener, err := net.Listen("tcp4", net.JoinHostPort(address.Listen, strconv.Itoa(address.Port)))
		if err != nil {
			os.Exit(5)
		}
		go func() {
			for {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				go func() {
					defer conn.Close()
					var greeting [3]byte
					if _, err := io.ReadFull(conn, greeting[:]); err == nil && greeting == [3]byte{5, 1, 0} {
						_, _ = conn.Write([]byte{5, 0})
					}
				}()
			}
		}()
	}
	if os.WriteFile(os.Getenv("XRAY_CHECKER_CORE_PID_FILE"), []byte(strconv.Itoa(os.Getpid())), 0600) != nil {
		os.Exit(6)
	}
	fmt.Fprintln(os.Stderr, "fixture-secret-password")
	for {
		time.Sleep(time.Hour)
	}
}

func TestRunOnceCoreDeathCancelsCheckAndExitsNonzero(t *testing.T) {
	probeStarted := make(chan struct{}, 1)
	probeCanceled := make(chan struct{}, 1)
	ipServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probeStarted <- struct{}{}
		<-r.Context().Done()
		probeCanceled <- struct{}{}
	}))
	t.Cleanup(ipServer.Close)
	pushed := make(chan struct{}, 1)
	pushServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pushed <- struct{}{}
	}))
	t.Cleanup(pushServer.Close)

	dir := t.TempDir()
	subscription := filepath.Join(dir, "subscription.yaml")
	if err := os.WriteFile(subscription, []byte("proxies:\n  - {name: fixture, type: socks5, server: 127.0.0.1, port: 9}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	reservation, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := reservation.Addr().(*net.TCPAddr).Port
	reservation.Close()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(dir, "core.pid")
	cmd := exec.Command(binary, "--run-once", "--proxy-engine=mihomo", "--mihomo-binary="+binary,
		"--subscription-url=file://"+subscription, "--xray-start-port="+strconv.Itoa(port),
		"--proxy-ip-check-url="+ipServer.URL, "--proxy-timeout=30", "--metrics-push-url="+pushServer.URL)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "XRAY_CHECKER_LIFECYCLE_HELPER=1", "XRAY_CHECKER_CORE_PID_FILE="+pidFile)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var waitErr error
	go func() { waitErr = cmd.Wait(); close(done) }()
	coreKilled := false
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-done
		// Also clean up after an assertion fails on platforms without a native
		// parent-death guard, where killing the checker alone leaves its child.
		if !coreKilled {
			if data, err := os.ReadFile(pidFile); err == nil {
				if pid, err := strconv.Atoi(string(data)); err == nil {
					if process, err := os.FindProcess(pid); err == nil {
						_ = process.Kill()
						_ = process.Release()
					}
				}
			}
		}
	})
	select {
	case <-probeStarted:
	case <-done:
		t.Fatalf("checker exited before starting a check: %v\n%s", waitErr, output.String())
	case <-time.After(10 * time.Second):
		t.Fatal("checker did not start a check")
	}
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatal(err)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	defer process.Release()
	if err := process.Kill(); err != nil {
		t.Fatal(err)
	}
	coreKilled = true
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("core death did not interrupt the 30-second check")
	}
	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) || exitErr.ExitCode() == 0 {
		t.Fatalf("checker exit = %v, want nonzero status\n%s", waitErr, output.String())
	}
	if !strings.Contains(output.String(), "Mihomo probe backend exited unexpectedly") || strings.Contains(output.String(), "fixture-secret-password") {
		t.Fatalf("missing generic failure or leaked core output:\n%s", output.String())
	}
	select {
	case <-probeCanceled:
	case <-time.After(time.Second):
		t.Fatal("in-flight probe was not canceled")
	}
	select {
	case <-pushed:
		t.Fatal("canceled probe cycle pushed metrics")
	default:
	}
}
