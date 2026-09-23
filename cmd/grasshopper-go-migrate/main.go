// grasshopper-go-migrate makes a new re-embedded shadow database. It never
// changes the source. Stop the source writer before running it for a cutover.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/MylesMCook/grasshopper/internal/goembed"
	"github.com/MylesMCook/grasshopper/internal/gomemory"
)

func run() error {
	var source, copyPath, library, model, tokenizer string
	flag.StringVar(&source, "source", "", "existing Grasshopper database to read without modification")
	flag.StringVar(&copyPath, "copy", "", "new shadow database path; must not exist")
	flag.StringVar(&library, "onnx-library", "", "local ONNX Runtime shared library")
	flag.StringVar(&model, "model", "", "pinned BGE ONNX model")
	flag.StringVar(&tokenizer, "tokenizer", "", "pinned BGE tokenizer.json")
	flag.Parse()
	if source == "" || copyPath == "" || library == "" || model == "" || tokenizer == "" {
		return errors.New("source, new copy, ONNX library, model, and tokenizer are required")
	}
	embedder, err := goembed.NewBGE(library, model, tokenizer)
	if err != nil {
		return err
	}
	defer embedder.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	shadow, err := gomemory.ReembedCopy(ctx, source, copyPath, goembed.ModelName, embedder.EmbedDocument)
	if err != nil {
		return err
	}
	if err := shadow.Close(); err != nil {
		return err
	}
	fmt.Println("Shadow copy ready; source database unchanged. Verify recall before any cutover.")
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Go migration:", err)
		os.Exit(1)
	}
}
