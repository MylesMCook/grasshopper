package memory

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestSmallStartupBudgetPrioritizesConfirmedDecisionsBeforeLongHandoff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "startup.db")
	if err := CreateEmpty(path); err != nil {
		t.Fatal(err)
	}
	w, err := OpenWritableExisting(path, "synthetic", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	ctx := context.Background()
	scope := Scope{Project: ptr("id:startup-test")}
	for i := 0; i < 5; i++ {
		_, err := w.Write(ctx, WriteInput{Scope: scope, Content: fmt.Sprintf("%d", i) + strings.Repeat("d", 199), Purpose: "decision", Confirmed: true, Provenance: Provenance{"test", "test", "synthetic"}, RequestID: fmt.Sprintf("decision-%d", i)}, nil, "")
		if err != nil {
			t.Fatal(err)
		}
	}
	before, err := w.Context(ctx, scope, 3000)
	if err != nil || len(before.Records) == 0 {
		t.Fatalf("baseline: %+v %v", before, err)
	}
	_, err = w.Write(ctx, WriteInput{Scope: scope, Content: strings.Repeat("h", 1700), Purpose: "handoff", Provenance: Provenance{"test", "test", "synthetic"}, RequestID: "long-handoff"}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	after, err := w.Context(ctx, scope, 3000)
	if err != nil {
		t.Fatal(err)
	}
	loaded := map[int64]bool{}
	for _, record := range after.Records {
		loaded[record.ID] = true
	}
	for _, record := range before.Records {
		if !loaded[record.ID] {
			t.Fatalf("handoff displaced decision %d", record.ID)
		}
	}
	preview, err := w.StartupPreview(ctx, scope, 3000)
	if err != nil || len(preview.Records) != len(after.Records) {
		t.Fatalf("preview drift: %+v %v", preview, err)
	}
	for i, record := range after.Records {
		if preview.Records[i].ID != record.ID {
			t.Fatal("preview order drift")
		}
	}
	t.Logf("3000-byte budget delivers %d confirmed decisions before considering the 1700-byte handoff; full record metadata is charged to budget", len(before.Records))
}
