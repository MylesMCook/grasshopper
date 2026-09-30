package gomemory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

func validOwnerPurpose(purpose string) bool {
	switch purpose {
	case "", "preference", "decision", "lesson", "handoff", "observation":
		return true
	}
	return false
}

// ownerPurposeFilter interpolates only a closed set of literals, never caller SQL.
func ownerPurposeFilter(purpose, prefix string) string {
	if purpose == "" || !validOwnerPurpose(purpose) {
		return ""
	}
	return " AND " + prefix + "purpose='" + purpose + "'"
}

func ownerViewFilter(scope BrowseScope) string {
	filter := ownerPurposeFilter(scope.Purpose, "")
	switch scope.View {
	case ViewReview:
		filter += " AND archived=0 AND confirmed=0 AND purpose<>'handoff'"
	case ViewArchived:
		filter += " AND archived=1"
	default:
		filter += " AND archived=0"
	}
	return filter
}

// BrowseReferences captures an ordered immutable revision set in one SQLite
// statement. Cursors can subsequently load these historical revisions without
// skipping or duplicating memories that an agent updates between pages.
func (r *Reader) BrowseReferences(ctx context.Context, scope BrowseScope, maximum int) ([]Reference, []string, []string, error) {
	_, devices, projects, err := r.browseScopeKeys(ctx, scope, scope.View == ViewArchived)
	if err != nil {
		return nil, nil, nil, err
	}
	args := append(ownerScopeArgs(scope), maximum+1)
	rows, err := r.db.QueryContext(ctx, ownerRowsCTE+`SELECT id,revision,memory_scope FROM owner_rows WHERE `+ownerScopePredicate+ownerViewFilter(scope)+` ORDER BY CASE purpose WHEN 'preference' THEN 0 WHEN 'decision' THEN 1 WHEN 'lesson' THEN 2 WHEN 'handoff' THEN 3 ELSE 4 END,updated_at DESC,id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()
	refs := []Reference{}
	for rows.Next() {
		var ref Reference
		var exact string
		if err := rows.Scan(&ref.ID, &ref.Revision, &exact); err != nil {
			return nil, nil, nil, err
		}
		if err := json.Unmarshal([]byte(exact), &ref.Scope); err != nil {
			return nil, nil, nil, err
		}
		refs = append(refs, ref)
	}
	if len(refs) > maximum {
		return nil, nil, nil, errors.New("owner list exceeds snapshot limit; narrow the filters")
	}
	return refs, devices, projects, rows.Err()
}

// WalkOwnerExport streams complete stored records from one SQLite read snapshot.
// Export never uses preview text or startup budgets and never includes legacy data.
func (r *Reader) WalkOwnerExport(ctx context.Context, scope BrowseScope, includeArchived bool, visit func(Record) error) error {
	if _, err := scope.Key(); err != nil || scope.Legacy || !validOwnerPurpose(scope.Purpose) || (scope.View != "" && scope.View != ViewReview && scope.View != ViewArchived) {
		return errors.New("invalid export filters")
	}
	filter := ownerViewFilter(scope)
	if includeArchived && scope.View != ViewArchived {
		filter = ownerPurposeFilter(scope.Purpose, "")
		if scope.View == ViewReview {
			filter += " AND (archived=1 OR (archived=0 AND confirmed=0 AND purpose<>'handoff'))"
		}
	}
	// Scope enumeration and content selection must be in the same SQLite
	// statement. Two independent reads can combine old scope keys with newer
	// content, producing an export that never existed at any point in time.
	rows, err := r.db.QueryContext(ctx, ownerRowsCTE+"SELECT "+recordColumns+` FROM owner_rows WHERE `+ownerScopePredicate+filter+` ORDER BY id`, ownerScopeArgs(scope)...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return err
		}
		if err := visit(record); err != nil {
			return err
		}
	}
	return rows.Err()
}

// OwnerMemoryVersion changes for every committed memory creation or revision.
// It is independent of bounded preview details and contains no memory text.
func (r *Reader) OwnerMemoryVersion(ctx context.Context) (string, error) {
	var count, revisions int64
	err := r.db.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(revision),0) FROM chunks WHERE kind='memory'`).Scan(&count, &revisions)
	return fmt.Sprintf("%d:%d:", count, revisions), err
}

// BrowseChoices returns owner filter choices without materializing record rows.
func (r *Reader) BrowseChoices(ctx context.Context, scope BrowseScope) ([]string, []string, error) {
	_, devices, projects, err := r.browseScopeKeys(ctx, scope, scope.View == ViewArchived)
	return devices, projects, err
}

// Scope predicates run alongside record selection, so dynamic scope creation
// cannot separate scope visibility from the ordered revision snapshot.
const ownerRowsCTE = `WITH owner_rows AS (SELECT *,CASE WHEN json_valid(memory_scope) THEN memory_scope ELSE '{}' END AS owner_scope FROM chunks WHERE kind='memory' AND memory_scope<>'legacy' AND json_valid(memory_scope)) `
const ownerScopePredicate = `(json_extract(owner_scope,'$.project') IS NULL OR ? OR json_extract(owner_scope,'$.project')=?)
 AND (? IS NULL OR json_extract(owner_scope,'$.device') IS NULL OR json_extract(owner_scope,'$.device')=?)
 AND (? IS NULL OR json_extract(owner_scope,'$.platform') IS NULL OR json_extract(owner_scope,'$.platform')=?)`

func ownerScopeArgs(scope BrowseScope) []any {
	return []any{scope.AllProjects, scope.Project, scope.Device, scope.Device, scope.Platform, scope.Platform}
}
