package gomemory

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func ptr(s string) *string { return &s }

func TestRustFixtureReadParity(t *testing.T) {
	root := filepath.Join("..", "..", "tests", "fixtures", "go-compat")
	r, err := OpenReadOnly(filepath.Join(root, "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	expectedBytes, err := os.ReadFile(filepath.Join(root, "expected.json"))
	if err != nil {
		t.Fatal(err)
	}
	var expected map[string]json.RawMessage
	if err := json.Unmarshal(expectedBytes, &expected); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	global := Scope{}
	projectA := Scope{Project: ptr("id:project-a")}
	mac := Scope{Platform: ptr("macos")}
	windows := Scope{Platform: ptr("windows")}
	for name, got := range map[string]any{
		"global_current":         mustGet(t, r, ctx, global, 1, nil),
		"global_prior":           mustGet(t, r, ctx, global, 1, intPtr(1)),
		"legacy":                 mustGet(t, r, ctx, Scope{Legacy: true}, 6, nil),
		"context_a":              mustContext(t, r, ctx, Scope{Project: projectA.Project, Platform: mac.Platform}, 16000),
		"context_b":              mustContext(t, r, ctx, Scope{Project: ptr("id:project-b"), Platform: mac.Platform}, 16000),
		"context_a_small":        mustContext(t, r, ctx, Scope{Project: projectA.Project, Platform: mac.Platform}, 1024),
		"context_windows":        mustContext(t, r, ctx, windows, 16000),
		"context_macos":          mustContext(t, r, ctx, mac, 16000),
		"search_a_keyword":       mustSearch(t, r, ctx, Scope{Project: projectA.Project, Platform: mac.Platform}, "bun", nil, ""),
		"search_b_keyword":       mustSearch(t, r, ctx, Scope{Project: ptr("id:project-b"), Platform: mac.Platform}, "npm", nil, ""),
		"search_a_semantic":      mustSearch(t, r, ctx, Scope{Project: projectA.Project, Platform: mac.Platform}, "package tool", []float32{1, 0}, "fixture-vector"),
		"search_b_semantic":      mustSearch(t, r, ctx, Scope{Project: ptr("id:project-b"), Platform: mac.Platform}, "package tool", []float32{0, 1}, "fixture-vector"),
		"search_a_wrong_project": mustSearch(t, r, ctx, Scope{Project: projectA.Project, Platform: mac.Platform}, "npm", nil, ""),
	} {
		actualBytes, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		var actualValue, expectedValue any
		if err := json.Unmarshal(actualBytes, &actualValue); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(expected[name], &expectedValue); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(actualValue, expectedValue) {
			t.Errorf("%s differs from Rust fixture\nGo: %s\nRust: %s", name, actualBytes, expected[name])
		}
	}
	if _, err := r.db.Exec("UPDATE chunks SET content='unexpected' WHERE id=1"); err == nil {
		t.Fatal("read-only Go connection accepted a write")
	}
	candidates, err := r.Candidates(ctx, Scope{Project: ptr("id:project-a"), Platform: mac.Platform})
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range candidates {
		if record.ID == 3 || record.ID == 4 || record.ID == 6 {
			t.Fatalf("out-of-scope record %d entered candidate set", record.ID)
		}
	}
}

func TestFullReadUsesApplicableScopeWithoutCrossingProjectOrOS(t *testing.T) {
	root := filepath.Join("..", "..", "tests", "fixtures", "go-compat")
	r, err := OpenReadOnly(filepath.Join(root, "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	project := "id:project-a"
	device := "test-mac"
	platform := "macos"
	caller := Scope{Project: &project, Device: &device, Platform: &platform}
	for _, id := range []int64{1, 2} {
		record, err := r.Get(context.Background(), caller, id, nil)
		if err != nil || record == nil || record.ID != id {
			t.Fatalf("applicable record %d unavailable: %+v %v", id, record, err)
		}
	}
	for _, id := range []int64{3, 4, 6} {
		record, err := r.Get(context.Background(), caller, id, nil)
		if err != nil || record != nil {
			t.Fatalf("out-of-scope record %d disclosed: %+v %v", id, record, err)
		}
	}
	prior, err := r.Get(context.Background(), caller, 1, intPtr(1))
	if err != nil || prior == nil || prior.Revision != 1 {
		t.Fatalf("applicable prior revision unavailable: %+v %v", prior, err)
	}
}

func TestReadOnlyDatabasePathWithURIPunctuation(t *testing.T) {
	source := filepath.Join("..", "..", "tests", "fixtures", "go-compat", "memory.db")
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "pilot #1.db")
	if err := os.WriteFile(destination, data, 0600); err != nil {
		t.Fatal(err)
	}
	r, err := OpenReadOnly(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	record, err := r.Get(context.Background(), Scope{}, 1, nil)
	if err != nil || record == nil {
		t.Fatalf("escaped read-only path failed: record=%v err=%v", record, err)
	}
}

func intPtr(n int64) *int64 { return &n }

func mustGet(t *testing.T, r *Reader, ctx context.Context, scope Scope, id int64, revision *int64) *Record {
	t.Helper()
	got, err := r.Get(ctx, scope, id, revision)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func mustContext(t *testing.T, r *Reader, ctx context.Context, scope Scope, budget int) Page {
	t.Helper()
	got, err := r.Context(ctx, scope, budget)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func mustSearch(t *testing.T, r *Reader, ctx context.Context, scope Scope, query string, vector []float32, model string) Page {
	t.Helper()
	got, err := r.Search(ctx, scope, query, vector, model, 10, 16000)
	if err != nil {
		t.Fatal(err)
	}
	return got
}
