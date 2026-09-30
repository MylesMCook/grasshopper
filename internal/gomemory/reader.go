// Package gomemory stores scoped memories, revision history, search indexes,
// and client-token metadata in SQLite.
package gomemory

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"

	_ "modernc.org/sqlite"
)

// Scope identifies project, device, and platform dimensions for a memory.
// A nil dimension is unscoped; Legacy selects isolated pre-scope records.
type Scope struct {
	Project  *string `json:"project"`
	Device   *string `json:"device"`
	Platform *string `json:"platform"`
	Legacy   bool    `json:"legacy"`
}

// Key validates a scope and returns its stable database key.
func (s Scope) Key() (string, error) {
	if s.Legacy {
		if s.Project != nil || s.Device != nil || s.Platform != nil {
			return "", errors.New("legacy scope cannot have dimensions")
		}
		return "legacy", nil
	}
	for _, value := range []*string{s.Project, s.Device} {
		if value == nil {
			continue
		}
		if strings.TrimSpace(*value) == "" || len(*value) > 512 || strings.IndexFunc(*value, unicode.IsControl) >= 0 {
			return "", errors.New("invalid scope identifier")
		}
	}
	if s.Project != nil && (strings.ContainsAny(*s.Project, "@?#\\") || strings.Contains(*s.Project, "://")) {
		return "", errors.New("project must be a sanitized durable ID")
	}
	if s.Platform != nil && *s.Platform != "macos" && *s.Platform != "windows" && *s.Platform != "linux" {
		return "", errors.New("invalid platform")
	}
	data, err := json.Marshal(s)
	return string(data), err
}

// New project records need an identity that survives clone paths and machines.
// Key still accepts older identifiers so existing records remain inspectable.
func (s Scope) durableProject() error {
	if s.Project == nil {
		return nil
	}
	project := *s.Project
	if strings.HasPrefix(project, "id:") && len(project) > len("id:") {
		return nil
	}
	if strings.HasPrefix(project, "git:") && len(project) > len("git:") && strings.Contains(project[len("git:"):], "/") {
		return nil
	}
	return errors.New("project must use id:<durable-id> or git:<host/repository>")
}

