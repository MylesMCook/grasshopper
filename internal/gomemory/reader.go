// Package gomemory is the SQLite compatibility slice of the Go transition.
// Existing Grasshopper databases open read-only; writes require a new copy.
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

type Scope struct {
	Project  *string `json:"project"`
	Device   *string `json:"device"`
	Platform *string `json:"platform"`
	Legacy   bool    `json:"legacy"`
}

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

type Provenance struct {
	Harness string `json:"harness"`
	Device  string `json:"device"`
	Source  string `json:"source"`
}

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
}

type Reference struct {
	ID       int64 `json:"id"`
	Revision int64 `json:"revision"`
	Scope    Scope `json:"scope"`
}

type Page struct {
	Records        []Record    `json:"records"`
	Omitted        int         `json:"omitted"`
	OmittedIDs     []int64     `json:"omitted_ids"`
	OmittedRecords []Reference `json:"omitted_records"`
}

type Reader struct{ db *sql.DB }

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
	// prevents accidental migrations and writes to this copied database.
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

func (r *Reader) Get(ctx context.Context, scope Scope, id int64, revision *int64) (*Record, error) {
	key, err := scope.Key()
	if err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	current, err := scanRecord(tx.QueryRowContext(ctx,
		"SELECT "+recordColumns+" FROM chunks WHERE id=? AND kind='memory' AND memory_scope=?", id, key))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if revision == nil || *revision == current.Revision {
		return &current, nil
	}
	var snapshot string
	err = tx.QueryRowContext(ctx, "SELECT record FROM memory_revisions WHERE memory_id=? AND revision=?", id, *revision).Scan(&snapshot)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var old Record
	if err := json.Unmarshal([]byte(snapshot), &old); err != nil {
		return nil, err
	}
	oldKey, err := old.Scope.Key()
	if err != nil {
		return nil, err
	}
	if old.ID != id || old.Revision != *revision || oldKey != key {
		return nil, errors.New("revision scope mismatch")
	}
	return &old, nil
}

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
func (r *Reader) BrowseContext(ctx context.Context, scope Scope, budget int) (Page, []string, []string, error) {
	if _, err := scope.Key(); err != nil || scope.Legacy {
		return Page{}, nil, nil, errors.New("invalid visualizer scope")
	}
	devices, err := r.browseDevices(ctx, scope)
	if err != nil {
		return Page{}, nil, nil, err
	}
	knownProjects, err := r.browseProjects(ctx)
	if err != nil {
		return Page{}, nil, nil, err
	}
	projects := []*string{nil}
	if scope.Project != nil {
		projects = append(projects, scope.Project)
	}
	selectedDevices := []*string{nil}
	if scope.Device != nil {
		selectedDevices = append(selectedDevices, scope.Device)
	} else {
		for index := range devices {
			selectedDevices = append(selectedDevices, &devices[index])
		}
	}
	platforms := []*string{nil}
	if scope.Platform != nil {
		platforms = append(platforms, scope.Platform)
	} else {
		for _, value := range []string{"macos", "windows", "linux"} {
			platform := value
			platforms = append(platforms, &platform)
		}
	}
	keys := make([]string, 0, len(projects)*len(selectedDevices)*len(platforms))
	for _, project := range projects {
		for _, device := range selectedDevices {
			for _, platform := range platforms {
				key, err := (Scope{Project: project, Device: device, Platform: platform}).Key()
				if err != nil {
					return Page{}, nil, nil, err
				}
				keys = append(keys, key)
			}
		}
	}
	page, err := r.contextKeys(ctx, keys, budget)
	return page, devices, knownProjects, err
}

func (r *Reader) browseProjects(ctx context.Context) ([]string, error) {
	const document = "CASE WHEN json_valid(memory_scope) THEN memory_scope ELSE '{}' END"
	const project = "json_extract(" + document + ", '$.project')"
	query := "SELECT DISTINCT " + project + " FROM chunks WHERE kind='memory' AND archived=0" +
		" AND (confirmed=1 OR purpose='handoff') AND " + project + " IS NOT NULL ORDER BY 1"
	rows, err := r.db.QueryContext(ctx, query)
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

func (r *Reader) browseDevices(ctx context.Context, scope Scope) ([]string, error) {
	// Legacy rows are not JSON. CASE keeps JSON extraction safe even if the
	// query planner reorders predicates; project filtering happens in SQLite.
	const document = "CASE WHEN json_valid(memory_scope) THEN memory_scope ELSE '{}' END"
	const device = "json_extract(" + document + ", '$.device')"
	const project = "json_extract(" + document + ", '$.project')"
	const platform = "json_extract(" + document + ", '$.platform')"
	query := "SELECT DISTINCT " + device + " FROM chunks WHERE kind='memory' AND archived=0" +
		" AND (confirmed=1 OR purpose='handoff') AND " + device + " IS NOT NULL" +
		" AND (" + project + " IS NULL OR " + project + "=?)" +
		" AND (? IS NULL OR " + platform + " IS NULL OR " + platform + "=?) ORDER BY 1"
	rows, err := r.db.QueryContext(ctx, query, scope.Project, scope.Platform, scope.Platform)
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
	if budget < 1024 {
		budget = 1024
	} else if budget > 65536 {
		budget = 65536
	}
	keyJSON, err := json.Marshal(keys)
	if err != nil {
		return Page{}, err
	}
	rows, err := r.db.QueryContext(ctx, "SELECT "+recordColumns+` FROM chunks WHERE kind='memory' AND archived=0
 AND memory_scope IN (SELECT value FROM json_each(?)) AND (confirmed=1 OR purpose='handoff')
 ORDER BY CASE purpose WHEN 'preference' THEN 0 WHEN 'decision' THEN 1 WHEN 'lesson' THEN 2 WHEN 'handoff' THEN 3 ELSE 4 END, updated_at DESC,id DESC`, string(keyJSON))
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	page := Page{Records: []Record{}, OmittedIDs: []int64{}, OmittedRecords: []Reference{}}
	handoffSeen := false
	records := []Record{}
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return Page{}, err
		}
		if record.Purpose == "handoff" {
			if handoffSeen {
				continue
			}
			handoffSeen = true
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	page, err = boundRecords(records, budget, 0)
	return page, err
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
