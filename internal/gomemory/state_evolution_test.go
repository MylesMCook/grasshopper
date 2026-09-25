package gomemory

import (
	"context"
	"strings"
	"testing"
)

// This probe separates current recall from historical inspection after a correction.
// A historical question needs get with a revision; ordinary search is active-only.
func TestStateEvolutionCurrentVsHistory(t *testing.T) {
	w := fixtureWriter(t)
	ctx := context.Background()
	key := "answer-style"
	currentText := "Use the synthetic marker navy comet for this test."
	receipt, err := w.Write(ctx, WriteInput{
		Scope: Scope{}, Content: currentText, Purpose: "preference", Confirmed: true,
		Provenance: Provenance{Harness: "codex", Device: "synthetic-mac", Source: "synthetic state-evolution probe"},
		RequestID:  "state-evolution-correction", Key: &key, ExpectedRevision: intPtr(2),
	}, nil, "")
	if err != nil || receipt.ID != 1 || receipt.Revision != 3 {
		t.Fatalf("correction: %+v %v", receipt, err)
	}
	page, err := w.Context(ctx, Scope{}, 16000)
	if err != nil {
		t.Fatal(err)
	}
	currentSeen := false
	for _, record := range page.Records {
		if record.ID == 1 {
			currentSeen = record.Revision == 3 && record.Content == currentText
		}
	}
	if !currentSeen {
		t.Fatal("context did not expose the current confirmed revision")
	}
	active, err := w.Search(ctx, Scope{}, "navy comet", nil, "", 10, 16000)
	if err != nil || len(active.Records) == 0 || active.Records[0].ID != 1 || active.Records[0].Revision != 3 {
		t.Fatalf("current search: %+v %v", active, err)
	}
	oldWord, err := w.Search(ctx, Scope{}, "concise", nil, "", 10, 16000)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range oldWord.Records {
		if record.ID == 1 {
			t.Fatal("superseded wording entered active search")
		}
	}
	prior, err := w.Get(ctx, Scope{}, 1, intPtr(1))
	if err != nil || prior == nil || !strings.Contains(prior.Content, "concise") || prior.Revision != 1 {
		t.Fatalf("historical evidence: %+v %v", prior, err)
	}
	if prior.Provenance.Source == "" {
		t.Fatal("historical provenance was lost")
	}
	project := "id:unrelated"
	otherContext, err := w.Context(ctx, Scope{Project: &project}, 16000)
	if err != nil {
		t.Fatal(err)
	}
	globalSeen := false
	for _, record := range otherContext.Records {
		if record.ID == 1 {
			globalSeen = record.Revision == 3 && record.Content == currentText
		}
	}
	if !globalSeen {
		t.Fatal("global preference was missing in another project's context")
	}
	// A full read accepts the same applicable scope as context and search.
	full, err := w.Get(ctx, Scope{Project: &project}, 1, intPtr(1))
	if err != nil || full == nil || full.Revision != 1 {
		t.Fatalf("applicable historical evidence unavailable: %+v %v", full, err)
	}
	t.Log("global context and historical get: applicable across projects; prior-only wording: absent from active lexical search")
}
