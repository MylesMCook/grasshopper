package memory

import (
	"context"
	"strings"
	"testing"
)

// A memory_type outside the stored kinds is rejected with the accepted values,
// and the message points purpose words at the purpose field.
func TestWriteRejectsUnknownMemoryTypeWithAcceptedValues(t *testing.T) {
	w := fixtureWriter(t)
	wrong := "preference"
	_, err := w.Write(context.Background(), WriteInput{
		Scope: Scope{}, Content: "Synthetic preference.", MemoryType: &wrong, Purpose: "preference", Confirmed: true,
		Provenance: Provenance{"test", "synthetic", "memory type test"}, RequestID: "wrong-memory-type",
	}, nil, "")
	if err == nil || !strings.Contains(err.Error(), "knowledge, identity, episode or procedure") || !strings.Contains(err.Error(), "go in purpose") {
		t.Fatalf("unhelpful memory_type error: %v", err)
	}
}
