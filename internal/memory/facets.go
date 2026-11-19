package memory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// Areas classify memories by the part of the owner's life they serve (see
// docs/adr/0003-faceted-classification.md). A memory has at most one area.
// Global memories with an area load only into sessions whose project draws on
// that area, unless they are marked everywhere; global memories without an
// area predate areas and keep loading everywhere until the owner classifies
// them. Project, device and platform memories load by scope as before.
//
// Areas live in their own tables, never new chunks columns, so an older
// server build still opens a database this one has used.

// Handoffs older than this stop loading at startup; search still finds them.
const handoffLoadWindow = 14 * 24 * time.Hour

// startupClock is the time startup selection judges handoff age by. Tests
// that read dated fixtures set it to the fixture's time.
var startupClock = time.Now

// EnsureFacetSchema adds the area tables in a transaction without changing
// memory tables.
func (w *Writer) EnsureFacetSchema(ctx context.Context) error {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS areas (
			name TEXT PRIMARY KEY CHECK(length(name) BETWEEN 1 AND 40), created_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS memory_areas (
			memory_id INTEGER PRIMARY KEY REFERENCES chunks(id),
			area TEXT NOT NULL REFERENCES areas(name),
			everywhere INTEGER NOT NULL DEFAULT 0 CHECK(everywhere IN (0,1)))`,
		`CREATE TABLE IF NOT EXISTS project_areas (
			project TEXT NOT NULL, area TEXT NOT NULL REFERENCES areas(name), PRIMARY KEY(project, area))`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// validAreaName accepts short display names such as "Home & family".
func validAreaName(name string) error {
	if name == "" || len(name) > 40 || strings.TrimSpace(name) != name {
		return errors.New("invalid area: use 1 to 40 characters without leading or trailing spaces")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return errors.New("invalid area: control characters are not allowed")
		}
	}
	return nil
}

// CreateArea adds an area to the owner's list. Adding an existing area is a no-op.
func (w *Writer) CreateArea(ctx context.Context, name string) error {
	if err := validAreaName(name); err != nil {
		return err
	}
	_, err := w.db.ExecContext(ctx, "INSERT INTO areas(name,created_at) VALUES(?,?) ON CONFLICT(name) DO NOTHING",
		name, time.Now().UTC().Format(time.RFC3339))
	return err
}

// SetProjectAreas replaces the areas a project draws on.
func (w *Writer) SetProjectAreas(ctx context.Context, project string, areas []string) (err error) {
	if err := (Scope{Project: &project}).durableProject(); err != nil {
		return err
	}
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer commitOrRollback(tx, &err)
	if _, err = tx.ExecContext(ctx, "DELETE FROM project_areas WHERE project=?", project); err != nil {
		return err
	}
	for _, area := range areas {
		if err = requireArea(ctx, tx, area); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO project_areas(project,area) VALUES(?,?)", project, area); err != nil {
			return err
		}
	}
	return nil
}

// Areas lists the owner's areas by name.
func (r *Reader) Areas(ctx context.Context) ([]string, error) {
	return r.names(ctx, "SELECT name FROM areas ORDER BY name")
}

// ProjectAreas lists the areas a project draws on.
func (r *Reader) ProjectAreas(ctx context.Context, project string) ([]string, error) {
	return r.names(ctx, "SELECT area FROM project_areas WHERE project=? ORDER BY area", project)
}

func (r *Reader) names(ctx context.Context, query string, args ...any) ([]string, error) {
	if ok, err := r.hasFacetTables(ctx); err != nil || !ok {
		return []string{}, err
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// hasFacetTables is false for a database no area-aware server has opened,
// such as a backup copy, where every memory is unclassified.
func (r *Reader) hasFacetTables(ctx context.Context) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('areas','memory_areas','project_areas')").Scan(&count)
	return count == 3, err
}

func requireArea(ctx context.Context, tx *sql.Tx, area string) error {
	var found string
	err := tx.QueryRowContext(ctx, "SELECT name FROM areas WHERE name=?", area).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		var names []string
		rows, listErr := tx.QueryContext(ctx, "SELECT name FROM areas ORDER BY name")
		if listErr != nil {
			return listErr
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if scanErr := rows.Scan(&name); scanErr != nil {
				return scanErr
			}
			names = append(names, name)
		}
		return fmt.Errorf("unknown_area: %q is not an area; use one of: %s", area, strings.Join(names, ", "))
	}
	return err
}

// setFacetTx records a memory's area and everywhere flag. Only global
// memories can load everywhere, and a new classification needs an area.
func setFacetTx(ctx context.Context, tx *sql.Tx, scope Scope, id int64, area *string, everywhere *bool) error {
	if area == nil && everywhere == nil {
		return nil
	}
	if everywhere != nil && *everywhere && (scope.Project != nil || scope.Device != nil || scope.Platform != nil) {
		return errors.New("invalid everywhere: only memories with no project, device or platform can load everywhere")
	}
	var currentArea string
	var currentEverywhere bool
	err := tx.QueryRowContext(ctx, "SELECT area,everywhere FROM memory_areas WHERE memory_id=?", id).Scan(&currentArea, &currentEverywhere)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if area != nil {
		if err := requireArea(ctx, tx, *area); err != nil {
			return err
		}
		currentArea = *area
	}
	if everywhere != nil {
		currentEverywhere = *everywhere
	}
	if currentArea == "" {
		return errors.New("area required: give the memory an area before marking it everywhere")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO memory_areas(memory_id,area,everywhere) VALUES(?,?,?)
		ON CONFLICT(memory_id) DO UPDATE SET area=excluded.area, everywhere=excluded.everywhere`, id, currentArea, currentEverywhere)
	return err
}

