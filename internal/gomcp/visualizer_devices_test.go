package gomcp

import (
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/MylesMCook/grasshopper/internal/gomemory"
)

func TestVisualizerDeviceChoicesIncludeActiveConnectionsWithoutMemories(t *testing.T) {
	server, store := testServer(t, true)
	ctx := t.Context()
	device, project := "synthetic-laptop", "id:device-choices"
	first, err := store.AddClientToken(ctx, device, sha256.Sum256([]byte("synthetic-first-token")))
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.AddClientToken(ctx, device, sha256.Sum256([]byte("synthetic-second-token")))
	if err != nil {
		t.Fatal(err)
	}
	memoryDevice := "synthetic-memory-only"
	_, err = store.Write(ctx, gomemory.WriteInput{
		Scope: gomemory.Scope{Project: &project, Device: &memoryDevice}, Content: "Synthetic device choice lesson",
		Purpose: "lesson", Confirmed: true, RequestID: "device-choice-memory",
		Provenance: gomemory.Provenance{Harness: "test", Device: "synthetic", Source: "device choice regression"},
	}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	retired, err := store.AddClientToken(ctx, memoryDevice, sha256.Sum256([]byte("synthetic-memory-device-token")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RevokeClientToken(ctx, retired.ID); err != nil {
		t.Fatal(err)
	}
	choices := func(path, body string) []string {
		t.Helper()
		status, data := ownerRead(t, server, path, body)
		if status != http.StatusOK {
			t.Fatalf("%s: status=%d body=%s", path, status, data)
		}
		var view struct {
			Devices []string `json:"devices"`
		}
		if err := json.Unmarshal(data, &view); err != nil {
			t.Fatal(err)
		}
		return view.Devices
	}
	for _, route := range []struct{ path, body string }{
		{"/visualizer/api/context", `{"project":"id:device-choices","platform":"windows"}`},
		{"/visualizer/api/search", `{"scope":{"project":"id:device-choices","platform":"windows"},"query":"lesson"}`},
	} {
		got := choices(route.path, route.body)
		if !slices.Equal(got, []string{device, memoryDevice}) {
			t.Fatalf("active connection without memories missing or duplicated: %v", got)
		}
	}
	if _, err := store.RevokeClientToken(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(choices("/visualizer/api/context", `{}`), device) {
		t.Fatal("revoking one credential hid a device with another active credential")
	}
	if _, err := store.RevokeClientToken(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	for _, route := range []struct{ path, body string }{
		{"/visualizer/api/context", `{"project":"id:device-choices"}`},
		{"/visualizer/api/search", `{"scope":{"project":"id:device-choices"},"query":"lesson"}`},
	} {
		if got := choices(route.path, route.body); !slices.Equal(got, []string{memoryDevice}) {
			t.Fatalf("revoked connection remained or saved memory choice disappeared: %v", got)
		}
	}
	page, err := store.Context(ctx, gomemory.Scope{}, 32768)
	if err != nil || len(page.Records) != 1 || page.Records[0].ID != 1 {
		t.Fatalf("agent context changed: %+v %v", page, err)
	}
}
