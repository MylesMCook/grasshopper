// grasshopper-go-server serves an existing database or creates a new one on request.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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

func quickstartFiles(executable string) (library, model, tokenizer string, err error) {
	root := filepath.Dir(filepath.Dir(executable))
	libraryName := map[string]string{
		"darwin":  "libonnxruntime.dylib",
		"linux":   "libonnxruntime.so",
		"windows": "onnxruntime.dll",
	}[runtime.GOOS]
	if libraryName == "" {
		return "", "", "", errors.New("quickstart is unavailable on this operating system")
	}
	library = filepath.Join(root, "runtime", libraryName)
	model = filepath.Join(root, "models", "bge-small-en-v1.5", "model.onnx")
	tokenizer = filepath.Join(root, "models", "bge-small-en-v1.5", "tokenizer.json")
	for _, path := range []string{library, model, tokenizer} {
		info, statErr := os.Stat(path)
		if statErr != nil || !info.Mode().IsRegular() {
			return "", "", "", fmt.Errorf("bundle file missing: %s", path)
		}
	}
	return library, model, tokenizer, nil
}

func quickstartState(dir string) (database, tokenFile string, err error) {
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		if err = os.MkdirAll(dir, 0700); err != nil {
			return "", "", err
		}
	} else if err != nil {
		return "", "", err
	} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", "", errors.New("quickstart data path must be a directory, not a link")
	} else if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return "", "", errors.New("quickstart data directory must be private (mode 0700)")
	}
	database = filepath.Join(dir, "memory.db")
	tokenFile = filepath.Join(dir, "access-token")
	dbInfo, dbErr := os.Lstat(database)
	tokenInfo, tokenErr := os.Lstat(tokenFile)
	dbExists, tokenExists := dbErr == nil, tokenErr == nil
	if dbErr != nil && !errors.Is(dbErr, os.ErrNotExist) {
		return "", "", dbErr
	}
	if tokenErr != nil && !errors.Is(tokenErr, os.ErrNotExist) {
		return "", "", tokenErr
	}
	if dbExists != tokenExists {
		return "", "", errors.New("incomplete quickstart state: back up and inspect the data directory before retrying")
	}
	if dbExists {
		if !dbInfo.Mode().IsRegular() || !tokenInfo.Mode().IsRegular() {
			return "", "", errors.New("quickstart state must use regular files")
		}
		if _, err := readToken(tokenFile); err != nil {
			return "", "", err
		}
		return database, tokenFile, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", "", err
	}
	if len(entries) != 0 {
		return "", "", errors.New("quickstart data directory is not empty: choose a new data-dir or open existing state with manual flags")
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", "", err
	}
	file, err := os.OpenFile(tokenFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", "", err
	}
	_, writeErr := fmt.Fprintln(file, base64.RawURLEncoding.EncodeToString(secret))
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(tokenFile)
		return "", "", errors.Join(writeErr, closeErr)
	}
	if err := gomemory.CreateEmpty(database); err != nil {
		_ = os.Remove(tokenFile)
		return "", "", err
	}
	return database, tokenFile, nil
}

func run() error {
	var database, library, model, tokenizer, tokenFile, listen, allowedProxyHost, visualizerStyleHashes, dataDir string
	var createDB, visualizer, quickstart bool
	flag.StringVar(&database, "db", "", "Grasshopper database path")
	flag.BoolVar(&quickstart, "quickstart", false, "start a private server from an extracted bundle")
	flag.StringVar(&dataDir, "data-dir", "", "quickstart state directory (default: user config directory/Grasshopper)")
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
	if quickstart {
		if database != "" || library != "" || model != "" || tokenizer != "" || tokenFile != "" || createDB || allowedProxyHost != "" {
			return errors.New("quickstart cannot be combined with manual database, model, token, proxy, or create-db flags")
		}
		if dataDir == "" {
			base, err := os.UserConfigDir()
			if err != nil {
				return err
			}
			dataDir = filepath.Join(base, "Grasshopper")
		}
		if !filepath.IsAbs(dataDir) {
			return errors.New("quickstart data-dir must be absolute")
		}
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		library, model, tokenizer, err = quickstartFiles(executable)
		if err != nil {
			return err
		}
		visualizer = true
	} else if dataDir != "" {
		return errors.New("data-dir requires quickstart")
	}
	if !quickstart && (database == "" || library == "" || model == "" || tokenizer == "" || tokenFile == "") {
		return errors.New("db, ONNX library, model, tokenizer, and token file are required")
	}
	if err := validateListen(listen); err != nil {
		return err
	}
	var token string
	var err error
	if !quickstart {
		token, err = readToken(tokenFile)
		if err != nil {
			return err
		}
	}
	embedder, err := goembed.NewBGE(library, model, tokenizer)
	if err != nil {
		return err
	}
	defer embedder.Close()
	if quickstart {
		database, tokenFile, err = quickstartState(dataDir)
		if err != nil {
			return err
		}
		token, err = readToken(tokenFile)
		if err != nil {
			return err
		}
	}
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
	if quickstart {
		fmt.Fprintf(os.Stderr, "Memory view: http://%s/visualizer/\nMemory database: %s\nAccess token file: %s\n", listener.Addr(), database, tokenFile)
	}
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
