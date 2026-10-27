package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/MylesMCook/grasshopper/internal/client"
)

// handedOffEnv marks a client started by another client's hand-off, so a
// managed client never hands off again. Its value is the bundled client's
// version, which decides whether a managed client is still needed.
const handedOffEnv = "GRASSHOPPER_HANDED_OFF"

// runManagedClient hands bridge and hook off to a newer managed client. If
// the hand-off cannot start, this client keeps serving the agent.
func runManagedClient(configPath string) {
	if os.Getenv(handedOffEnv) != "" {
		return
	}
	if program, ok := client.HandOffTarget(configPath, clientVersion); ok {
		if err := handOff(program); err != nil {
			fmt.Fprintf(os.Stderr, "Grasshopper: managed client did not start (%v); using the bundled client\n", err)
		}
	}
}

// bundledVersion is the client that runs when no managed client does.
func bundledVersion() string {
	if version := os.Getenv(handedOffEnv); version != "" {
		return version
	}
	return clientVersion
}

// followServerInBackground starts the bridge's self-update. It never delays
// or fails the bridge; the new client is used from the next session.
func followServerInBackground(configPath string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		_, _ = client.FollowServer(ctx, configPath, bundledVersion())
	}()
}

// effectiveVersion is the newest client this machine runs: this one or the
// managed client it would hand off to.
func effectiveVersion(configPath string) string {
	if managed, _, ok := client.ManagedClient(client.ManagedDir(configPath)); ok {
		if order, ok := client.CompareVersions(managed, clientVersion); ok && order > 0 {
			return managed
		}
	}
	return clientVersion
}

func updateClient(args []string) error {
	flags := flag.NewFlagSet("update", flag.ContinueOnError)
	configArg := flags.String("config", "", "client configuration path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected update arguments")
	}
	path, err := client.ConfigPath(*configArg)
	if err != nil {
		return err
	}
	dir := client.ManagedDir(path)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if client.UpdatesDisabled(dir) {
		return fmt.Errorf("updates are off on this machine; remove %s to turn them on", dir+string(os.PathSeparator)+"updates-off")
	}
	installed, err := client.FollowServer(ctx, path, bundledVersion())
	if err != nil {
		return err
	}
	if installed == "" {
		fmt.Fprintf(os.Stdout, "Grasshopper client %s already matches the server.\n", effectiveVersion(path))
		return nil
	}
	fmt.Fprintf(os.Stdout, "Agents on this machine run Grasshopper client %s from their next session.\n", installed)
	return nil
}
