// grasshopper-go-server serves an existing database or creates a new one on request.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/MylesMCook/grasshopper/internal/goembed"
	"github.com/MylesMCook/grasshopper/internal/gomcp"
	"github.com/MylesMCook/grasshopper/internal/gomemory"
)

func readToken(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("token file is not a regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return "", errors.New("token file must be private (mode 0600)")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) > 4096 {
		return "", errors.New("token file is too large")
	}
	return strings.TrimRight(string(data), "\r\n"), nil
}

func validateListen(address string) error {
	host, _, err := net.SplitHostPort(address)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() {
		return errors.New("listen must use an explicit loopback IP and port")
	}
	return nil
}

func run() error {
	var database, library, model, tokenizer, tokenFile, listen, allowedProxyHost, visualizerStyleHashes string
	var createDB, visualizer bool
	flag.StringVar(&database, "db", "", "Grasshopper database path")
	flag.BoolVar(&createDB, "create-db", false, "create an empty memory database if missing")
	flag.BoolVar(&visualizer, "visualizer", false, "serve optional read-only live memory view at /visualizer/")
	flag.StringVar(&visualizerStyleHashes, "visualizer-style-hashes", "", "comma-separated SHA-256 hashes for optional browser annotation styles")
	flag.StringVar(&library, "onnx-library", "", "local ONNX Runtime shared library")
	flag.StringVar(&model, "model", "", "pinned BGE ONNX model")
	flag.StringVar(&tokenizer, "tokenizer", "", "pinned BGE tokenizer.json")
	flag.StringVar(&tokenFile, "token-file", "", "private bearer token file")
	flag.StringVar(&listen, "listen", "127.0.0.1:8106", "loopback listen address")
	flag.StringVar(&allowedProxyHost, "allowed-proxy-host", "", "exact HTTPS proxy Host, including port")
	flag.Parse()
	if database == "" || library == "" || model == "" || tokenizer == "" || tokenFile == "" {
		return errors.New("db, ONNX library, model, tokenizer, and token file are required")
	}
	if err := validateListen(listen); err != nil {
		return err
	}
	token, err := readToken(tokenFile)
	if err != nil {
		return err
	}
	embedder, err := goembed.NewBGE(library, model, tokenizer)
	if err != nil {
		return err
	}
	defer embedder.Close()
	if createDB {
		if _, err := os.Stat(database); errors.Is(err, os.ErrNotExist) {
			if err := gomemory.CreateEmpty(database); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	store, err := gomemory.OpenWritableExisting(database, goembed.ModelName, goembed.Dimensions)
	if err != nil {
		return err
	}
	defer store.Close()
	var styleHashes []string
	if visualizerStyleHashes != "" {
		styleHashes = strings.Split(visualizerStyleHashes, ",")
	}
	handler, err := gomcp.NewHandler(gomcp.Backend{Store: store, Embedder: embedder, Model: goembed.ModelName, Visualizer: visualizer, VisualizerStyleHashes: styleHashes, AllowedProxyHost: allowedProxyHost}, token)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	defer listener.Close()
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() { errCh <- server.Serve(listener) }()
	fmt.Fprintf(os.Stderr, "Grasshopper listening on http://%s/mcp\n", listener.Addr())
	select {
	case <-ctx.Done():
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Grasshopper:", err)
		os.Exit(1)
	}
}
