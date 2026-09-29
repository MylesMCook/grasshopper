package gomcp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/MylesMCook/grasshopper/internal/gomemory"
)

func TestVisualizerAllProjectsShowsEveryProjectAndKeepsFilters(t *testing.T) {
	server, store := testServer(t, true)
	ctx := context.Background()
	provenance := gomemory.Provenance{Harness: "test", Device: "synthetic", Source: "all projects regression"}
	save := func(request, title, content string, scope gomemory.Scope) int64 {
		receipt, err := store.Write(ctx, gomemory.WriteInput{Scope: scope, Title: &title, Content: content, Purpose: "decision", Confirmed: true, Provenance: provenance, RequestID: request}, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		return receipt.ID
	}
	alpha, beta, laptop, mac := "id:alpha", "id:beta", "laptop", "mac"
	save("all-global", "Global orchard", "Every project shares the orchard rule.", gomemory.Scope{})
	alphaID := save("all-alpha", "Alpha orchard", "Alpha keeps its own orchard notes.", gomemory.Scope{Project: &alpha})
	save("all-beta", "Beta orchard", "Beta keeps its own orchard notes.", gomemory.Scope{Project: &beta})
	save("all-beta-laptop", "Beta laptop orchard", "Only the laptop sees this orchard note.", gomemory.Scope{Project: &beta, Device: &laptop})
	save("all-beta-mac", "Beta mac orchard", "Only the mac sees this orchard note.", gomemory.Scope{Project: &beta, Device: &mac})
	large := strings.Repeat("Large orchard record. ", 2000)[:32768]
	save("all-large", "Large alpha orchard", large, gomemory.Scope{Project: &alpha})

	titles := func(path, body string) (map[string]bool, gomemory.Page, map[string]string) {
		status, data := ownerRead(t, server, path, body)
		if status != http.StatusOK {
			t.Fatalf("%s %s status=%d body=%s", path, body, status, data)
		}
		var page struct {
			gomemory.Page
			OmittedTitles map[string]string `json:"omitted_titles"`
		}
		if err := json.Unmarshal(data, &page); err != nil {
			t.Fatal(err)
		}
		found := map[string]bool{}
		for _, record := range page.Records {
			found[record.Title] = true
		}
		return found, page.Page, page.OmittedTitles
	}

	found, _, _ := titles("/visualizer/api/context", `{}`)
	if !found["Global orchard"] || found["Alpha orchard"] || found["Beta orchard"] {
		t.Fatalf("default view must stay global-only: %v", found)
	}
	found, page, _ := titles("/visualizer/api/context", `{"all_projects":true}`)
	for _, want := range []string{"Global orchard", "Alpha orchard", "Beta orchard", "Beta laptop orchard", "Beta mac orchard"} {
		if !found[want] {
			t.Fatalf("all projects omitted %q: %v", want, found)
		}
	}
	if page.Omitted != 0 || !found["Large alpha orchard"] {
		t.Fatalf("a large record must appear as a preview, not be omitted: %+v", page)
	}
	found, _, _ = titles("/visualizer/api/context", `{"all_projects":true,"device":"laptop"}`)
	if !found["Beta laptop orchard"] || found["Beta mac orchard"] || !found["Beta orchard"] {
		t.Fatalf("device filter lost inside all projects: %v", found)
	}
	found, _, _ = titles("/visualizer/api/search", `{"scope":{"all_projects":true},"query":"orchard"}`)
	if !found["Alpha orchard"] || !found["Beta orchard"] || !found["Global orchard"] {
		t.Fatalf("all-project search missed a project: %v", found)
	}
	status, _ := ownerRead(t, server, "/visualizer/api/record", `{"scope":{"all_projects":true},"id":`+jsonNumber(alphaID)+`}`)
	if status != http.StatusOK {
		t.Fatalf("all-project record read=%d", status)
	}
	status, _ = ownerRead(t, server, "/visualizer/api/record", `{"scope":{},"id":`+jsonNumber(alphaID)+`}`)
	if status != http.StatusNotFound {
		t.Fatalf("global-only view read a project record: %d", status)
	}
	status, _ = ownerRead(t, server, "/visualizer/api/context", `{"all_projects":true,"legacy":true}`)
	if status != http.StatusBadRequest {
		t.Fatalf("all projects must not reach legacy records: %d", status)
	}
}
