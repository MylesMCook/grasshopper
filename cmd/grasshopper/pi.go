package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/MylesMCook/grasshopper/internal/client"
)

// syncPiWiring keeps Pi's extension current from an agent session. Only the
// client that is the managed client writes it, so an older bundled client
// never replaces a newer template.
func syncPiWiring(configPath string) {
	managed, _, ok := client.ManagedClient(client.ManagedDir(configPath))
	if !ok || managed != clientVersion {
		return
	}
	if dir, err := client.PiAgentDir(); err == nil {
		_, _ = client.SyncPi(configPath, dir)
	}
}

func piCommand(args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(os.Stdout, "Use pi install to connect Pi now, or pi remove to disconnect it. Agents connect Pi on their own when it is installed.")
		return nil
	}
	if len(args) == 0 || (args[0] != "install" && args[0] != "remove") {
		return errors.New("use pi install or pi remove")
	}
	flags := flag.NewFlagSet("pi "+args[0], flag.ContinueOnError)
	configArg := flags.String("config", "", "client configuration path")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected Pi arguments")
	}
	path, err := client.ConfigPath(*configArg)
	if err != nil {
		return err
	}
	if path, err = filepath.Abs(path); err != nil {
		return err
	}
	// A newer managed client carries the newer extension template; an older
	// client from an extracted archive must not overwrite it.
	runManagedClient(path)
	dir, err := client.PiAgentDir()
	if err != nil {
		return err
	}
	if args[0] == "remove" {
		if err := client.RemovePi(path, dir); err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, "Grasshopper removed from Pi, and agents will not add it back. Run grasshopper pi install to reconnect; server memories remain.")
		return nil
	}
	managedDir := client.ManagedDir(path)
	if _, _, ok := client.ManagedClient(managedDir); !ok {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if _, err := client.FollowServer(ctx, path, clientVersion); err != nil {
			return fmt.Errorf("Pi runs the self-updated client, which could not be installed: %w", err)
		}
	}
	if _, _, ok := client.ManagedClient(managedDir); !ok {
		return errors.New("Pi runs the self-updated client, and none is installed; run grasshopper update first")
	}
	if err := client.InstallPi(path, dir); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "Grasshopper connected to Pi at %s. New Pi sessions load memory at startup.\n", client.PiExtensionPath(dir))
	return nil
}

// piAgentReport describes Pi's connection for check. Pi without the
// extension is listed only when it is installed and not turned off.
func piAgentReport(configPath, agentDir, serverVersion string) (agentReport, bool) {
	extension := client.PiExtensionPath(agentDir)
	if _, err := os.Stat(extension); err != nil {
		if info, err := os.Stat(agentDir); err != nil || !info.IsDir() || client.PiOff(configPath) {
			return agentReport{}, false
		}
		return agentReport{Agent: "Pi", Route: "Pi extension", State: agentUnknown,
			Detail: "Not connected yet; agents connect Pi at their next session.", Update: "Or run grasshopper pi install."}, true
	}
	report := agentReport{Agent: "Pi", Route: "Pi extension " + extension}
	managed, _, ok := client.ManagedClient(client.ManagedDir(configPath))
	if !ok {
		report.State, report.Detail, report.Update = agentBroken, "No self-updated client for Pi to run.", "Run grasshopper update."
		return report, true
	}
	report.Version, report.State = managed, versionState(managed, serverVersion)
	if report.State == agentBehind {
		report.Update = "Run grasshopper update."
	}
	return withNewerDetail(report), true
}
