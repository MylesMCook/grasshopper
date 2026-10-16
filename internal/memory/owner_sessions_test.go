package memory

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestOwnerSessionRevocationPersistsAcrossDatabaseReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sessions.db")
	w, err := OpenWritableCopy(ctx, filepath.Join("..", "..", "tests", "fixtures", "go-compat", "memory.db"), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.EnsureOwnerSessionSchema(ctx); err != nil {
		t.Fatal(err)
	}
	if err := w.EnsureOwnerSessionSchema(ctx); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"revoked-browser-cookie", "remaining-browser-cookie"} {
		if err := w.AddOwnerSession(ctx, value, time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.RevokeOwnerSession(ctx, "revoked-browser-cookie", false); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	for _, tc := range []struct {
		value string
		valid bool
	}{{"revoked-browser-cookie", false}, {"remaining-browser-cookie", true}, {"old-stateless-cookie", false}} {
		valid, err := reopened.OwnerSessionValid(ctx, tc.value, time.Now())
		if err != nil || valid != tc.valid {
			t.Fatalf("reopen %s valid%v err%v", tc.value, valid, err)
		}
	}
}

func TestOwnerSessionCapEvictsTheOldestInsteadOfLockingOut(t *testing.T) {
	ctx := context.Background()
	w, err := OpenWritableCopy(ctx, filepath.Join("..", "..", "tests", "fixtures", "go-compat", "memory.db"), filepath.Join(t.TempDir(), "cap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := w.EnsureOwnerSessionSchema(ctx); err != nil {
		t.Fatal(err)
	}
	start := time.Now().Add(time.Hour)
	for i := 0; i < 257; i++ {
		if err := w.AddOwnerSession(ctx, fmt.Sprintf("browser-%d", i), start.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatalf("sign-in %d: %v", i, err)
		}
	}
	now := time.Now()
	if ok, _ := w.OwnerSessionValid(ctx, "browser-0", now); ok {
		t.Fatal("the session closest to expiry was kept past the cap")
	}
	if ok, _ := w.OwnerSessionValid(ctx, "browser-256", now); !ok {
		t.Fatal("the newest session was not saved")
	}
	if ok, _ := w.OwnerSessionValid(ctx, "browser-1", now); !ok {
		t.Fatal("more than one session was evicted")
	}
}
