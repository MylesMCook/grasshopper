package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"github.com/MylesMCook/grasshopper/internal/memory"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOwnerConfirmPreservesProvenanceAndReplay(t *testing.T) {
	server, store := testServer(t, true)
	saved := agentSave(t, store, "confirm-original", "Original", "An observed preference.", "observation", false)
	body := map[string]any{"action": "confirm", "id": saved.ID, "expected_revision": saved.Revision, "request_id": "confirm-owner"}
	for i := 0; i < 2; i++ {
		status, _ := ownerJSON(t, server, "/visualizer/api/update", body)
		if status != 200 {
			t.Fatalf("confirm/replay status %d", status)
		}
	}
	record, err := store.RecordByID(context.Background(), saved.ID, nil)
	if err != nil || !record.Confirmed || record.Provenance.Harness != "codex-desktop" || record.Revision != 2 {
		t.Fatalf("confirm changed source: %+v %v", record, err)
	}
}

func TestOwnerExportHasFullContentAndArchivedChoice(t *testing.T) {
	server, store := testServer(t, true)
	content := strings.Repeat("Long memory. ", 1500)
	saved := agentSave(t, store, "export-long", "Export original", content, "lesson", true)
	status, data := ownerRead(t, server, "/visualizer/api/export", `{"scope":{"project":"id:review"},"include_archived":false}`)
	var out struct {
		Records []memory.Record `json:"records"`
	}
	_ = json.Unmarshal(data, &out)
	var exported *memory.Record
	for i := range out.Records {
		if out.Records[i].ID == saved.ID {
			exported = &out.Records[i]
		}
	}
	if status != 200 || exported == nil || exported.Content != content || exported.ContentTruncated {
		t.Fatalf("export incomplete: status=%d records=%d", status, len(out.Records))
	}
	_, err := store.Archive(context.Background(), memory.ArchiveInput{Scope: exported.Scope, ID: saved.ID, ExpectedRevision: 1, Archived: true, RequestID: "export-archive", Provenance: ownerProvenance})
	if err != nil {
		t.Fatal(err)
	}
	status, data = ownerRead(t, server, "/visualizer/api/export", `{"scope":{"project":"id:review"},"include_archived":true}`)
	_ = json.Unmarshal(data, &out)
	exported = nil
	for i := range out.Records {
		if out.Records[i].ID == saved.ID {
			exported = &out.Records[i]
		}
	}
	if status != http.StatusOK || exported == nil || !exported.Archived {
		t.Fatalf("archived export status=%d records=%+v", status, out.Records)
	}
}

func TestOwnerPaginationSnapshotTraversesConcurrentWrites(t *testing.T) {
	server, store := testServer(t, true)
	for i := 0; i < 300; i++ {
		agentSave(t, store, "page-"+jsonNumber(int64(i)), "Page "+jsonNumber(int64(i)), "Snapshot record "+jsonNumber(int64(i)), "observation", false)
	}
	after := ""
	seen := map[int64]bool{}
	total := 0
	for pageNumber := 0; pageNumber < 20; pageNumber++ {
		body, _ := json.Marshal(map[string]any{"project": "id:review", "purpose": "observation", "limit": 31, "after": after})
		status, data := ownerRead(t, server, "/visualizer/api/context", string(body))
		var page struct {
			Records []memory.Record `json:"records"`
			Next    string          `json:"next"`
			Total   int             `json:"total"`
		}
		if err := json.Unmarshal(data, &page); err != nil {
			t.Fatalf("status%d %s", status, data)
		}
		if status != 200 || page.Total != 300 {
			t.Fatalf("bad page%d status%d total%d", pageNumber, status, page.Total)
		}
		if pageNumber == 0 {
			total = page.Total
			// Update records not yet visited so mutable keysets would move them ahead
			// of the cursor and silently lose them. The snapshot keeps old revisions.
			for _, id := range []int64{page.Records[len(page.Records)-1].ID - 1, page.Records[len(page.Records)-1].ID - 2} {
				rec, err := store.RecordByID(context.Background(), id, nil)
				if err != nil || rec == nil {
					t.Fatal(err)
				}
				_, err = store.Write(context.Background(), memory.WriteInput{ID: &id, ExpectedRevision: &rec.Revision, Scope: rec.Scope, Content: "Concurrent replacement " + jsonNumber(id), Purpose: rec.Purpose, Confirmed: false, RequestID: "concurrent-" + jsonNumber(id), Provenance: ownerProvenance}, nil, "")
				if err != nil {
					t.Fatal(err)
				}
			}
			agentSave(t, store, "page-new", "New after snapshot", "Not in old snapshot", "observation", false)
		}
		for _, record := range page.Records {
			if seen[record.ID] || record.Revision != 1 {
				t.Fatalf("duplicate or changed snapshot %+v", record)
			}
			seen[record.ID] = true
		}
		after = page.Next
		if after == "" {
			break
		}
	}
	if len(seen) != total {
		t.Fatalf("lost records: got%d expected%d", len(seen), total)
	}
}

