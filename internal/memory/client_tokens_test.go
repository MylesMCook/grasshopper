package memory

import (
	"context"
	"crypto/sha256"
	"path/filepath"
	"testing"
	"time"
)

func TestClientTokensUpgradeBackupAndRevocation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "memory.db")
	if err := CreateEmpty(path); err != nil {
		t.Fatal(err)
	}
	w, err := OpenWritableExisting(path, "test-model", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	// A pre-pairing database keeps its records and gains only the auth table.
	if _, err := w.db.Exec(`DROP TABLE client_tokens`); err != nil {
		t.Fatal(err)
	}
	if err := w.EnsureClientTokenSchema(ctx); err != nil {
		t.Fatal(err)
	}
	token := "synthetic-device-token-0123456789-abcdef"
	issued, err := w.AddClientToken(ctx, "test-laptop", sha256.Sum256([]byte(token)))
	if err != nil || issued.ID < 1 || issued.Device != "test-laptop" {
		t.Fatalf("issue token: %+v %v", issued, err)
	}
	if valid, err := w.ClientTokenValid(ctx, token); err != nil || !valid {
		t.Fatalf("issued token rejected: %v %v", valid, err)
	}
	if valid, err := w.ClientTokenValid(ctx, "wrong-token"); err != nil || valid {
		t.Fatalf("wrong token accepted: %v %v", valid, err)
	}
	copyPath := filepath.Join(t.TempDir(), "backup.db")
	copy, err := OpenWritableCopy(ctx, path, copyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer copy.Close()
	if valid, err := copy.ClientTokenValid(ctx, token); err != nil || !valid {
		t.Fatalf("restored token missing: %v %v", valid, err)
	}
	if changed, err := w.RevokeClientToken(ctx, issued.ID); err != nil || !changed {
		t.Fatalf("revoke token: %v %v", changed, err)
	}
	if valid, err := w.ClientTokenValid(ctx, token); err != nil || valid {
		t.Fatalf("revoked token accepted: %v %v", valid, err)
	}
	listed, err := w.ListClientTokens(ctx)
	if err != nil || len(listed) != 1 || listed[0].RevokedAt == nil {
		t.Fatalf("revocation not inspectable: %+v %v", listed, err)
	}
}

// Last seen is recorded per active token to the minute and never for a
// revoked or unknown token.
func TestClientTokenLastSeen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "memory.db")
	if err := CreateEmpty(path); err != nil {
		t.Fatal(err)
	}
	w, err := OpenWritableExisting(path, "test-model", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := w.EnsureClientTokenSchema(ctx); err != nil {
		t.Fatal(err)
	}
	active, revoked := "synthetic-active-token-0123456789-abcdef", "synthetic-revoked-token-0123456789-abcdef"
	first, _ := w.AddClientToken(ctx, "test-laptop", sha256.Sum256([]byte(active)))
	second, _ := w.AddClientToken(ctx, "old-desktop", sha256.Sum256([]byte(revoked)))
	if _, err := w.RevokeClientToken(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	seen := func() map[int64]*string {
		items, err := w.ListClientTokens(ctx)
		if err != nil {
			t.Fatal(err)
		}
		out := map[int64]*string{}
		for _, item := range items {
			out[item.ID] = item.LastSeenAt
		}
		return out
	}
	if got := seen(); got[first.ID] != nil {
		t.Fatalf("new token already has activity: %v", *got[first.ID])
	}
	at := time.Date(2026, 10, 1, 9, 30, 45, 0, time.UTC)
	for _, token := range []string{active, revoked, "unknown-token-0123456789-0123456789"} {
		if err := w.TouchClientToken(ctx, token, at); err != nil {
			t.Fatal(err)
		}
	}
	got := seen()
	if got[first.ID] == nil || *got[first.ID] != "2026-10-01T09:30:00Z" || got[second.ID] != nil {
		t.Fatalf("unexpected activity: active=%v revoked=%v", got[first.ID], got[second.ID])
	}
	if err := w.TouchClientToken(ctx, active, at.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got := seen(); *got[first.ID] != "2026-10-01T09:32:00Z" {
		t.Fatalf("activity not updated: %v", *got[first.ID])
	}
}
