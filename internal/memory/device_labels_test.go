package memory

import (
	"context"
	"crypto/sha256"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeviceLabelsRenameWithoutChangingTheID(t *testing.T) {
	ctx := context.Background()
	w, err := OpenWritableCopy(ctx, filepath.Join("..", "..", "tests", "fixtures", "go-compat", "memory.db"), filepath.Join(t.TempDir(), "labels.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := w.EnsureClientTokenSchema(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := w.AddClientToken(ctx, "HPLT-1234", sha256.Sum256([]byte("synthetic-label-token-one-0123456789"))); err != nil {
		t.Fatal(err)
	}
	label := func() *string {
		t.Helper()
		listed, err := w.ListClientTokens(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range listed {
			if token.Device != "HPLT-1234" {
				t.Fatalf("the device ID changed: %+v", token)
			}
		}
		return listed[0].Label
	}
	if err := w.SetDeviceLabel(ctx, "HPLT-1234", "  Work laptop  "); err != nil {
		t.Fatal(err)
	}
	if got := label(); got == nil || *got != "Work laptop" {
		t.Fatalf("label after rename: %v", got)
	}
	// A replacement credential for the same device keeps its name.
	if _, err := w.AddClientToken(ctx, "HPLT-1234", sha256.Sum256([]byte("synthetic-label-token-two-0123456789"))); err != nil {
		t.Fatal(err)
	}
	if listed, _ := w.ListClientTokens(ctx); len(listed) != 2 || listed[0].Label == nil || listed[1].Label == nil {
		t.Fatalf("a rotated token lost its name: %+v", listed)
	}
	if err := w.SetDeviceLabel(ctx, "HPLT-1234", ""); err != nil {
		t.Fatal(err)
	}
	if got := label(); got != nil {
		t.Fatalf("an empty name should show the ID again: %q", *got)
	}
	// The limit counts characters, not bytes, matching the form and SQLite.
	if err := w.SetDeviceLabel(ctx, "HPLT-1234", strings.Repeat("机", 64)); err != nil {
		t.Fatalf("64 multibyte characters rejected: %v", err)
	}
	for _, bad := range []struct{ device, label, want string }{
		{"HPLT-1234", strings.Repeat("机", 65), "invalid device name"},
		{"no-such-device", "Name", "device_not_found"},
		{"HPLT-1234", strings.Repeat("x", 65), "invalid device name"},
		{"HPLT-1234", "line\nbreak", "invalid device name"},
	} {
		if err := w.SetDeviceLabel(ctx, bad.device, bad.label); err == nil || !strings.HasPrefix(err.Error(), bad.want) {
			t.Fatalf("SetDeviceLabel(%q, %q) = %v, want %s", bad.device, bad.label, err, bad.want)
		}
	}
}