func TestOwnerConditionalResponsesAndPreviewBound(t *testing.T) {
	server, store := testServer(t, true)
	for i := 0; i < 130; i++ {
		agentSave(t, store, "preview-"+jsonNumber(int64(i)), "Unconfirmed "+jsonNumber(int64(i)), "Do not load me "+jsonNumber(int64(i)), "observation", false)
	}
	for _, route := range []struct{ path, body string }{{"/visualizer/api/context", `{"all_projects":true}`}, {"/visualizer/api/startup", `{"scope":{"project":"id:review"},"budget":3000}`}} {
		request := func(tag string) (int, string, []byte) {
			req, _ := http.NewRequest(http.MethodPost, server.URL+route.path, strings.NewReader(route.body))
			req.Header.Set("Authorization", "Bearer "+testToken)
			req.Header.Set("Origin", server.URL)
			req.Header.Set("If-None-Match", tag)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			data, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			return resp.StatusCode, resp.Header.Get("ETag"), data
		}
		status, etag, data := request("")
		if status != 200 || etag == "" {
			t.Fatalf("initial %d %s", status, data)
		}
		if route.path == "/visualizer/api/startup" {
			var preview memory.Startup
			_ = json.Unmarshal(data, &preview)
			if len(preview.NotLoaded) != 100 || preview.NotLoadedTotal < 130 {
				t.Fatalf("unbounded or imprecise preview %+v", preview)
			}
		}
		status, _, data = request(etag)
		if status != 304 || len(data) != 0 {
			t.Fatalf("conditional status%d body%s", status, data)
		}
		agentSave(t, store, "conditional-"+route.path, "Changed by write", "Changed context "+route.path, "decision", true)
		status, newTag, _ := request(etag)
		if status != 200 || newTag == etag {
			t.Fatalf("write did not change ETag %d %s", status, newTag)
		}
	}
}