func (s Scope) applicableKeys() ([]string, error) {
	if _, err := s.Key(); err != nil {
		return nil, err
	}
	if s.Legacy {
		return nil, errors.New("legacy records require explicit get review")
	}
	keys := make([]string, 0, 8)
	seen := make(map[string]bool)
	for mask := 0; mask < 8; mask++ {
		next := Scope{}
		if mask&1 != 0 {
			next.Project = s.Project
		}
		if mask&2 != 0 {
			next.Device = s.Device
		}
		if mask&4 != 0 {
			next.Platform = s.Platform
		}
		key, err := next.Key()
		if err != nil {
			return nil, err
		}
		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	return keys, nil
}

// BrowseScope is the owner's view filter. AllProjects widens the view to every
// project's memories plus global ones; without it a nil project means only
// global scope. It never changes what agents retrieve.
type BrowseScope struct {
	Scope
	AllProjects bool `json:"all_projects"`
	// View selects what the owner lists: active memories (empty), those
	// awaiting review, or archived ones.
	View    string `json:"view"`
	Purpose string `json:"purpose"`
}

// Owner list views. Review omits handoffs: they load at startup unconfirmed
// by design, so they never wait on the owner's decision.
const (
	ViewReview   = "review"
	ViewArchived = "archived"
)

// Provenance records the harness, device, and source supplied with a write.
type Provenance struct {
	Harness string `json:"harness"`
	Device  string `json:"device"`
	Source  string `json:"source"`
}

// Record is a memory at its current or requested historical revision.
type Record struct {
	ID         int64      `json:"id"`
	Revision   int64      `json:"revision"`
	Scope      Scope      `json:"scope"`
	Content    string     `json:"content"`
	Title      string     `json:"title"`
	Tags       string     `json:"tags"`
	MemoryType string     `json:"memory_type"`
	Purpose    string     `json:"purpose"`
	Confirmed  bool       `json:"confirmed"`
	Provenance Provenance `json:"provenance"`
	Key        *string    `json:"key"`
	Archived   bool       `json:"archived"`
	CreatedAt  string     `json:"created_at"`
	UpdatedAt  string     `json:"updated_at"`
	// ContentTruncated marks an owner-list preview. Only browse lists set it;
	// Get and agent context always carry complete text.
	ContentTruncated bool `json:"content_truncated,omitempty"`
}

// Reference identifies an omitted record without repeating its content.
type Reference struct {
	ID       int64 `json:"id"`
	Revision int64 `json:"revision"`
	Scope    Scope `json:"scope"`
}

// Page contains bounded records and omission metadata. Omitted counts
// eligible records left out by budget, limit, or handoff selection;
// OmittedIDs and OmittedRecords each list at most 100.
type Page struct {
	Records        []Record    `json:"records"`
	Omitted        int         `json:"omitted"`
	OmittedIDs     []int64     `json:"omitted_ids"`
	OmittedRecords []Reference `json:"omitted_records"`
}

// Reader provides scoped read operations over a SQLite database.
type Reader struct{ db *sql.DB }

// OpenReadOnly opens an existing regular database file without migration.
func OpenReadOnly(path string) (*Reader, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("database is not a regular file")
	}
	// Escape path punctuation before adding SQLite URI options. SQLite mode=ro
	// prevents accidental migrations and writes to the opened database.
	db, err := sql.Open("sqlite", fileURI(abs, "mode=ro"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return &Reader{db: db}, nil
}

func fileURI(abs, query string) string {
	uriPath := filepath.ToSlash(abs)
	if runtime.GOOS == "windows" {
		uriPath = "/" + uriPath
	}
	uri := url.URL{Scheme: "file", Path: uriPath, RawQuery: query}
	return uri.String()
}

// Close releases the reader's database connection.
func (r *Reader) Close() error { return r.db.Close() }

const recordColumns = `id, revision, memory_scope, content, title, descriptors,
 memory_type, purpose, confirmed, provenance, preference_key, archived, created_at, updated_at`

type scanner interface{ Scan(...any) error }

func scanRecord(row scanner) (Record, error) {
	var record Record
	var scopeJSON, provenanceJSON string
	var key sql.NullString
	var confirmed, archived int64
	err := row.Scan(&record.ID, &record.Revision, &scopeJSON, &record.Content, &record.Title,
		&record.Tags, &record.MemoryType, &record.Purpose, &confirmed, &provenanceJSON,
		&key, &archived, &record.CreatedAt, &record.UpdatedAt)
	if err != nil {
		return Record{}, err
	}
	if scopeJSON == "legacy" {
		record.Scope.Legacy = true
	} else if err := json.Unmarshal([]byte(scopeJSON), &record.Scope); err != nil {
		return Record{}, fmt.Errorf("decode memory scope: %w", err)
	}
	if err := json.Unmarshal([]byte(provenanceJSON), &record.Provenance); err != nil {
		return Record{}, fmt.Errorf("decode provenance: %w", err)
	}
	if key.Valid {
		record.Key = &key.String
	}
	record.Confirmed = confirmed != 0
	record.Archived = archived != 0
	return record, nil
}

// Get returns the current or requested revision when its scope applies to
// the caller. Missing and out-of-scope records return nil; legacy records
// require an explicit Legacy scope.
func (r *Reader) Get(ctx context.Context, scope Scope, id int64, revision *int64) (*Record, error) {
	if scope.Legacy {
		if _, err := scope.Key(); err != nil {
			return nil, err
		}
		tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		return getInTx(ctx, tx, scope, id, revision)
	}
	keys, err := scope.applicableKeys()
	if err != nil {
		return nil, err
	}
	return r.getKeys(ctx, keys, id, revision)
}

func (r *Reader) getKeys(ctx context.Context, keys []string, id int64, revision *int64) (*Record, error) {
	keyJSON, err := json.Marshal(keys)
	if err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var scopeJSON string
	err = tx.QueryRowContext(ctx, "SELECT memory_scope FROM chunks WHERE id=? AND kind='memory' AND memory_scope IN (SELECT value FROM json_each(?))", id, string(keyJSON)).Scan(&scopeJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var exact Scope
	if err := json.Unmarshal([]byte(scopeJSON), &exact); err != nil {
		return nil, err
	}
	return getInTx(ctx, tx, exact, id, revision)
}

// Context returns applicable confirmed records and at most one handoff,
// preferring a project handoff. Records excluded by budget appear in
// omission metadata.
func (r *Reader) Context(ctx context.Context, scope Scope, budget int) (Page, error) {
	keys, err := scope.applicableKeys()
	if err != nil {
		return Page{}, err
	}
	return r.contextKeys(ctx, keys, budget)
}

// BrowseContext is for the authenticated read-only visualizer. A missing
// device or platform means all of them; a missing project still means only
// global project scope. Agent context keeps its narrower applicable-scope rule.
func (r *Reader) BrowseContext(ctx context.Context, scope BrowseScope, budget int) (Page, []string, []string, error) {
	keys, devices, projects, err := r.browseScopeKeys(ctx, scope, scope.View == ViewArchived)
	if err != nil {
		return Page{}, nil, nil, err
	}
	page, err := r.browseKeys(ctx, keys, budget, scope.View, scope.Purpose)
	return page, devices, projects, err
}

// BrowseGet uses the owner's device/platform browsing semantics for a full
// current or historical record. Agent Get retains its narrower scope rule.
func (r *Reader) BrowseGet(ctx context.Context, scope BrowseScope, id int64, revision *int64) (*Record, error) {
	if _, err := scope.Key(); err != nil || scope.Legacy {
		return nil, errors.New("invalid visualizer scope")
	}
	return r.ownerRecord(ctx, id, revision, func(exact Scope) bool {
		return !(exact.Project != nil && !scope.AllProjects && (scope.Project == nil || *scope.Project != *exact.Project)) &&
			!(scope.Device != nil && exact.Device != nil && *scope.Device != *exact.Device) &&
			!(scope.Platform != nil && exact.Platform != nil && *scope.Platform != *exact.Platform)
	})
}

// RecordByID returns a record by ID alone, with the exact scope it is stored
// under. Owner actions use it to act on that scope rather than a client's
// claim. Missing and legacy records return nil.
func (r *Reader) RecordByID(ctx context.Context, id int64, revision *int64) (*Record, error) {
	return r.ownerRecord(ctx, id, revision, func(Scope) bool { return true })
}

func (r *Reader) ownerRecord(ctx context.Context, id int64, revision *int64, visible func(Scope) bool) (*Record, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var scopeJSON string
	err = tx.QueryRowContext(ctx, "SELECT memory_scope FROM chunks WHERE id=? AND kind='memory'", id).Scan(&scopeJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if scopeJSON == "legacy" {
		return nil, nil
	}
	var exact Scope
	if err := json.Unmarshal([]byte(scopeJSON), &exact); err != nil {
		return nil, err
	}
	if exact.Legacy || !visible(exact) {
		return nil, nil
	}
	return getInTx(ctx, tx, exact, id, revision)
}

// BrowseCounts reports how many memories await the owner's review and how many
// are archived within the owner's project, device and platform filter.
func (r *Reader) BrowseCounts(ctx context.Context, scope BrowseScope) (review, archived int, err error) {
	keys, _, _, err := r.browseScopeKeys(ctx, scope, true)
	if err != nil {
		return 0, 0, err
	}
	keyJSON, err := json.Marshal(keys)
	if err != nil {
		return 0, 0, err
	}
	err = r.db.QueryRowContext(ctx, `SELECT
 COALESCE(SUM(archived=0 AND confirmed=0 AND purpose<>'handoff'),0),
 COALESCE(SUM(archived=1),0)
 FROM chunks WHERE kind='memory' AND memory_scope IN (SELECT value FROM json_each(?))`+ownerPurposeFilter(scope.Purpose, ""), string(keyJSON)).Scan(&review, &archived)
	return review, archived, err
}

// browseScopeKeys expands the owner's filter into stored scope keys. Choices
// normally come from active memories; includeArchived also counts archived
// ones so their scopes can be listed and counted.
func (r *Reader) browseScopeKeys(ctx context.Context, scope BrowseScope, includeArchived bool) ([]string, []string, []string, error) {
	if _, err := scope.Key(); err != nil || scope.Legacy {
		return nil, nil, nil, errors.New("invalid visualizer scope")
	}
	if scope.View != "" && scope.View != ViewReview && scope.View != ViewArchived {
		return nil, nil, nil, errors.New("invalid visualizer view")
	}
	if !validOwnerPurpose(scope.Purpose) {
		return nil, nil, nil, errors.New("invalid purpose")
	}
	devices, err := r.browseDevices(ctx, scope, includeArchived)
	if err != nil {
		return nil, nil, nil, err
	}
	knownProjects, err := r.browseProjects(ctx, includeArchived)
	if err != nil {
		return nil, nil, nil, err
	}
	// Each dimension is independent: a record shows when its project, device
	// and platform are each unset or selected. Reading the scopes that exist
	// keeps the cost proportional to stored scopes, not to their combinations.
	rows, err := r.db.QueryContext(ctx, "SELECT DISTINCT memory_scope FROM chunks WHERE kind='memory' AND memory_scope<>'legacy'")
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()
	keys := []string{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, nil, nil, err
		}
		var stored Scope
		if json.Unmarshal([]byte(key), &stored) != nil || stored.Legacy {
			continue
		}
		if (stored.Project == nil || scope.AllProjects || (scope.Project != nil && *scope.Project == *stored.Project)) &&
			(stored.Device == nil || scope.Device == nil || *scope.Device == *stored.Device) &&
			(stored.Platform == nil || scope.Platform == nil || *scope.Platform == *stored.Platform) {
			keys = append(keys, key)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, err
	}
	return keys, devices, knownProjects, nil
}

// Titles returns current titles for memory IDs so the owner's view can name
// records that a size-limited list omitted. Unknown IDs are absent.
func (r *Reader) Titles(ctx context.Context, ids []int64) (map[int64]string, error) {
	titles := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return titles, nil
	}
	idJSON, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, "SELECT id, title FROM chunks WHERE kind='memory' AND id IN (SELECT value FROM json_each(?))", string(idJSON))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var title string
		if err := rows.Scan(&id, &title); err != nil {
			return nil, err
		}
		titles[id] = title
	}
	return titles, rows.Err()
}

// Totals counts every active and archived memory, regardless of scope.
// Quarantined legacy records are not counted.
func (r *Reader) Totals(ctx context.Context) (active, archived int, err error) {
	err = r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(archived=0),0), COALESCE(SUM(archived=1),0)
 FROM chunks WHERE kind='memory' AND memory_scope<>'legacy'`).Scan(&active, &archived)
	return active, archived, err
}

func (r *Reader) browseProjects(ctx context.Context, includeArchived bool) ([]string, error) {
	const document = "CASE WHEN json_valid(memory_scope) THEN memory_scope ELSE '{}' END"
	const project = "json_extract(" + document + ", '$.project')"
	query := "SELECT DISTINCT " + project + " FROM chunks WHERE kind='memory' AND (? OR archived=0)" +
		" AND " + project + " IS NOT NULL ORDER BY 1"
	rows, err := r.db.QueryContext(ctx, query, includeArchived)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	projects := []string{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		projects = append(projects, value)
	}
	return projects, rows.Err()
}

func (r *Reader) browseDevices(ctx context.Context, scope BrowseScope, includeArchived bool) ([]string, error) {
	// Legacy rows are not JSON. CASE keeps JSON extraction safe even if the
	// query planner reorders predicates; project filtering happens in SQLite.
	const document = "CASE WHEN json_valid(memory_scope) THEN memory_scope ELSE '{}' END"
	const device = "json_extract(" + document + ", '$.device')"
	const project = "json_extract(" + document + ", '$.project')"
	const platform = "json_extract(" + document + ", '$.platform')"
	query := "SELECT DISTINCT " + device + " FROM chunks WHERE kind='memory' AND (? OR archived=0)" +
		" AND " + device + " IS NOT NULL" +
		" AND (? OR " + project + " IS NULL OR " + project + "=?)" +
		" AND (? IS NULL OR " + platform + " IS NULL OR " + platform + "=?) ORDER BY 1"
	rows, err := r.db.QueryContext(ctx, query, includeArchived, scope.AllProjects, scope.Project, scope.Platform, scope.Platform)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	devices := []string{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		devices = append(devices, value)
	}
	return devices, rows.Err()
}

func (r *Reader) contextKeys(ctx context.Context, keys []string, budget int) (Page, error) {
	records, err := r.scopedRecords(ctx, keys, agentContextFilter, false)
	if err != nil {
		return Page{}, err
	}
	selected, skipped := chooseHandoff(records)
	page, err := boundRecords(selected, budget, 0)
	if err != nil {
		return Page{}, err
	}
	for _, record := range skipped {
		page.Omitted++
		if len(page.OmittedIDs) < 100 {
			page.OmittedIDs = append(page.OmittedIDs, record.ID)
			page.OmittedRecords = append(page.OmittedRecords, Reference{record.ID, record.Revision, record.Scope})
		}
	}
	return page, nil
}

// Agent startup context loads confirmed records and handoffs only.
const agentContextFilter = " AND (confirmed=1 OR purpose='handoff')"

// chooseHandoff keeps every non-handoff record and one handoff, preferring a
// project handoff; the rest are skipped. The choice is shared by agent context
// and the owner's startup preview so the two cannot drift apart.
func chooseHandoff(records []Record) (selected, skipped []Record) {
	chosen := -1
	for i, record := range records {
		if record.Purpose != "handoff" {
			continue
		}
		if chosen < 0 || (records[chosen].Scope.Project == nil && record.Scope.Project != nil) {
			chosen = i
		}
	}
	selected = make([]Record, 0, len(records))
	for i, record := range records {
		if record.Purpose == "handoff" && i != chosen {
			skipped = append(skipped, record)
			continue
		}
		selected = append(selected, record)
	}
	return selected, skipped
}

// Reasons the owner's startup preview gives for a memory an agent will not load.
const (
	NotLoadedUnconfirmed  = "unconfirmed"
	NotLoadedOlderHandoff = "older_handoff"
	NotLoadedOverBudget   = "over_budget"
)

// NotLoaded names a memory in scope that startup context leaves out, and why.
type NotLoaded struct {
	ID       int64  `json:"id"`
	Revision int64  `json:"revision"`
	Scope    Scope  `json:"scope"`
	Title    string `json:"title"`
	Purpose  string `json:"purpose"`
	Reason   string `json:"reason"`
}

// Startup is what agent startup context would hold for one scope and budget.
// It shows what the service would send; it cannot show what a running agent
// session received.
type Startup struct {
	Budget         int         `json:"budget"`
	Records        []Record    `json:"records"`
	NotLoaded      []NotLoaded `json:"not_loaded"`
	NotLoadedTotal int         `json:"not_loaded_total"`
}

// StartupPreview applies the same selection as Context, with previews in place
// of full text. It reports the exact exclusion count and at most 100 details.
func (r *Reader) StartupPreview(ctx context.Context, scope Scope, budget int) (Startup, error) {
	keys, err := scope.applicableKeys()
	if err != nil {
		return Startup{}, err
	}
	eligible, err := r.scopedRecords(ctx, keys, agentContextFilter, false)
	if err != nil {
		return Startup{}, err
	}
	selected, skipped := chooseHandoff(eligible)
	page, err := boundRecords(selected, budget, 0)
	if err != nil {
		return Startup{}, err
	}
	// Recompute exclusions to report an exact total independently of the bounded
	// detail list returned to the browser.
	loaded := make(map[int64]bool, len(page.Records))
	for _, record := range page.Records {
		loaded[record.ID] = true
	}
	preview := Startup{Budget: budget, Records: previewRecords(page.Records), NotLoaded: []NotLoaded{}}
	add := func(record Record, reason string) {
		preview.NotLoadedTotal++
		if len(preview.NotLoaded) < 100 {
			preview.NotLoaded = append(preview.NotLoaded, NotLoaded{record.ID, record.Revision, record.Scope, record.Title, record.Purpose, reason})
		}
	}
	for _, record := range selected {
		if !loaded[record.ID] {
			add(record, NotLoadedOverBudget)
		}
	}
	for _, record := range skipped {
		add(record, NotLoadedOlderHandoff)
	}
	unconfirmed, err := r.scopedRecords(ctx, keys, " AND confirmed=0 AND purpose<>'handoff'", false)
	if err != nil {
		return Startup{}, err
	}
	for _, record := range unconfirmed {
		add(record, NotLoadedUnconfirmed)
	}
	return preview, nil
}

func (r *Reader) browseKeys(ctx context.Context, keys []string, budget int, view string, purpose ...string) (Page, error) {
	filter, archived := "", false
	switch view {
	case ViewReview:
		filter = " AND confirmed=0 AND purpose<>'handoff'"
	case ViewArchived:
		archived = true
	}
	if len(purpose) > 0 {
		filter += ownerPurposeFilter(purpose[0], "")
	}
	records, err := r.scopedRecords(ctx, keys, filter, archived)
	if err != nil {
		return Page{}, err
	}
	return boundRecords(previewRecords(records), budget, 0)
}

// browsePreviewRunes is how much of a memory the owner's list carries. A list
// of previews shows every memory; the full text opens on demand through Get.
const browsePreviewRunes = 240

func previewRecords(records []Record) []Record {
	for i, record := range records {
		if runes := []rune(record.Content); len(runes) > browsePreviewRunes {
			records[i].Content = string(runes[:browsePreviewRunes])
			records[i].ContentTruncated = true
		}
	}
	return records
}

// scopedRecords lists records in the given scope keys, active or archived,
// narrowed by an extra SQL condition that never contains caller input.
func (r *Reader) scopedRecords(ctx context.Context, keys []string, filter string, archived bool) ([]Record, error) {
	keyJSON, err := json.Marshal(keys)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, "SELECT "+recordColumns+` FROM chunks WHERE kind='memory' AND archived=?
 AND memory_scope IN (SELECT value FROM json_each(?))`+filter+`
 ORDER BY CASE purpose WHEN 'preference' THEN 0 WHEN 'decision' THEN 1 WHEN 'lesson' THEN 2 WHEN 'handoff' THEN 3 ELSE 4 END, updated_at DESC,id DESC`, archived, string(keyJSON))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []Record{}
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func boundRecords(records []Record, budget, limit int) (Page, error) {
	if budget < 1024 {
		budget = 1024
	} else if budget > 65536 {
		budget = 65536
	}
	page := Page{Records: []Record{}, OmittedIDs: []int64{}, OmittedRecords: []Reference{}}
	used := 0
	for _, record := range records {
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(record); err != nil {
			return Page{}, err
		}
		size := buf.Len() - 1 // Encoder adds a newline; Rust serde_json does not.
		if used+size > budget || (limit > 0 && len(page.Records) >= limit) {
			page.Omitted++
			if len(page.OmittedIDs) < 100 {
				page.OmittedIDs = append(page.OmittedIDs, record.ID)
				page.OmittedRecords = append(page.OmittedRecords, Reference{record.ID, record.Revision, record.Scope})
			}
			continue
		}
		used += size
		page.Records = append(page.Records, record)
	}
	return page, nil
}

// Candidates applies scope in SQLite before a caller ranks memory records.
// It returns active rows only and never changes the database.
func (r *Reader) Candidates(ctx context.Context, scope Scope) ([]Record, error) {
	keys, err := scope.applicableKeys()
	if err != nil {
		return nil, err
	}
	keyJSON, err := json.Marshal(keys)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, "SELECT "+recordColumns+` FROM chunks WHERE kind='memory' AND archived=0
 AND memory_scope IN (SELECT value FROM json_each(?)) ORDER BY id`, string(keyJSON))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []Record{}
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}
