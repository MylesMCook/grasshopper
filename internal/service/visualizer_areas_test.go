package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/MylesMCook/grasshopper/internal/memory"
)

// The owner creates areas, gives a project its areas and classifies a memory;
// invalid requests change nothing and say why.
func TestOwnerManagesAreas(t *testing.T) {
	server, store := testServer(t, true)
	ctx := context.Background()
	for _, name := range []string{"Home", "Work"} {
		if status, _ := ownerJSON(t, server, "/visualizer/api/areas/create", map[string]any{"name": name}); status != http.StatusNoContent {
			t.Fatalf("create area %s: status=%d", name, status)
		}
	}
	if status, _ := ownerJSON(t, server, "/visualizer/api/areas/create", map[string]any{"name": " padded "}); status != http.StatusBadRequest {
		t.Fatalf("a padded area name: status=%d", status)
	}
	if status, _ := ownerJSON(t, server, "/visualizer/api/areas/project", map[string]any{"project": "id:house", "areas": []string{"Home"}}); status != http.StatusNoContent {
		t.Fatalf("set project areas: status=%d", status)
	}
	if status, _ := ownerJSON(t, server, "/visualizer/api/areas/project", map[string]any{"project": "id:house", "areas": []string{"Garden"}}); status != http.StatusBadRequest {
		t.Fatalf("an unknown project area: status=%d", status)
	}
	status, body := ownerJSON(t, server, "/visualizer/api/areas", map[string]any{"project": "id:house"})
	var areas, projectAreas []string
	_ = json.Unmarshal(body["areas"], &areas)
	_ = json.Unmarshal(body["project_areas"], &projectAreas)
	if status != http.StatusOK || strings.Join(areas, ",") != "Home,Work" || strings.Join(projectAreas, ",") != "Home" {
		t.Fatalf("list areas: status=%d areas=%v project=%v", status, areas, projectAreas)
	}

	receipt, err := store.Write(ctx, memory.WriteInput{Scope: memory.Scope{}, Content: "Garage code changes monthly.", Purpose: "decision", Confirmed: true,
		Provenance: memory.Provenance{Harness: "test", Device: "synthetic", Source: "areas"}, RequestID: "area-memory"}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := ownerJSON(t, server, "/visualizer/api/areas/memory", map[string]any{"id": receipt.ID, "area": "Home", "everywhere": true}); status != http.StatusNoContent {
		t.Fatalf("classify memory: status=%d", status)
	}
	records := []memory.Record{{ID: receipt.ID}}
	if err := store.AttachFacets(ctx, records); err != nil || records[0].Area != "Home" || !records[0].Everywhere {
		t.Fatalf("memory area not saved: %+v %v", records, err)
	}
	if current, _ := store.RecordByID(ctx, receipt.ID, nil); current == nil || current.Revision != receipt.Revision {
		t.Fatalf("classifying a memory must not create a revision: %+v", current)
	}
	if status, _ := ownerJSON(t, server, "/visualizer/api/areas/memory", map[string]any{"id": 999999, "area": "Home"}); status != http.StatusNotFound {
		t.Fatalf("a missing memory: status=%d", status)
	}
}
