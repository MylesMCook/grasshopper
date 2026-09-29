package gomcp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MylesMCook/grasshopper/internal/gomemory"
)

func ownerRead(t *testing.T, server *httptest.Server, path, body string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Origin", server.URL)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, data
}

func TestVisualizerSearchFullRecordAndHistory(t *testing.T) {
	server, store := testServer(t, true)
	project, device, platform := "id:owner-read", "other-device", "windows"
	scope := gomemory.Scope{Project: &project, Device: &device, Platform: &platform}
	title := "Synthetic orchard decision"
	before := "The synthetic orchard uses a local backup."
	input := gomemory.WriteInput{Scope: scope, Title: &title, Content: before, Purpose: "decision", Confirmed: true, Provenance: gomemory.Provenance{Harness: "test", Device: "synthetic", Source: "owner read regression"}, RequestID: "owner-record"}
	receipt, err := store.Write(context.Background(), input, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	input.ID, input.ExpectedRevision = &receipt.ID, &receipt.Revision
	input.Content = strings.Repeat("The synthetic orchard uses an encrypted backup. ", 1000)[:32768]
	input.RequestID = "owner-correction"
	if _, err := store.Write(context.Background(), input, nil, ""); err != nil {
		t.Fatal(err)
	}
	status, data := ownerRead(t, server, "/visualizer/api/search", `{"scope":{"project":"id:owner-read"},"query":"orchard"}`)
	if status != http.StatusOK {
		t.Fatalf("search status=%d body=%s", status, data)
	}
	var page gomemory.Page
	if err := json.Unmarshal(data, &page); err != nil {
		t.Fatal(err)
	}
	if page.Omitted != 0 || len(page.Records) != 1 || page.Records[0].ID != receipt.ID || !page.Records[0].ContentTruncated {
		t.Fatalf("large cross-device result not discoverable as a preview: %+v", page)
	}
	for _, revision := range []int64{1, 2} {
		body, _ := json.Marshal(map[string]any{"scope": scope, "id": receipt.ID, "revision": revision})
		status, data := ownerRead(t, server, "/visualizer/api/record", string(body))
		if status != http.StatusOK {
			t.Fatalf("revision %d status=%d body=%s", revision, status, data)
		}
		var record gomemory.Record
		if err := json.Unmarshal(data, &record); err != nil {
			t.Fatal(err)
		}
		want := before
		if revision == 2 {
			want = input.Content
		}
		if record.Content != want || record.Revision != revision || !record.Confirmed || record.UpdatedAt == "" || record.Provenance.Source != input.Provenance.Source {
			t.Fatalf("incomplete revision %d: %+v", revision, record)
		}
	}
	status, _ = ownerRead(t, server, "/visualizer/api/record", `{"scope":{"project":"id:another"},"id":`+jsonNumber(receipt.ID)+`}`)
	if status != http.StatusNotFound {
		t.Fatalf("wrong project record status=%d", status)
	}
	status, _ = ownerRead(t, server, "/visualizer/api/record", `{"scope":{},"id":6}`)
	if status != http.StatusNotFound {
		t.Fatalf("legacy record escaped quarantine: %d", status)
	}
	status, data = ownerRead(t, server, "/visualizer/api/search", `{"scope":{"project":"id:another"},"query":"orchard"}`)
	if status != http.StatusOK {
		t.Fatalf("other project search: %d", status)
	}
	if err := json.Unmarshal(data, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 0 || page.Omitted != 0 {
		t.Fatalf("cross-project search leaked: %+v", page)
	}
}

func jsonNumber(value int64) string { data, _ := json.Marshal(value); return string(data) }

func TestVisualizerOwnerReadsRejectUntrustedAndMalformedRequests(t *testing.T) {
	server, store := testServer(t, true)
	paired := "synthetic-paired-device-token-0123456789"
	if _, err := store.AddClientToken(context.Background(), "paired", sha256.Sum256([]byte(paired))); err != nil {
		t.Fatal(err)
	}
	key := sha256.Sum256([]byte(testToken))
	expired, err := newVisualizerSession(key[:], time.Now().Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	valid, err := newVisualizerSession(key[:], time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []struct{ path, body string }{
		{"/visualizer/api/search", `{"scope":{},"query":"answer"}`},
		{"/visualizer/api/record", `{"scope":{},"id":1}`},
	} {
		for _, tc := range []struct {
			name, token, origin, cookie string
			status                      int
		}{
			{"anonymous", "", server.URL, "", 401},
			{"paired token", paired, server.URL, "", 401},
			{"wrong origin", testToken, "https://other.example", "", 403},
			{"absent origin", testToken, "", "", 403},
			{"expired cookie", "", server.URL, expired, 401},
			{"valid cookie wrong origin", "", "https://other.example", valid, 403},
			{"valid owner cookie", "", server.URL, valid, 200},
		} {
			t.Run(route.path+"/"+tc.name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodPost, server.URL+route.path, strings.NewReader(route.body))
				if tc.token != "" {
					req.Header.Set("Authorization", "Bearer "+tc.token)
				}
				if tc.origin != "" {
					req.Header.Set("Origin", tc.origin)
				}
				if tc.cookie != "" {
					req.AddCookie(&http.Cookie{Name: visualizerCookieName, Value: tc.cookie})
				}
				response := httptest.NewRecorder()
				server.Config.Handler.ServeHTTP(response, req)
				if response.Code != tc.status || response.Header().Get("Cache-Control") != "no-store" {
					t.Fatalf("status=%d expected=%d body=%s", response.Code, tc.status, response.Body.String())
				}
			})
		}
	}
	for _, tc := range []struct{ path, body string }{
		{"search", `{"scope":{"legacy":true},"query":"answer"}`},
		{"search", `{"scope":{},"query":""}`},
		{"search", `{"scope":{},"query":"answer","unknown":1}`},
		{"search", `{"scope":{},"query":"answer"} {}`},
		{"search", `{"scope":{},"query":"` + strings.Repeat("x", 4097) + `"}`},
		{"record", `{"scope":{"platform":"darwin"},"id":1}`},
		{"record", `{"scope":{},"id":1,"revision":0}`},
		{"record", `{"scope":{},"id":0}`},
		{"record", `{"scope":{},"id":1,"extra":true}`},
		{"record", `{"scope":{},"id":1} {}`},
	} {
		status, _ := ownerRead(t, server, "/visualizer/api/"+tc.path, tc.body)
		if status != http.StatusBadRequest {
			t.Fatalf("accepted invalid %s request: %d", tc.path, status)
		}
	}
}

func TestVisualizerSemanticSearchSharesRankingAndDisclosesFallback(t *testing.T) {
	_, store := testServer(t)
	project, device, platform := "id:semantic-owner", "another-device", "windows"
	scope := gomemory.Scope{Project: &project, Device: &device, Platform: &platform}
	receipt, err := store.Write(context.Background(), gomemory.WriteInput{Scope: scope, Content: "Protect synthetic records with a remote copy.", Purpose: "decision", Confirmed: true, Provenance: gomemory.Provenance{Harness: "test", Device: "test", Source: "semantic owner regression"}, RequestID: "owner-semantic"}, []float32{0, 1}, "owner-test")
	if err != nil {
		t.Fatal(err)
	}
	for _, embedder := range []Embedder{constantEmbedder{}, failedEmbedder{}} {
		handler, err := NewHandler(Backend{Store: store, Visualizer: true, Embedder: embedder, Model: "owner-test"}, testToken)
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(handler)
		func() {
			defer server.Close()
			status, data := ownerRead(t, server, "/visualizer/api/search", `{"scope":{"project":"id:semantic-owner"},"query":"remote archive"}`)
			if status != http.StatusOK {
				t.Fatalf("search status=%d body=%s", status, data)
			}
			var result struct {
				gomemory.Page
				SemanticReady bool `json:"semantic_ready"`
			}
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatal(err)
			}
			_, ready := embedder.(constantEmbedder)
			if result.SemanticReady != ready || len(result.Records) != 1 || result.Records[0].ID != receipt.ID {
				t.Fatalf("owner semantic/fallback result: %+v", result)
			}
			if ready {
				agentPage, err := store.Search(context.Background(), gomemory.Scope{Project: &project}, "remote archive", []float32{0, 1}, "owner-test", 100, 32768)
				if err != nil || len(agentPage.Records) != 0 {
					t.Fatalf("agent scope broadened: %+v %v", agentPage, err)
				}
				matchingPage, err := store.Search(context.Background(), scope, "remote archive", []float32{0, 1}, "owner-test", 100, 32768)
				if err != nil || len(matchingPage.Records) != 1 || matchingPage.Records[0].ID != result.Records[0].ID {
					t.Fatalf("ranking differs: %+v %v", matchingPage, err)
				}
			}
		}()
	}
}

func TestVisualizerStalledInferenceReturnsWordingFallback(t *testing.T) {
	_, store := testServer(t)
	receipt, err := store.Write(context.Background(), gomemory.WriteInput{Content: "Synthetic orchard recovery guidance.", Purpose: "lesson", Confirmed: true, Provenance: gomemory.Provenance{Harness: "test", Device: "test", Source: "stalled inference regression"}, RequestID: "stalled-wording"}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	defer close(release)
	handler, err := NewHandler(Backend{Store: store, Visualizer: true, Embedder: stalledEmbedder{release: release}, Model: "stalled-test", InferenceTimeout: 20 * time.Millisecond}, testToken)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	status, data := ownerRead(t, server, "/visualizer/api/search", `{"scope":{},"query":"orchard"}`)
	if status != http.StatusOK {
		t.Fatalf("wording fallback status=%d body=%s", status, data)
	}
	var result struct {
		gomemory.Page
		SemanticReady bool `json:"semantic_ready"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.SemanticReady || len(result.Records) != 1 || result.Records[0].ID != receipt.ID {
		t.Fatalf("stalled inference lost wording results: %+v", result)
	}
}
