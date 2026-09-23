// grasshopper is the Go-only stateless client for Codex Desktop, Cursor, and
// Claude Code. It never stores memory locally.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/MylesMCook/grasshopper/internal/goclient"
)

func run() error {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Fprintln(os.Stdout, "grasshopper 2.0.0 (Go client)")
		return nil
	}
	if len(os.Args) < 2 {
		return errors.New("use bridge or hook")
	}
	switch os.Args[1] {
	case "bridge":
		flags := flag.NewFlagSet("bridge", flag.ContinueOnError)
		config := flags.String("config", "", "client configuration path")
		if err := flags.Parse(os.Args[2:]); err != nil {
			return err
		}
		if *config == "" {
			return errors.New("bridge needs --config")
		}
		return goclient.Bridge(context.Background(), *config, os.Stdin, os.Stdout)
	case "hook":
		flags := flag.NewFlagSet("hook", flag.ContinueOnError)
		config := flags.String("config", "", "client configuration path")
		harness := flags.String("harness", "", "codex, cursor, or claude")
		if err := flags.Parse(os.Args[2:]); err != nil {
			return err
		}
		if *config == "" || *harness == "" {
			return errors.New("hook needs --config and --harness")
		}
		input, err := goclient.ParseHookInput(os.Stdin)
		if err != nil {
			return err
		}
		output, err := goclient.Hook(*config, *harness, input)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Grasshopper hook unavailable; no persistence acknowledged")
			return nil
		}
		return json.NewEncoder(os.Stdout).Encode(output)
	default:
		return errors.New("use bridge or hook")
	}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Grasshopper client:", err)
		os.Exit(1)
	}
}
