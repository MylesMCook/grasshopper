package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/MylesMCook/grasshopper/internal/client"
)

func fakeManagedClient(t *testing.T, configPath, version string) {
	t.Helper()
	name := "grasshopper"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dir := client.ManagedDir(configPath)
	if err := os.MkdirAll(filepath.Join(dir, "versions", version), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "versions", version, name), []byte("program"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "current"), []byte(version+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPiReportFollowsTheExtensionAndManagedClient(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "grasshopper", "client.json")
	agentDir := filepath.Join(root, "pi", "agent")

	if _, ok := piAgentReport(config, agentDir, "2.12.0"); ok {
		t.Fatal("a machine without Pi lists no Pi agent")
	}
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if report, ok := piAgentReport(config, agentDir, "2.12.0"); !ok || report.State != agentUnknown || report.Update == "" {
		t.Fatalf("installed Pi without the extension says how to connect it, got %+v", report)
	}
	if _, err := client.SyncPi(config, agentDir); err != nil {
		t.Fatal(err)
	}
	if report, _ := piAgentReport(config, agentDir, "2.12.0"); report.State != agentBroken {
		t.Fatalf("the extension needs a managed client, got %+v", report)
	}
	fakeManagedClient(t, config, "2.12.0")
	if report, _ := piAgentReport(config, agentDir, "2.12.0"); report.State != agentCurrent || report.Version != "2.12.0" {
		t.Fatalf("Pi runs the managed client, got %+v", report)
	}
	if report, _ := piAgentReport(config, agentDir, "2.13.0"); report.State != agentBehind || report.Update == "" {
		t.Fatalf("a managed client behind the server needs an update, got %+v", report)
	}
	if err := client.RemovePi(config, agentDir); err != nil {
		t.Fatal(err)
	}
	if _, ok := piAgentReport(config, agentDir, "2.12.0"); ok {
		t.Fatal("Pi turned off by its owner is not listed")
	}
}

func TestSyncPiWiringWritesOnlyFromTheManagedClient(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "grasshopper", "client.json")
	agentDir := filepath.Join(root, "pi", "agent")
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_CODING_AGENT_DIR", agentDir)
	previous := clientVersion
	t.Cleanup(func() { clientVersion = previous })

	clientVersion = "2.11.0"
	fakeManagedClient(t, config, "2.12.0")
	syncPiWiring(config)
	if _, err := os.Stat(client.PiExtensionPath(agentDir)); !os.IsNotExist(err) {
		t.Fatalf("an older bundled client must not write its template, got %v", err)
	}
	clientVersion = "2.12.0"
	syncPiWiring(config)
	if _, err := os.Stat(client.PiExtensionPath(agentDir)); err != nil {
		t.Fatalf("the managed client writes the extension: %v", err)
	}
}