// copyFacetTx carries a moved memory's area to its new ID. Everywhere is kept
// only when the target is still global.
func copyFacetTx(ctx context.Context, tx *sql.Tx, from, to int64, target Scope) error {
	var tables int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type='table' AND name='memory_areas'").Scan(&tables); err != nil || tables == 0 {
		return err
	}
	global := target.Project == nil && target.Device == nil && target.Platform == nil
	_, err := tx.ExecContext(ctx, `INSERT INTO memory_areas(memory_id,area,everywhere)
		SELECT ?, area, everywhere AND ? FROM memory_areas WHERE memory_id=?`, to, global, from)
	return err
}

// attachFacets fills Area and Everywhere on records that have them.
func (r *Reader) attachFacets(ctx context.Context, records []Record) error {
	if ok, err := r.hasFacetTables(ctx); err != nil || !ok || len(records) == 0 {
		return err
	}
	ids := make([]int64, len(records))
	for i, record := range records {
		ids[i] = record.ID
	}
	idJSON, err := canonicalJSON(ids)
	if err != nil {
		return err
	}
	rows, err := r.db.QueryContext(ctx, "SELECT memory_id,area,everywhere FROM memory_areas WHERE memory_id IN (SELECT value FROM json_each(?))", string(idJSON))
	if err != nil {
		return err
	}
	defer rows.Close()
	type facet struct {
		area       string
		everywhere bool
	}
	facets := map[int64]facet{}
	for rows.Next() {
		var id int64
		var f facet
		if err := rows.Scan(&id, &f.area, &f.everywhere); err != nil {
			return err
		}
		facets[id] = f
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range records {
		if f, ok := facets[records[i].ID]; ok {
			records[i].Area, records[i].Everywhere = f.area, f.everywhere
		}
	}
	return nil
}

// startupSelection is what startup loads for a session and what it leaves out
// before the budget applies. Context and the owner's startup preview share it
// so the two cannot drift apart.
type startupSelection struct {
	selected, olderHandoffs, expiredHandoffs, otherArea []Record
	areas, projectAreas                                []string
}

func (r *Reader) selectStartup(ctx context.Context, scope Scope, eligible []Record, now time.Time) (startupSelection, error) {
	var s startupSelection
	if err := r.attachFacets(ctx, eligible); err != nil {
		return s, err
	}
	var err error
	if s.areas, err = r.Areas(ctx); err != nil {
		return s, err
	}
	s.projectAreas = []string{}
	if scope.Project != nil {
		if s.projectAreas, err = r.ProjectAreas(ctx, *scope.Project); err != nil {
			return s, err
		}
	}
	drawn := make(map[string]bool, len(s.projectAreas))
	for _, area := range s.projectAreas {
		drawn[area] = true
	}
	var current []Record
	for _, record := range eligible {
		global := record.Scope.Project == nil && record.Scope.Device == nil && record.Scope.Platform == nil
		switch {
		case record.Purpose == "handoff" && handoffExpired(record, now):
			s.expiredHandoffs = append(s.expiredHandoffs, record)
		case global && record.Area != "" && !record.Everywhere && !drawn[record.Area]:
			s.otherArea = append(s.otherArea, record)
		default:
			current = append(current, record)
		}
	}
	s.selected, s.olderHandoffs = chooseHandoff(current)
	return s, nil
}

// handoffExpired treats an unreadable timestamp as current, so a malformed
// legacy row is never hidden by mistake.
func handoffExpired(record Record, now time.Time) bool {
	updated, err := time.Parse(time.RFC3339Nano, record.UpdatedAt)
	return err == nil && now.Sub(updated) > handoffLoadWindow
}
