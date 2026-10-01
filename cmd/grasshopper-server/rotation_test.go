package main

import (
	"context"
	"crypto/sha256"
	"github.com/MylesMCook/grasshopper/internal/memory"
	"github.com/MylesMCook/grasshopper/internal/service"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRotateTokenOfflinePreservesDatabaseAndRollback(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	db, token, err := quickstartState(dir)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(db)
	old, _ := readToken(token)
	if err := rotateOwnerToken(dir, false); err == nil {
		t.Fatal("rotation accepted without stopped-server acknowledgement")
	}
	if err := rotateOwnerToken(dir, true); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		for _, path := range []string{token, token + ".previous"} {
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatal("rotation credential is not private")
			}
		}
	}
	next, _ := readToken(token)
	previous, _ := readToken(token + ".previous")
	after, _ := os.ReadFile(db)
	if old == next || previous != old || string(before) != string(after) {
		t.Fatal("rotation lost rollback or changed database")
	}
	if err := rotateOwnerToken(dir, true); err == nil {
		t.Fatal("existing rollback overwritten")
	}
	if err := os.Remove(token); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(token + ".previous"); err != nil {
		t.Fatal(err)
	}
	if err := rotateOwnerToken(dir, true); err != nil {
		t.Fatal("lost-token recovery:", err)
	}
	if next, err := readToken(token); err != nil || len(next) < 40 {
		t.Fatal("recovered token unusable")
	}
}

func TestRotationAppliesOnlyAfterHandlerReloadAndPreservesDevices(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	database, tokenPath, err := quickstartState(dir)
	if err != nil {
		t.Fatal(err)
	}
	store, err := memory.OpenWritableExisting(database, "synthetic", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	old, _ := readToken(tokenPath)
	paired := strings.Repeat("p", 40)
	if _, err := store.AddClientToken(context.Background(), "synthetic-device", sha256.Sum256([]byte(paired))); err != nil {
		t.Fatal(err)
	}
	before, err := service.NewHandler(service.Backend{Store: store, Visualizer: true}, old)
	if err != nil {
		t.Fatal(err)
	}
	call := func(handler http.Handler, path, token string, cookie *http.Cookie) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+path, nil)
		request.Header.Set("Origin", "http://127.0.0.1")
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		if cookie != nil {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	login := call(before, "/visualizer/api/session", old, nil)
	if login.Code != 204 {
		t.Fatal("initial sign-in:", login.Code)
	}
	cookie := login.Result().Cookies()[0]
	if err := rotateOwnerToken(dir, true); err != nil {
		t.Fatal(err)
	}
	if got := call(before, "/visualizer/api/session", old, nil).Code; got != 204 {
		t.Fatal("file rewrite changed running handler")
	}
	next, _ := readToken(tokenPath)
	after, err := service.NewHandler(service.Backend{Store: store, Visualizer: true}, next)
	if err != nil {
		t.Fatal(err)
	}
	if got := call(after, "/visualizer/api/session", old, nil).Code; got != 401 {
		t.Fatal("old owner token retained after reload:", got)
	}
	if got := call(after, "/visualizer/api/browse", "", cookie).Code; got != 401 {
		t.Fatal("old cookie retained after reload:", got)
	}
	if got := call(after, "/visualizer/api/session", next, nil).Code; got != 204 {
		t.Fatal("new owner rejected:", got)
	}
	if valid, err := store.ClientTokenValid(context.Background(), paired); err != nil || !valid {
		t.Fatal("paired device credential changed:", err)
	}
}
