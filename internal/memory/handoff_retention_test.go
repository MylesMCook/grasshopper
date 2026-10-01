package memory

import (
	"context"
	"strings"
	"testing"
)

// A new handoff archives the earlier one in the same exact scope and leaves
// other scopes alone; the replaced handoff stays readable in history.
func TestNewHandoffRetiresTheEarlierOneInItsScope(t *testing.T) {
	w := fixtureWriter(t)
	ctx := context.Background()
	alpha, beta := "id:alpha", "id:beta"
	save := func(project *string, text, request string) Receipt {
		t.Helper()
		receipt, err := w.Write(ctx, WriteInput{
			Scope: Scope{Project: project}, Content: text, Purpose: "handoff",
			Provenance: Provenance{"codex", "synthetic-mac", "end of session"}, RequestID: request,
		}, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		return receipt
	}
	first := save(&alpha, "Alpha: shipped the parser. Next: tests.", "handoff-a1")
	other := save(&beta, "Beta: halfway through the migration.", "handoff-b1")
	second := save(&alpha, "Alpha: tests pass. Next: release.", "handoff-a2")

	replaced, err := w.RecordByID(ctx, first.ID, nil)
	if err != nil || replaced == nil || !replaced.Archived || replaced.Revision != 2 || !strings.HasPrefix(replaced.Provenance.Source, "Replaced by handoff") {
		t.Fatalf("earlier handoff was not archived with a replacement note: %+v %v", replaced, err)
	}
	original, err := w.RecordByID(ctx, first.ID, &first.Revision)
	if err != nil || original == nil || original.Content != "Alpha: shipped the parser. Next: tests." {
		t.Fatalf("replaced handoff lost its original text: %+v %v", original, err)
	}
	for _, check := range []struct {
		project *string
		id      int64
	}{{&alpha, second.ID}, {&beta, other.ID}} {
		current, err := w.Get(ctx, Scope{Project: check.project}, check.id, nil)
		if err != nil || current == nil || current.Archived {
			t.Fatalf("current handoff %d is not active: %+v %v", check.id, current, err)
		}
	}

	// A retried request replays its receipt without retiring anything again.
	again := save(&alpha, "Alpha: tests pass. Next: release.", "handoff-a2")
	if again != second {
		t.Fatalf("retry changed the receipt: %+v vs %+v", again, second)
	}
}
