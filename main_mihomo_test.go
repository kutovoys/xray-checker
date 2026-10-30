package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"xray-checker/checker"
	"xray-checker/config"
	"xray-checker/models"
)

type lifecycleGenerator struct {
	generate func([]*models.ProxyConfig, string) ([]*models.ProxyConfig, error)
}

func (g lifecycleGenerator) GenerateValidatedConfig(proxies []*models.ProxyConfig, _ int, filename, _ string) ([]*models.ProxyConfig, error) {
	return g.generate(proxies, filename)
}

type lifecycleRunner struct {
	start func() error
	stop  func() error
}

func (r lifecycleRunner) Start() error { return r.start() }
func (r lifecycleRunner) Stop() error  { return r.stop() }

func lifecycleProxy(name string) *models.ProxyConfig {
	p := &models.ProxyConfig{
		Name: name, Protocol: "vless", Server: name + ".example.invalid", Port: 443,
		UUID: "11111111-1111-4111-8111-111111111111",
	}
	p.StableID = p.GenerateStableID()
	return p
}

func lifecycleFixture(t *testing.T) (string, []*models.ProxyConfig, *checker.ProxyChecker) {
	t.Helper()
	original := config.CLIConfig
	t.Cleanup(func() { config.CLIConfig = original })
	config.CLIConfig.Xray.StartPort = 10000
	filename := filepath.Join(t.TempDir(), "mihomo_config.yaml")
	if err := os.WriteFile(filename, []byte("previous configuration"), 0600); err != nil {
		t.Fatal(err)
	}
	old := []*models.ProxyConfig{lifecycleProxy("previous")}
	pc := checker.NewProxyChecker(old, 10000, "", 30, "", "", 30, 1, "status", 0)
	return filename, old, pc
}

func assertLifecycleFile(t *testing.T, filename, expected string) {
	t.Helper()
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != expected {
		t.Fatalf("configuration contents = %q, want %q", data, expected)
	}
}

func assertLifecycleSnapshot(t *testing.T, current []*models.ProxyConfig, pc *checker.ProxyChecker, expected []*models.ProxyConfig) {
	t.Helper()
	if !reflect.DeepEqual(current, expected) {
		t.Fatal("stored configuration snapshot changed unexpectedly")
	}
	if !reflect.DeepEqual(pc.GetProxies(), expected) {
		t.Fatal("public checker snapshot changed unexpectedly")
	}
}

func assertNoStagedConfiguration(t *testing.T, filename string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(filename), ".checker-config-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Fatalf("staged configurations were not removed: %v", paths)
	}
}

func TestReplaceConfigurationValidationFailureKeepsRunningState(t *testing.T) {
	filename, previous, pc := lifecycleFixture(t)
	current := previous
	wanted := []*models.ProxyConfig{lifecycleProxy("candidate")}
	validationErr := errors.New("candidate rejected")
	generator := lifecycleGenerator{generate: func(_ []*models.ProxyConfig, staged string) ([]*models.ProxyConfig, error) {
		if staged == filename {
			t.Fatal("validation overwrote the live configuration")
		}
		info, err := os.Stat(staged)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
			t.Fatalf("staged secret file permissions = %o, want 600", info.Mode().Perm())
		}
		if err := os.WriteFile(staged, []byte("invalid candidate"), 0600); err != nil {
			t.Fatal(err)
		}
		return nil, validationErr
	}}
	runner := lifecycleRunner{
		start: func() error { t.Error("backend started after failed validation"); return nil },
		stop:  func() error { t.Error("running backend stopped after failed validation"); return nil },
	}
	err := replaceConfiguration(wanted, &current, runner, pc, filename, generator)
	if !errors.Is(err, validationErr) {
		t.Fatalf("error = %v, want validation error", err)
	}
	assertLifecycleFile(t, filename, "previous configuration")
	assertLifecycleSnapshot(t, current, pc, previous)
	assertNoStagedConfiguration(t, filename)
}

func TestReplaceConfigurationStartFailureRestoresPreviousState(t *testing.T) {
	filename, previous, pc := lifecycleFixture(t)
	current := previous
	wanted := []*models.ProxyConfig{lifecycleProxy("candidate")}
	startErr := errors.New("candidate did not become ready")
	events := []string{}
	generator := lifecycleGenerator{generate: func(proxies []*models.ProxyConfig, staged string) ([]*models.ProxyConfig, error) {
		events = append(events, "validate")
		return proxies, os.WriteFile(staged, []byte("candidate configuration"), 0600)
	}}
	starts := 0
	runner := lifecycleRunner{
		stop: func() error { events = append(events, "stop"); return nil },
		start: func() error {
			starts++
			assertLifecycleSnapshot(t, current, pc, previous)
			if starts == 1 {
				events = append(events, "start candidate")
				assertLifecycleFile(t, filename, "candidate configuration")
				return startErr
			}
			events = append(events, "start previous")
			assertLifecycleFile(t, filename, "previous configuration")
			return nil
		},
	}
	err := replaceConfiguration(wanted, &current, runner, pc, filename, generator)
	if !errors.Is(err, startErr) {
		t.Fatalf("error = %v, want candidate startup error", err)
	}
	if want := []string{"validate", "stop", "start candidate", "stop", "start previous"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("lifecycle = %v, want %v", events, want)
	}
	assertLifecycleFile(t, filename, "previous configuration")
	assertLifecycleSnapshot(t, current, pc, previous)
	assertNoStagedConfiguration(t, filename)
}

func TestReplaceConfigurationPublishesValidatedSnapshotAfterReadiness(t *testing.T) {
	filename, previous, pc := lifecycleFixture(t)
	current := previous
	wanted := []*models.ProxyConfig{lifecycleProxy("candidate"), lifecycleProxy("rejected")}
	validated := wanted[:1]
	events := []string{}
	generator := lifecycleGenerator{generate: func(proxies []*models.ProxyConfig, staged string) ([]*models.ProxyConfig, error) {
		if !reflect.DeepEqual(proxies, wanted) {
			t.Fatal("generator did not receive the complete candidate set")
		}
		events = append(events, "validate")
		return validated, os.WriteFile(staged, []byte("validated configuration"), 0600)
	}}
	runner := lifecycleRunner{
		stop: func() error { events = append(events, "stop"); return nil },
		start: func() error {
			events = append(events, "start")
			assertLifecycleFile(t, filename, "validated configuration")
			assertLifecycleSnapshot(t, current, pc, previous)
			return nil
		},
	}
	if err := replaceConfiguration(wanted, &current, runner, pc, filename, generator); err != nil {
		t.Fatal(err)
	}
	if want := []string{"validate", "stop", "start"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("lifecycle = %v, want %v", events, want)
	}
	assertLifecycleSnapshot(t, current, pc, validated)
	assertNoStagedConfiguration(t, filename)
	info, err := os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("installed secret file permissions = %o, want 600", info.Mode().Perm())
	}
}