func TestOwnerSessionsRevokeCopiedCookiesAndEverywhere(t *testing.T) {
	server, store := testServer(t, true)
	login := func() *http.Cookie {
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/visualizer/api/session", nil)
		req.Header.Set("Origin", server.URL)
		req.Header.Set("Authorization", "Bearer "+testToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 204 || len(resp.Cookies()) != 1 {
			t.Fatalf("login %d", resp.StatusCode)
		}
		return resp.Cookies()[0]
	}
	first, second := login(), login()
	call := func(method, path string, cookie *http.Cookie) int {
		req, _ := http.NewRequest(method, server.URL+path, strings.NewReader(`{}`))
		req.Header.Set("Origin", server.URL)
		req.AddCookie(cookie)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if got := call(http.MethodDelete, "/visualizer/api/session", first); got != 204 {
		t.Fatalf("logout %d", got)
	}
	if got := call(http.MethodPost, "/visualizer/api/context", first); got != 401 {
		t.Fatalf("copied logged-out cookie accepted %d", got)
	}
	if got := call(http.MethodPost, "/visualizer/api/context", second); got != 200 {
		t.Fatalf("other session affected %d", got)
	}
	if got := call(http.MethodPost, "/visualizer/api/session/revoke-all", second); got != 204 {
		t.Fatalf("revoke all %d", got)
	}
	if got := call(http.MethodPost, "/visualizer/api/context", second); got != 401 {
		t.Fatalf("everywhere copy accepted %d", got)
	}
	// A fresh handler represents a process restart against the same persistent DB.
	handler, err := NewHandler(Backend{Store: store, Visualizer: true}, testToken)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/visualizer/api/context", strings.NewReader(`{}`))
	req.Header.Set("Origin", "http://127.0.0.1")
	req.AddCookie(second)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != 401 {
		t.Fatalf("revocation lost on restart %d", response.Code)
	}
	status, _ := ownerRead(t, server, "/visualizer/api/context", `{}`)
	if status != 200 {
		t.Fatalf("master bearer affected %d", status)
	}
	paired := "synthetic-paired-device-token-0123456789"
	if _, err := store.AddClientToken(context.Background(), "paired", sha256.Sum256([]byte(paired))); err != nil {
		t.Fatal(err)
	}
	if valid, err := store.ClientTokenValid(context.Background(), paired); !valid || err != nil {
		t.Fatalf("paired access affected %v %v", valid, err)
	}
}

func TestOwnerLoginAttemptsAreBounded(t *testing.T) {
	server, _ := testServer(t, true)
	for i := 0; i < 11; i++ {
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/visualizer/api/session", nil)
		req.Header.Set("Origin", server.URL)
		req.Header.Set("Authorization", "Bearer wrong")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		expected := 401
		if i == 10 {
			expected = 429
		}
		if resp.StatusCode != expected {
			t.Fatalf("attempt%d status%d expected%d", i, resp.StatusCode, expected)
		}
	}
}

func TestOwnerCursorBindsFiltersLimitAndExpires(t *testing.T) {
	_, store := testServer(t, true)
	for i := 0; i < 4; i++ {
		agentSave(t, store, "cursor-"+jsonNumber(int64(i)), "Cursor "+jsonNumber(int64(i)), "Cursor text "+jsonNumber(int64(i)), "observation", false)
	}
	pager := &ownerPager{snapshots: map[string]ownerSnapshot{}}
	input := ownerListInput{BrowseScope: memory.BrowseScope{AllProjects: true, Purpose: "observation"}, Limit: 2}
	_, _, _, next, _, err := pager.page(context.Background(), store, input)
	if err != nil || next == "" {
		t.Fatalf("first page %v", err)
	}
	input.After = next
	input.Limit = 3
	if _, _, _, _, _, err := pager.page(context.Background(), store, input); err == nil {
		t.Fatal("cursor accepted changed limit")
	}
	input.Limit = 2
	input.Purpose = "lesson"
	if _, _, _, _, _, err := pager.page(context.Background(), store, input); err == nil {
		t.Fatal("cursor accepted changed purpose")
	}
	input.Purpose = "observation"
	for key, snapshot := range pager.snapshots {
		snapshot.expires = time.Now().Add(-time.Second)
		pager.snapshots[key] = snapshot
	}
	if _, _, _, _, _, err := pager.page(context.Background(), store, input); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired cursor %v", err)
	}
}

func TestOwnerPurposeFilterAppliesBeforeSearchAndList(t *testing.T) {
	server, store := testServer(t, true)
	agentSave(t, store, "purpose-preference", "Orchard preference", "Orchard planting rule", "preference", true)
	agentSave(t, store, "purpose-lesson", "Orchard lesson", "Orchard learned rule", "lesson", true)
	for _, route := range []struct{ path, body string }{{"/visualizer/api/context", `{"all_projects":true,"purpose":"lesson"}`}, {"/visualizer/api/search", `{"scope":{"all_projects":true,"purpose":"lesson"},"query":"orchard"}`}} {
		status, data := ownerRead(t, server, route.path, route.body)
		var page memory.Page
		_ = json.Unmarshal(data, &page)
		if status != 200 || len(page.Records) == 0 {
			t.Fatalf("filtered request %d %s", status, data)
		}
		for _, record := range page.Records {
			if record.Purpose != "lesson" {
				t.Fatalf("wrong kind in results %+v", record)
			}
		}
	}
	for _, route := range []struct{ path, body string }{{"/visualizer/api/context", `{"purpose":"invalid"}`}, {"/visualizer/api/search", `{"scope":{"purpose":"invalid"},"query":"orchard"}`}} {
		status, _ := ownerRead(t, server, route.path, route.body)
		if status != http.StatusBadRequest {
			t.Fatalf("invalid purpose status %d", status)
		}
	}
}

func TestOwnerExportRejectsForeignOriginAndPairedCredential(t *testing.T) {
	server, store := testServer(t, true)
	paired := "synthetic-paired-device-token-0123456789"
	if _, err := store.AddClientToken(context.Background(), "paired", sha256.Sum256([]byte(paired))); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		token, origin string
		status        int
	}{{"", server.URL, 401}, {paired, server.URL, 401}, {testToken, "https://foreign.example", 403}, {testToken, "", 403}} {
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/visualizer/api/export", strings.NewReader(`{"all":true}`))
		req.Header.Set("Authorization", "Bearer "+tc.token)
		req.Header.Set("Origin", tc.origin)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != tc.status {
			t.Fatalf("export auth %d expected%d", resp.StatusCode, tc.status)
		}
	}
}

