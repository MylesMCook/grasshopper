package gomemory

import (
	"context"
	"crypto/sha256"
	"path/filepath"
	"testing"
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
	issued, err := w.AddClientToken(ctx, "work-hp", sha256.Sum256([]byte(token)))
	if err != nil || issued.ID < 1 || issued.Device != "work-hp" {
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
