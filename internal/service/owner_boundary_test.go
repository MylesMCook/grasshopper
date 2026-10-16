package service

import (
	"context"
	"crypto/sha256"
	"net/http"
	"strings"
	"testing"
)

func TestPairedTokenCannotListOwnerContext(t *testing.T) {
	server, store := testServer(t, true)
	paired := "synthetic-paired-device-token-0123456789"
	if _, err := store.AddClientToken(context.Background(), "paired", sha256.Sum256([]byte(paired))); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		token  string
		status int
	}{{paired, 401}, {testToken, 200}} {
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/visualizer/api/context", strings.NewReader(`{"all_projects":true,"view":"archived"}`))
		req.Header.Set("Authorization", "Bearer "+tc.token)
		req.Header.Set("Origin", server.URL)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != tc.status {
			t.Fatalf("context with a %s token: %d, want %d", map[bool]string{true: "paired", false: "master"}[tc.token == paired], resp.StatusCode, tc.status)
		}
	}
}

func TestPairingRejectsAForeignHost(t *testing.T) {
	server, _ := testServer(t, true)
	for _, path := range []string{"/pair/start", "/pair/poll"} {
		req, _ := http.NewRequest(http.MethodPost, server.URL+path, strings.NewReader(`{}`))
		req.Host = "attacker.example:8106"
		req.Header.Set("Origin", "http://attacker.example:8106")
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("%s from a foreign Host: %d, want 403", path, resp.StatusCode)
		}
	}
}

func TestClearingATitleInTheMemoryViewDerivesItFromTheText(t *testing.T) {
	server, store := testServer(t, true)
	saved := agentSave(t, store, "title-reset", "Custom title", "Merge with squash only.", "decision", true)
	status, body := ownerJSON(t, server, "/visualizer/api/update", map[string]any{
		"id": saved.ID, "expected_revision": saved.Revision, "title": "",
		"content": "Merge with squash only.", "purpose": "decision", "request_id": "title-reset-1",
	})
	if status != http.StatusOK {
		t.Fatalf("update status=%d body=%s", status, body)
	}
	current, err := store.RecordByID(context.Background(), saved.ID, nil)
	if err != nil || current == nil || current.Title != "Merge with squash only." {
		t.Fatalf("a cleared title kept %q: %v", current.Title, err)
	}
}
