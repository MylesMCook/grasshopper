package gomcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MylesMCook/grasshopper/internal/gomemory"
)

func TestOwnerViewReportsServerVersionModelAndCounts(t *testing.T) {
	_, store := testServer(t)
	active, archivedBefore, err := store.Totals(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	receipt := agentSave(t, store, "status-retired", "Retired", "No longer applies.", "decision", true)
	if _, err := store.Archive(context.Background(), gomemory.ArchiveInput{Scope: gomemory.Scope{Project: observation2project()}, ID: receipt.ID, ExpectedRevision: 1, Archived: true, RequestID: "status-archive", Provenance: ownerProvenance}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		embedder Embedder
		model    string
	}{
		{"with an embedder", constantEmbedder{}, "synthetic-model"},
		{"wording search only", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler, err := NewHandler(Backend{Store: store, Visualizer: true, Version: "9.9.9", Embedder: tc.embedder, Model: "synthetic-model"}, testToken)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(handler)
			defer server.Close()
			status, data := ownerRead(t, server, "/visualizer/api/context", `{"all_projects":true}`)
			if status != http.StatusOK {
				t.Fatalf("status=%d body=%s", status, data)
			}
			var page struct {
				Server struct {
					Version  string `json:"version"`
					Model    string `json:"model"`
					Memories int    `json:"memories"`
					Archived int    `json:"archived"`
				} `json:"server"`
			}
			if err := json.Unmarshal(data, &page); err != nil {
				t.Fatal(err)
			}
			if page.Server.Version != "9.9.9" || page.Server.Model != tc.model || page.Server.Memories != active || page.Server.Archived != archivedBefore+1 {
				t.Fatalf("server status is wrong: %+v (active %d, archived %d)", page.Server, active, archivedBefore)
			}
		})
	}
}

func TestPendingConnectionRequestsReportTimeLeft(t *testing.T) {
	server, _ := testServer(t, true)
	hash := sha256.Sum256([]byte("synthetic-per-device-token-0123456789-abcdef"))
	body, _ := json.Marshal(map[string]any{"device": "work-hp", "token_hash": hex.EncodeToString(hash[:])})
	response, err := http.Post(server.URL+"/pair/start", "application/json", strings.NewReader(string(body)))
	if err != nil || response.StatusCode != http.StatusCreated {
		t.Fatalf("start pairing: %v %v", response, err)
	}
	response.Body.Close()
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/visualizer/api/pairings", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Origin", server.URL)
	listing, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer listing.Body.Close()
	var items []struct {
		Device    string `json:"device"`
		Status    string `json:"status"`
		ExpiresIn *int   `json:"expires_in"`
	}
	if err := json.NewDecoder(listing.Body).Decode(&items); err != nil || len(items) != 1 {
		t.Fatalf("listing: %+v err=%v", items, err)
	}
	if items[0].Status != "pending" || items[0].ExpiresIn == nil || *items[0].ExpiresIn < 1 || *items[0].ExpiresIn > 300 {
		t.Fatalf("a pending request must say how long is left: %+v", items[0])
	}
}
