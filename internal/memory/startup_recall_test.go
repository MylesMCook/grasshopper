package memory

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartupRecallBalancesScopes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	if err := CreateEmpty(path); err != nil {
		t.Fatal(err)
	}
	w, err := OpenWritableExisting(path, "synthetic", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	ctx := context.Background()
	project := Scope{Project: ptr("id:recall")}
	save := func(scope Scope, purpose, label string) {
		t.Helper()
		title := label
		_, err := w.Write(ctx, WriteInput{Scope: scope, Title: &title, Content: label + strings.Repeat(" detail", 90), Purpose: purpose, Confirmed: true, Provenance: Provenance{"test", "synthetic", "startup recall"}, RequestID: label}, nil, "")
		if err != nil {
			t.Fatal(err)
		}
	}
	save(project, "decision", "project decision")
	for i := 0; i < 10; i++ {
		save(Scope{}, "preference", fmt.Sprintf("global preference %d", i))
	}
	save(Scope{Project: ptr("id:other")}, "decision", "other project secret")
	keys, err := project.applicableKeys()
	if err != nil {
		t.Fatal(err)
	}
	records, err := w.scopedRecords(ctx, keys, agentContextFilter, false)
	if err != nil {
		t.Fatal(err)
	}
	oldSelection, _ := chooseHandoff(records)
	oldPage, err := boundRecords(oldSelection, 3000, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range oldPage.Records {
		if record.Scope.Project != nil {
			t.Fatal("synthetic baseline must reproduce project crowding")
		}
	}
	page, err := w.Context(ctx, project, 3000)
	if err != nil {
		t.Fatal(err)
	}
	global, scoped := false, false
	for _, record := range page.Records {
		if record.Scope.Project == nil {
			global = true
		} else if *record.Scope.Project == *project.Project {
			scoped = true
		} else {
			t.Fatal("other project leaked into startup")
		}
	}
	if !global || !scoped {
		t.Fatalf("startup must expose global and project context when both fit: global=%v project=%v omitted=%d", global, scoped, page.Omitted)
	}
	if page.Omitted == 0 {
		t.Fatal("budget exclusions must remain visible")
	}
	search, err := w.Search(ctx, project, "global preference 5", nil, "", 10, 12000)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, record := range search.Records {
		if record.Title == "global preference 5" {
			found = true
			for _, loaded := range page.Records {
				if loaded.ID == record.ID {
					t.Fatal("search case must use an omitted record")
				}
			}
		}
	}
	if !found {
		t.Fatal("task search did not discover the omitted memory")
	}
	other, err := w.Search(ctx, project, "other project secret", nil, "", 10, 12000)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range other.Records {
		if record.Title == "other project secret" {
			t.Fatal("task search crossed the project boundary")
		}
	}
}
