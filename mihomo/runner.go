package mihomo

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultStartTimeout = 15 * time.Second
	defaultStopTimeout  = 5 * time.Second
)

type childProcess struct {
	cmd      *exec.Cmd
	done     chan struct{}
	err      error // read only after done closes
	guard    processGuard
	ready    bool // protected by Runner.mu
	reported bool // protected by Runner.mu
}

type Runner struct {
	binary       string
	configFile   string
	mu           sync.Mutex
	process      *childProcess
	failures     chan error
	workDir      string
	startTimeout time.Duration
	stopTimeout  time.Duration
}

func NewRunner(binary, configFile string) *Runner {
	return &Runner{
		binary:       binary,
		configFile:   configFile,
		failures:     make(chan error, 1),
		startTimeout: defaultStartTimeout,
		stopTimeout:  defaultStopTimeout,
	}
}

func (r *Runner) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.process != nil {
		select {
		case <-r.process.done:
			r.cleanupLocked()
		default:
			return fmt.Errorf("Mihomo is already running")
		}
	}
	binary, err := resolveBinary(r.binary)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(r.configFile)
	if err != nil {
		return fmt.Errorf("read Mihomo configuration: %w", err)
	}
	addresses, err := listenerAddresses(data)
	if err != nil {
		return err
	}
	// Do not mistake a pre-existing local SOCKS server for this child's readiness.
	if err := checkPortsAvailable(addresses); err != nil {
		return err
	}
	r.workDir, err = os.MkdirTemp("", "xray-checker-mihomo-")
	if err != nil {
		return fmt.Errorf("create Mihomo runtime directory: %w", err)
	}
	privateConfig := filepath.Join(r.workDir, "config.yaml")
	if err := os.WriteFile(privateConfig, data, 0o600); err != nil {
		r.cleanupLocked()
		return fmt.Errorf("write private Mihomo runtime configuration: %w", err)
	}
	cmd := exec.Command(binary, "-f", privateConfig, "-d", r.workDir)
	cmd.Dir = r.workDir
	cmd.Env = childEnvironment()
	guard, err := newProcessGuard(cmd)
	if err != nil {
		r.cleanupLocked()
		return fmt.Errorf("prepare Mihomo process supervision: %w", err)
	}
	process := &childProcess{cmd: cmd, done: make(chan struct{}), guard: guard}
	r.process = process
	started := make(chan error, 1)
	go process.run(started)
	if err := <-started; err != nil {
		<-process.done
		r.cleanupLocked()
		return err
	}
	if err := r.waitReadyLocked(process, addresses); err != nil {
		_ = r.stopLocked()
		return err
	}
	process.ready = true
	go r.watchProcess(process)
	return nil
}

// Failures reports an unexpected exit of an active, ready child. The channel
// survives configuration reloads; failed starts and intentional stops are not
// failures. Callers should stop the checker so its service manager can recover.
func (r *Runner) Failures() <-chan error { return r.failures }

func (r *Runner) watchProcess(process *childProcess) {
	<-process.done
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.process != process {
		// Stop or a subsequent Start already retired this child.
		return
	}
	r.reportExitLocked(process)
}

func (r *Runner) reportExitLocked(process *childProcess) {
	if !process.ready || process.reported {
		return
	}
	process.reported = true
	// Child output can contain proxy credentials. Keep the failure generic, and
	// never block cleanup if the caller is already shutting down or not watching.
	select {
	case r.failures <- fmt.Errorf("Mihomo probe backend exited unexpectedly"):
	default:
	}
}

func (process *childProcess) run(started chan<- error) {
	// On Linux, Pdeathsig is tied to the creating OS thread, not merely to the
	// checker process. Own that thread for the entire child lifetime: the caller
	// of Start may be a short-lived or explicitly locked Go goroutine.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(process.done)
	if err := process.cmd.Start(); err != nil {
		process.err = err
		started <- fmt.Errorf("could not start Mihomo executable")
		return
	}
	if err := process.guard.attach(process.cmd); err != nil {
		_ = process.cmd.Process.Kill()
		process.err = process.cmd.Wait()
		started <- fmt.Errorf("could not supervise Mihomo child process")
		return
	}
	started <- nil
	process.err = process.cmd.Wait()
}

func (r *Runner) waitReadyLocked(process *childProcess, addresses []string) error {
	deadline := time.NewTimer(r.startTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-process.done:
			return fmt.Errorf("Mihomo exited before its SOCKS listeners were ready")
		case <-deadline.C:
			return fmt.Errorf("Mihomo SOCKS listeners did not become ready before the startup timeout")
		case <-tick.C:
			ready := true
			for _, address := range addresses {
				if !socksReady(address) {
					ready = false
					break
				}
			}
			if ready {
				select {
				case <-process.done:
					return fmt.Errorf("Mihomo exited during startup")
				default:
					return nil
				}
			}
		}
	}
}

func (r *Runner) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopLocked()
}

func (r *Runner) stopLocked() error {
	process := r.process
	if process == nil {
		r.cleanupLocked()
		return nil
	}
	select {
	case <-process.done:
		// A final probe can finish before the watcher runs. Preserve that failure
		// even when its caller immediately stops the backend (e.g. --run-once).
		r.reportExitLocked(process)
	default:
		if err := terminateProcess(process.cmd); err != nil {
			_ = process.cmd.Process.Kill()
		}
		timer := time.NewTimer(r.stopTimeout)
		select {
		case <-process.done:
			timer.Stop()
		case <-timer.C:
			_ = process.cmd.Process.Kill()
			<-process.done
		}
	}
	r.cleanupLocked()
	return nil
}

func (r *Runner) cleanupLocked() {
	if r.process != nil && r.process.guard != nil {
		r.process.guard.close()
	}
	r.process = nil
	if r.workDir != "" {
		_ = os.RemoveAll(r.workDir)
		r.workDir = ""
	}
}

func listenerAddresses(data []byte) ([]string, error) {
	var config generatedConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("invalid generated Mihomo configuration")
	}
	if len(config.Listeners) == 0 {
		return nil, fmt.Errorf("Mihomo configuration has no checker listeners")
	}
	addresses := make([]string, 0, len(config.Listeners))
	seen := map[int]bool{}
	for _, listener := range config.Listeners {
		if listener.Type != "socks" || listener.Listen != "127.0.0.1" || listener.Port < 1 || listener.Port > 65535 || seen[listener.Port] {
			return nil, fmt.Errorf("invalid checker SOCKS listener in Mihomo configuration")
		}
		seen[listener.Port] = true
		addresses = append(addresses, net.JoinHostPort(listener.Listen, strconv.Itoa(listener.Port)))
	}
	return addresses, nil
}

func checkPortsAvailable(addresses []string) error {
	var reservations []net.Listener
	defer func() {
		for _, listener := range reservations {
			listener.Close()
		}
	}()
	for _, address := range addresses {
		listener, err := net.Listen("tcp4", address)
		if err != nil {
			return fmt.Errorf("Mihomo checker listener %s is unavailable", address)
		}
		reservations = append(reservations, listener)
	}
	return nil
}

func socksReady(address string) bool {
	conn, err := net.DialTimeout("tcp4", address, 100*time.Millisecond)
	if err != nil {
		return false
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(100 * time.Millisecond))
	if _, err := conn.Write([]byte{5, 1, 0}); err != nil {
		return false
	}
	var response [2]byte
	_, err = io.ReadFull(conn, response[:])
	return err == nil && response == [2]byte{5, 0}
}
