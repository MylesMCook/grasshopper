package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Exercise the actual bundle CLI and an extracted native server, including
// external model weights. Unit tests alone cannot prove portable packaging.
func TestNativeServerArchiveQuickstart(t *testing.T) {
	modelRoot := os.Getenv("GRASSHOPPER_EMBED_TEST_ROOT")
	if modelRoot == "" {
		t.Skip("set GRASSHOPPER_EMBED_TEST_ROOT for native archive verification")
	}
	library := os.Getenv("GRASSHOPPER_ONNX_RUNTIME_LIBRARY")
	if library == "" {
		t.Fatal("GRASSHOPPER_ONNX_RUNTIME_LIBRARY is required")
	}
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "built")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	build := exec.Command("go", "build", "-o", bin+string(filepath.Separator), "./cmd/grasshopper", "./cmd/grasshopper-go-server", "./cmd/grasshopper-go-backup", "./cmd/grasshopper-go-migrate")
	build.Dir = repository
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("native build: %v\n%s", err, output)
	}
	// The pinned runtime archive has LICENSE and ThirdPartyNotices.txt beside lib.
	licenseRoot := filepath.Dir(filepath.Dir(library))
	archive := filepath.Join(dir, "server.zip")
	command := exec.Command("go", "run", "./cmd/grasshopper-go-bundle",
		"-output", archive, "-client", filepath.Join(bin, "grasshopper"+suffix),
		"-server", filepath.Join(bin, "grasshopper-go-server"+suffix),
		"-backup", filepath.Join(bin, "grasshopper-go-backup"+suffix),
		"-migrate", filepath.Join(bin, "grasshopper-go-migrate"+suffix),
		"-onnx-library", library, "-model", filepath.Join(modelRoot, "model.onnx"),
		"-tokenizer", filepath.Join(modelRoot, "tokenizer.json"),
		"-onnx-license", filepath.Join(licenseRoot, "LICENSE"),
		"-onnx-notices", filepath.Join(licenseRoot, "ThirdPartyNotices.txt"))
	command.Dir = repository
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("bundle CLI: %v\n%s", err, output)
	}
	z, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	extracted := filepath.Join(dir, "extracted")
	digests := map[string]string{}
	var sums string
	for _, entry := range z.File {
		if strings.HasSuffix(entry.Name, ".db") || strings.Contains(entry.Name, "access-token") {
			t.Fatalf("private state packaged: %s", entry.Name)
		}
		if !filepath.IsLocal(entry.Name) {
			t.Fatalf("unsafe archive path: %s", entry.Name)
		}
		path := filepath.Join(extracted, filepath.FromSlash(entry.Name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		reader, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, entry.Mode().Perm())
		if err != nil {
			reader.Close()
			t.Fatal(err)
		}
		hash := sha256.New()
		_, copyErr := io.Copy(io.MultiWriter(file, hash), reader)
		reader.Close()
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil {
			t.Fatalf("extract: %v %v", copyErr, closeErr)
		}
		if entry.Name == "SHA256SUMS" {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			sums = string(data)
		} else {
			digests[entry.Name] = fmt.Sprintf("%x", hash.Sum(nil))
		}
	}
	for name, digest := range digests {
		if !strings.Contains(sums, digest+"  "+name+"\n") {
			t.Fatalf("missing or wrong archive digest: %s", name)
		}
	}
	for _, name := range []string{"models/granite-embedding-small-english-r2/model.onnx_data", "licenses/Granite-LICENSE", "licenses/Granite-NOTICE.txt", "docs/embedding-model.md"} {
		if digests[name] == "" {
			t.Fatalf("missing model dependency or notice: %s", name)
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	state := filepath.Join(dir, "state")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := exec.CommandContext(ctx, filepath.Join(extracted, "bin", "grasshopper-server"+suffix), "--quickstart", "--data-dir", state, "--listen", address)
	var logs bytes.Buffer
	server.Stdout, server.Stderr = &logs, &logs
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = server.Wait() }()
	client := &http.Client{Timeout: time.Second}
	endpoint := "http://" + address + "/healthz"
	deadline := time.Now().Add(20 * time.Second)
	for {
		response, err := client.Get(endpoint)
		if err == nil {
			response.Body.Close()
			if response.StatusCode != http.StatusUnauthorized {
				t.Fatalf("anonymous health: %d", response.StatusCode)
			}
			break
		}
		if time.Now().After(deadline) {
			cancel()
			_ = server.Wait()
			t.Fatalf("packaged server not ready: %v\n%s", err, logs.String())
		}
		time.Sleep(100 * time.Millisecond)
	}
	token, err := os.ReadFile(filepath.Join(state, "access-token"))
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("authenticated packaged health: %d", response.StatusCode)
	}
	t.Logf("native %s/%s archive: %d hashes verified; extracted model/runtime started", runtime.GOOS, runtime.GOARCH, len(digests))
}
