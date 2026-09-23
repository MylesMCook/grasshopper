// grasshopper-go-backup creates a consistent, no-overwrite SQLite snapshot.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/MylesMCook/grasshopper/internal/gomemory"
)

func run() error {
	var source, destination string
	flag.StringVar(&source, "source", "", "existing Grasshopper database to back up")
	flag.StringVar(&destination, "dest", "", "new backup path; must not exist")
	flag.Parse()
	if source == "" || destination == "" {
		return errors.New("source and new destination are required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	copy, err := gomemory.OpenWritableCopy(ctx, source, destination)
	if err != nil {
		return err
	}
	if err := copy.Close(); err != nil {
		return err
	}
	fmt.Println("Consistent backup ready. Verify it before changing the source.")
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Go backup:", err)
		os.Exit(1)
	}
}