func TestValidOwnerLoginBypassesAndResetsFailureLimit(t *testing.T) {
	server, _ := testServer(t, true)
	call := func(token string) int {
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/visualizer/api/session", nil)
		req.Header.Set("Origin", server.URL)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	for i := 0; i < 11; i++ {
		call("wrong")
	}
	if status := call(testToken); status != 204 {
		t.Fatalf("valid owner locked out %d", status)
	}
	if status := call("wrong"); status != 401 {
		t.Fatalf("successful login did not reset failures %d", status)
	}
	for i := 0; i < 11; i++ {
		if status := call(testToken); status != 204 {
			t.Fatalf("successful signins limited %d", status)
		}
	}
}

func TestReviewExportIncludesArchivedWhenRequested(t *testing.T) {
	server, store := testServer(t, true)
	archived := agentSave(t, store, "export-reviewed-archive", "Archived reviewed", "Archived reviewed memory", "lesson", true)
	agentSave(t, store, "export-pending", "Pending lesson", "Pending lesson memory", "lesson", false)
	agentSave(t, store, "export-settled", "Settled lesson", "Settled lesson memory", "lesson", true)
	_, err := store.Archive(context.Background(), memory.ArchiveInput{Scope: memory.Scope{Project: observation2project()}, ID: archived.ID, ExpectedRevision: 1, Archived: true, RequestID: "export-reviewed-archive-action", Provenance: ownerProvenance})
	if err != nil {
		t.Fatal(err)
	}
	status, data := ownerRead(t, server, "/visualizer/api/export", `{"scope":{"project":"id:review","purpose":"lesson","view":"review"},"include_archived":true}`)
	var out struct {
		Records []memory.Record `json:"records"`
	}
	_ = json.Unmarshal(data, &out)
	foundArchive, foundPending := false, false
	for _, record := range out.Records {
		if record.ID == archived.ID {
			foundArchive = true
		}
		if record.Title == "Pending lesson" {
			foundPending = true
		}
		if !record.Archived && record.Confirmed {
			t.Fatalf("review exported confirmed active %+v", record)
		}
	}
	if status != 200 || !foundArchive || !foundPending {
		t.Fatalf("review archive inclusion %d %+v", status, out)
	}
	status, data = ownerRead(t, server, "/visualizer/api/export", `{"scope":{"project":"id:review","view":"archived"},"include_archived":true}`)
	_ = json.Unmarshal(data, &out)
	if status != 200 || len(out.Records) == 0 {
		t.Fatalf("archived view export %d", status)
	}
	for _, record := range out.Records {
		if !record.Archived {
			t.Fatalf("archived view widened to active %+v", record)
		}
	}
}
