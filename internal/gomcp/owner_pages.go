package gomcp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/MylesMCook/grasshopper/internal/gomemory"
	"net/http"
	"sync"
	"time"
)

const ownerSnapshotLifetime = 10 * time.Minute
const ownerSnapshotMaxRecords = 50000
const ownerSnapshotMaxReferences = 100000

type ownerSnapshot struct {
	filter  string
	refs    []gomemory.Reference
	expires time.Time
}
type ownerPager struct {
	mu        sync.Mutex
	snapshots map[string]ownerSnapshot
}
type ownerListInput struct {
	gomemory.BrowseScope
	Limit int    `json:"limit"`
	After string `json:"after"`
}
type ownerCursor struct {
	Snapshot string `json:"s"`
	Offset   int    `json:"o"`
}

// Only ordered IDs and revision numbers are cached. Historical SQLite revisions
// remain the source of content; restart/expiry/eviction requires a fresh list.
func (pager *ownerPager) page(ctx context.Context, store *gomemory.Writer, input ownerListInput) (gomemory.Page, []string, []string, string, int, error) {
	if input.Limit == 0 {
		input.Limit = 50
	}
	if input.Limit < 1 || input.Limit > 100 {
		return gomemory.Page{}, nil, nil, "", 0, errors.New("invalid page limit")
	}
	filter, _ := json.Marshal(struct {
		Scope gomemory.BrowseScope
		Limit int
	}{input.BrowseScope, input.Limit})
	var refs []gomemory.Reference
	var devices, projects []string
	var err error
	if input.After == "" {
		refs, devices, projects, err = store.BrowseReferences(ctx, input.BrowseScope, ownerSnapshotMaxRecords)
	} else {
		devices, projects, err = store.BrowseChoices(ctx, input.BrowseScope)
	}
	if err != nil {
		return gomemory.Page{}, nil, nil, "", 0, err
	}
	now := time.Now()
	cursor := ownerCursor{}
	pager.mu.Lock()
	for key, snapshot := range pager.snapshots {
		if !snapshot.expires.After(now) {
			delete(pager.snapshots, key)
		}
	}
	if input.After != "" {
		raw, err := base64.RawURLEncoding.DecodeString(input.After)
		if err != nil || json.Unmarshal(raw, &cursor) != nil {
			pager.mu.Unlock()
			return gomemory.Page{}, nil, nil, "", 0, errors.New("invalid page cursor")
		}
		snapshot, ok := pager.snapshots[cursor.Snapshot]
		if !ok {
			pager.mu.Unlock()
			return gomemory.Page{}, nil, nil, "", 0, errors.New("page cursor expired; reload the list")
		}
		if snapshot.filter != string(filter) || cursor.Offset < 1 || cursor.Offset >= len(snapshot.refs) {
			pager.mu.Unlock()
			return gomemory.Page{}, nil, nil, "", 0, errors.New("page cursor does not match filters or limit")
		}
		refs = snapshot.refs
	} else {
		raw, _ := json.Marshal(refs)
		digest := sha256.Sum256(append(filter, raw...))
		cursor.Snapshot = hex.EncodeToString(digest[:])
		if _, ok := pager.snapshots[cursor.Snapshot]; !ok {
			total := len(refs)
			for _, snapshot := range pager.snapshots {
				total += len(snapshot.refs)
			}
			for len(pager.snapshots) >= 64 || total > ownerSnapshotMaxReferences {
				oldestKey := ""
				var oldest time.Time
				for key, snapshot := range pager.snapshots {
					if oldestKey == "" || snapshot.expires.Before(oldest) {
						oldestKey, oldest = key, snapshot.expires
					}
				}
				if oldestKey == "" {
					break
				}
				total -= len(pager.snapshots[oldestKey].refs)
				delete(pager.snapshots, oldestKey)
			}
			pager.snapshots[cursor.Snapshot] = ownerSnapshot{string(filter), refs, now.Add(ownerSnapshotLifetime)}
		}
	}
	pager.mu.Unlock()
	page := gomemory.Page{Records: []gomemory.Record{}, OmittedIDs: []int64{}, OmittedRecords: []gomemory.Reference{}}
	used := 0
	offset := cursor.Offset
	for offset < len(refs) && len(page.Records) < input.Limit {
		ref := refs[offset]
		record, err := store.RecordByID(ctx, ref.ID, &ref.Revision)
		if err != nil || record == nil {
			return gomemory.Page{}, nil, nil, "", 0, errors.New("snapshot revision unavailable; reload the list")
		}
		if runes := []rune(record.Content); len(runes) > 240 {
			record.Content = string(runes[:240])
			record.ContentTruncated = true
		}
		raw, _ := json.Marshal(record)
		if used+len(raw) > 32768 {
			break
		}
		used += len(raw)
		page.Records = append(page.Records, *record)
		offset++
	}
	if offset == cursor.Offset && offset < len(refs) {
		return gomemory.Page{}, nil, nil, "", 0, errors.New("memory preview exceeds page budget")
	}
	next := ""
	if offset < len(refs) {
		raw, _ := json.Marshal(ownerCursor{cursor.Snapshot, offset})
		next = base64.RawURLEncoding.EncodeToString(raw)
	}
	// Pagination replaces omission references: subsequent memories are reachable
	// through next and total instead of a truncated secondary list.
	return page, devices, projects, next, len(refs), nil
}

// writeOwnerJSON hashes deterministic response bytes. Authentication and input
// validation have already run before any conditional response can return 304.
func writeOwnerJSON(w http.ResponseWriter, r *http.Request, store *gomemory.Writer, value any) {
	raw, err := json.Marshal(value)
	if err != nil {
		http.Error(w, "response unavailable", 503)
		return
	}
	version, err := store.OwnerMemoryVersion(r.Context())
	if err != nil {
		http.Error(w, "response unavailable", 503)
		return
	}
	hash := sha256.Sum256(append([]byte(version), raw...))
	etag := `"` + hex.EncodeToString(hash[:]) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = w.Write(append(raw, '\n'))
}

func visualizerExport(store *gomemory.Writer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Scope           gomemory.BrowseScope `json:"scope"`
			All             bool                 `json:"all"`
			IncludeArchived bool                 `json:"include_archived"`
		}
		if !decodeVisualizerRead(w, r, &input) {
			return
		}
		if input.All {
			input.Scope = gomemory.BrowseScope{AllProjects: true}
		}
		started := false
		err := store.WalkOwnerExport(r.Context(), input.Scope, input.IncludeArchived, func(record gomemory.Record) error {
			if !started {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.Header().Set("Content-Disposition", `attachment; filename="grasshopper-memories.json"`)
				if _, err := w.Write([]byte(`{"records":[`)); err != nil {
					return err
				}
				started = true
			} else {
				if _, err := w.Write([]byte(",")); err != nil {
					return err
				}
			}
			return json.NewEncoder(w).Encode(record)
		})
		if err != nil {
			if !started {
				http.Error(w, "export unavailable or invalid filters", http.StatusBadRequest)
			}
			return
		}
		if !started {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="grasshopper-memories.json"`)
			_, _ = w.Write([]byte(`{"records":[`))
		}
		_, _ = w.Write([]byte("]}\n"))
	}
}
