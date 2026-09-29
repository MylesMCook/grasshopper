package gomemory

import (
	"context"
	"fmt"
	"testing"
)

// The owner's default view must cost in proportion to the scopes that exist,
// not to every project-by-device combination.
func TestBrowseScopeKeysFollowStoredScopesNotTheirCombinations(t *testing.T) {
	w := fixtureWriter(t)
	ctx := context.Background()
	baseline, _, _, err := w.browseScopeKeys(ctx, BrowseScope{AllProjects: true})
	if err != nil {
		t.Fatal(err)
	}
	const count = 40
	for index := 0; index < count; index++ {
		project, device := fmt.Sprintf("id:project-%d", index), fmt.Sprintf("device-%d", index)
		title := fmt.Sprintf("Scoped %d", index)
		_, err := w.Write(ctx, WriteInput{Scope: Scope{Project: &project, Device: &device}, Title: &title, Content: "Scoped " + title, Purpose: "decision", Confirmed: true,
			Provenance: Provenance{"test", "test", "scope enumeration"}, RequestID: fmt.Sprintf("scope-%d", index)}, nil, "")
		if err != nil {
			t.Fatal(err)
		}
	}
	keys, _, _, err := w.browseScopeKeys(ctx, BrowseScope{AllProjects: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != len(baseline)+count {
		t.Fatalf("%d stored scopes produced %d keys (was %d): keys must not be a Cartesian product", count, len(keys), len(baseline))
	}
	one := "id:project-7"
	keys, _, _, err = w.browseScopeKeys(ctx, BrowseScope{Scope: Scope{Project: &one}})
	if err != nil {
		t.Fatal(err)
	}
	page, err := w.browseKeys(ctx, keys, 32768)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, record := range page.Records {
		if record.Title == "Scoped 8" {
			t.Fatal("another project's memory appeared")
		}
		found = found || record.Title == "Scoped 7"
	}
	if !found {
		t.Fatal("the selected project's memory is missing")
	}
}
