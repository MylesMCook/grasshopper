package gomemory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"
)

// WriteInput is the shared memory write contract.
type WriteInput struct {
	Scope            Scope      `json:"scope"`
	Content          string     `json:"content"`
	Title            *string    `json:"title"`
	Tags             *string    `json:"tags"`
	MemoryType       *string    `json:"memory_type"`
	Purpose          string     `json:"purpose"`
	Confirmed        bool       `json:"confirmed"`
	Provenance       Provenance `json:"provenance"`
	RequestID        string     `json:"request_id"`
	Key              *string    `json:"key"`
	ID               *int64     `json:"id"`
	ExpectedRevision *int64     `json:"expected_revision"`
	RestoreRevision  *int64     `json:"restore_revision"`
}

type ArchiveInput struct {
	Scope            Scope      `json:"scope"`
	ID               int64      `json:"id"`
	ExpectedRevision int64      `json:"expected_revision"`
	Archived         bool       `json:"archived"`
	RequestID        string     `json:"request_id"`
	Provenance       Provenance `json:"provenance"`
}

type Receipt struct {
	ID           int64 `json:"id"`
	Revision     int64 `json:"revision"`
	Deduplicated bool  `json:"deduplicated"`
}

// Writer is deliberately available only for a new, explicit database copy.
// It never opens its source for writing and performs no schema migration.
type Writer struct{ *Reader }

// OpenWritableExisting is for a previously verified Go shadow database after
// cutover. It never creates or migrates a database, and refuses memory rows
// whose vectors do not match the running model.
func OpenWritableExisting(path, model string, dimensions int) (*Writer, error) {
	if model == "" || dimensions < 1 || dimensions > 4096 {
		return nil, errors.New("valid model and dimensions required")
	}
	reader, err := openWritableFile(path)
	if err != nil {
		return nil, err
	}
	var incompatible int
	if err := reader.db.QueryRow(`SELECT count(*) FROM chunks WHERE kind='memory'
 AND (embedding IS NULL OR embedding_model<>? OR length(embedding)<>?)`, model, dimensions*4).Scan(&incompatible); err != nil {
		reader.Close()
		return nil, err
	}
	if incompatible != 0 {
		reader.Close()
		return nil, fmt.Errorf("%d memory rows need shadow re-embedding before Go can serve this database", incompatible)
	}
	var journalMode string
	if err := reader.db.QueryRow("PRAGMA journal_mode=WAL").Scan(&journalMode); err != nil || journalMode != "wal" {
		reader.Close()
		return nil, fmt.Errorf("could not enable WAL journal mode: %v", err)
	}
	return &Writer{reader}, nil
}

func OpenWritableCopy(ctx context.Context, source, destination string) (*Writer, error) {
	if _, err := os.Stat(destination); err == nil {
		return nil, errors.New("backup destination already exists")
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	// VACUUM INTO requires a nonexistent output. Build beside the destination,
	// then hard-link it into place without overwriting an existing backup.
	temporary, err := os.CreateTemp(filepath.Dir(destination), "."+filepath.Base(destination)+".tmp-*")
	if err != nil {
		return nil, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Close(); err != nil {
		return nil, err
	}
	if err := os.Remove(temporaryPath); err != nil {
		return nil, err
	}
	sourceReader, err := OpenReadOnly(source)
	if err != nil {
		return nil, err
	}
	defer sourceReader.Close()
	if _, err := sourceReader.db.ExecContext(ctx, "VACUUM INTO ?", temporaryPath); err != nil {
		return nil, err
	}
	if err := os.Chmod(temporaryPath, 0600); err != nil {
		return nil, err
	}
	if err := os.Link(temporaryPath, destination); err != nil {
		if os.IsExist(err) {
			return nil, errors.New("backup destination already exists")
		}
		return nil, err
	}
	reader, err := openWritableFile(destination)
	if err != nil {
		_ = os.Remove(destination)
		return nil, err
	}
	return &Writer{reader}, nil
}

func openWritableFile(path string) (*Reader, error) {
	// A missing schema is an error, not an invitation to create a second format.
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
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("database must not be readable or writable by other users")
	}
	uri := fileURI(abs, "mode=rw&_txlock=immediate")
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA busy_timeout=3000"); err != nil {
		db.Close()
		return nil, err
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM pragma_table_info('chunks') WHERE name='memory_scope'").Scan(&count); err != nil || count != 1 {
		db.Close()
		return nil, errors.New("shared-memory schema missing")
	}
	var check string
	if err := db.QueryRow("PRAGMA quick_check").Scan(&check); err != nil {
		db.Close()
		return nil, err
	}
	if check != "ok" {
		db.Close()
		return nil, fmt.Errorf("database quick_check failed: %s", check)
	}
	return &Reader{db}, nil
}

func validateProvenance(p Provenance) error {
	for _, value := range []string{p.Harness, p.Device, p.Source} {
		if strings.TrimSpace(value) == "" || len(value) > 2048 {
			return errors.New("provenance requires bounded harness, device, and source")
		}
	}
	return nil
}

func canonicalJSON(value any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte{'\n'}), nil
}

func digest(value any) (string, error) {
	data, err := canonicalJSON(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func checkRequestID(request string) error {
	if strings.TrimSpace(request) == "" || len(request) > 128 {
		return errors.New("request_id must be 1-128 bytes")
	}
	return nil
}

func replay(ctx context.Context, tx *sql.Tx, request, fingerprint string) (*Receipt, error) {
	var previous, raw string
	err := tx.QueryRowContext(ctx, "SELECT fingerprint,receipt FROM memory_requests WHERE request_id=?", request).Scan(&previous, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if previous != fingerprint {
		return nil, errors.New("idempotency_conflict: request_id was used with a different payload")
	}
	var receipt Receipt
	if err := json.Unmarshal([]byte(raw), &receipt); err != nil {
		return nil, err
	}
	return &receipt, nil
}

func acknowledge(ctx context.Context, tx *sql.Tx, request, fingerprint string, receipt Receipt) (Receipt, error) {
	raw, err := canonicalJSON(receipt)
	if err != nil {
		return Receipt{}, err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO memory_requests(request_id,fingerprint,receipt) VALUES(?,?,?)", request, fingerprint, string(raw))
	return receipt, err
}

func getInTx(ctx context.Context, tx *sql.Tx, scope Scope, id int64, revision *int64) (*Record, error) {
	key, err := scope.Key()
	if err != nil {
		return nil, err
	}
	current, err := scanRecord(tx.QueryRowContext(ctx, "SELECT "+recordColumns+" FROM chunks WHERE id=? AND kind='memory' AND memory_scope=?", id, key))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if revision == nil || *revision == current.Revision {
		return &current, nil
	}
	var raw string
	err = tx.QueryRowContext(ctx, "SELECT record FROM memory_revisions WHERE memory_id=? AND revision=?", id, *revision).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var old Record
	if err := json.Unmarshal([]byte(raw), &old); err != nil {
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

func snapshot(ctx context.Context, tx *sql.Tx, record Record) error {
	raw, err := canonicalJSON(record)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO memory_revisions(memory_id,revision,record,embedding,embedding_model)
 SELECT id,revision,?,embedding,embedding_model FROM chunks WHERE id=?`, string(raw), record.ID)
	return err
}

func commitOrRollback(tx *sql.Tx, err *error) {
	if *err != nil {
		_ = tx.Rollback()
		return
	}
	*err = tx.Commit()
}

func (w *Writer) Write(ctx context.Context, input WriteInput, vector []float32, model string) (receipt Receipt, err error) {
	scopeKey, err := input.Scope.Key()
	if err != nil {
		return receipt, err
	}
	if input.ID == nil && input.ExpectedRevision == nil {
		if err := input.Scope.durableProject(); err != nil {
			return receipt, err
		}
	}
	if input.Scope.Legacy {
		return receipt, errors.New("new writes cannot target legacy scope")
	}
	if err = validateProvenance(input.Provenance); err != nil {
		return receipt, err
	}
	if err = checkRequestID(input.RequestID); err != nil {
		return receipt, err
	}
	switch input.Purpose {
	case "preference", "decision", "lesson", "handoff", "observation":
	default:
		return receipt, errors.New("invalid purpose")
	}
	if input.Key != nil && (strings.TrimSpace(*input.Key) == "" || len(*input.Key) > 128) {
		return receipt, errors.New("invalid preference key")
	}
	if vector != nil {
		if len(vector) == 0 || model == "" {
			return receipt, errors.New("invalid embedding")
		}
		for _, v := range vector {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return receipt, errors.New("invalid embedding")
			}
		}
	}
	fingerprint, err := digest(input)
	if err != nil {
		return receipt, err
	}
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return receipt, err
	}
	defer commitOrRollback(tx, &err)
	if previous, replayErr := replay(ctx, tx, input.RequestID, fingerprint); replayErr != nil {
		return receipt, replayErr
	} else if previous != nil {
		return *previous, nil
	}
	id := input.ID
	if id == nil && input.Key != nil {
		var found int64
		queryErr := tx.QueryRowContext(ctx, "SELECT id FROM chunks WHERE kind='memory' AND memory_scope=? AND preference_key=?", scopeKey, *input.Key).Scan(&found)
		if queryErr == nil {
			id = &found
		} else if !errors.Is(queryErr, sql.ErrNoRows) {
			return receipt, queryErr
		}
	}
	var current *Record
	if id != nil {
		current, err = getInTx(ctx, tx, input.Scope, *id, nil)
		if err != nil {
			return receipt, err
		}
		if current == nil {
			return receipt, errors.New("memory_not_found")
		}
	}
	if current != nil {
		if input.ExpectedRevision == nil || *input.ExpectedRevision != current.Revision {
			return receipt, fmt.Errorf("revision_conflict: current revision %d", current.Revision)
		}
		if current.Confirmed && !input.Confirmed {
			return receipt, errors.New("confirmation_conflict: observation cannot replace confirmed content")
		}
		if input.Key != nil && (current.Key == nil || *input.Key != *current.Key) {
			return receipt, errors.New("preference key is immutable")
		}
	} else if input.ExpectedRevision != nil || input.RestoreRevision != nil {
		return receipt, errors.New("revision_conflict: no existing record")
	}
	var restored *Record
	if input.RestoreRevision != nil {
		if input.Content != "" {
			return receipt, errors.New("restore requires empty content")
		}
		restored, err = getInTx(ctx, tx, input.Scope, *id, input.RestoreRevision)
		if err != nil {
			return receipt, err
		}
		if restored == nil {
			return receipt, errors.New("revision_not_found")
		}
	}
	content := input.Content
	if restored != nil {
		content = restored.Content
	}
	if strings.TrimSpace(content) == "" || len(content) > 32768 {
		return receipt, errors.New("content must be 1-32768 bytes")
	}
	title := firstRunes(content, 80)
	if input.Title != nil {
		title = *input.Title
	}
	if restored != nil {
		title = restored.Title
	}
	tags := ""
	if input.Tags != nil {
		tags = *input.Tags
	}
	if restored != nil {
		tags = restored.Tags
	}
	if len(title) > 512 || len(tags) > 2048 {
		return receipt, errors.New("metadata too large")
	}
	memoryType := "knowledge"
	if input.MemoryType != nil {
		memoryType = *input.MemoryType
	}
	if restored != nil {
		memoryType = restored.MemoryType
	}
	switch memoryType {
	case "identity", "knowledge", "episode", "procedure":
	default:
		return receipt, errors.New("invalid memory_type")
	}
	purpose := input.Purpose
	if restored != nil {
		purpose = restored.Purpose
	}
	contentHash, err := digest([]string{content, title, tags})
	if err != nil {
		return receipt, err
	}
	if current == nil {
		var exactID, exactRevision int64
		var exactKey sql.NullString
		queryErr := tx.QueryRowContext(ctx, `SELECT id,revision,preference_key FROM chunks WHERE kind='memory' AND archived=0 AND memory_scope=? AND memory_type=? AND purpose=? AND confirmed=? AND content_hash=?`, scopeKey, memoryType, purpose, input.Confirmed, contentHash).Scan(&exactID, &exactRevision, &exactKey)
		if queryErr == nil {
			if (input.Key == nil) != (!exactKey.Valid) || (input.Key != nil && *input.Key != exactKey.String) {
				return receipt, errors.New("dedup_key_conflict: exact content already has a different key")
			}
			return acknowledge(ctx, tx, input.RequestID, fingerprint, Receipt{exactID, exactRevision, true})
		}
		if !errors.Is(queryErr, sql.ErrNoRows) {
			return receipt, queryErr
		}
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.999999-07:00")
	provenanceJSON, err := canonicalJSON(input.Provenance)
	if err != nil {
		return receipt, err
	}
	var revision int64
	if current != nil {
		if err = snapshot(ctx, tx, *current); err != nil {
			return receipt, err
		}
		result, execErr := tx.ExecContext(ctx, `UPDATE chunks SET content=?,title=?,descriptors=?,memory_type=?,purpose=?,confirmed=?,provenance=?,content_hash=?,revision=revision+1,updated_at=?,archived=0,embedding=NULL,embedding_model='' WHERE id=? AND revision=?`, content, title, tags, memoryType, purpose, input.Confirmed, string(provenanceJSON), contentHash, now, current.ID, current.Revision)
		if execErr != nil {
			return receipt, execErr
		}
		changed, _ := result.RowsAffected()
		if changed != 1 {
			return receipt, errors.New("revision_conflict: concurrent update")
		}
		id = &current.ID
		revision = current.Revision + 1
	} else {
		result, execErr := tx.ExecContext(ctx, `INSERT INTO chunks(kind,content,title,descriptors,memory_type,purpose,confirmed,provenance,content_hash,created_at,updated_at,memory_scope,preference_key,agent_id)
 VALUES('memory',?,?,?,?,?,?,?,?,?,?,?,?,?)`, content, title, tags, memoryType, purpose, input.Confirmed, string(provenanceJSON), contentHash, now, now, scopeKey, input.Key, input.Provenance.Harness)
		if execErr != nil {
			return receipt, execErr
		}
		newID, idErr := result.LastInsertId()
		if idErr != nil {
			return receipt, idErr
		}
		id = &newID
		revision = 1
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM chunks_fts WHERE rowid=?", *id); err != nil {
		return receipt, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO chunks_fts(rowid,title,content,snippet,symbol_name,descriptors) VALUES(?,?,?,'','',?)", *id, title, content, tags); err != nil {
		return receipt, err
	}
	if vector != nil {
		blob := vectorBlob(vector)
		_, err = tx.ExecContext(ctx, "UPDATE chunks SET embedding=?,embedding_model=? WHERE id=?", blob, model, *id)
		if err != nil {
			return receipt, err
		}
	} else if input.RestoreRevision != nil {
		_, err = tx.ExecContext(ctx, `UPDATE chunks SET embedding=(SELECT embedding FROM memory_revisions WHERE memory_id=? AND revision=?),embedding_model=COALESCE((SELECT embedding_model FROM memory_revisions WHERE memory_id=? AND revision=?),'') WHERE id=?`, *id, *input.RestoreRevision, *id, *input.RestoreRevision, *id)
		if err != nil {
			return receipt, err
		}
	}
	newRecord, err := getInTx(ctx, tx, input.Scope, *id, nil)
	if err != nil {
		return receipt, err
	}
	if newRecord == nil {
		return receipt, errors.New("memory_not_found")
	}
	if err = snapshot(ctx, tx, *newRecord); err != nil {
		return receipt, err
	}
	return acknowledge(ctx, tx, input.RequestID, fingerprint, Receipt{*id, revision, false})
}

func vectorBlob(vector []float32) []byte {
	blob := make([]byte, len(vector)*4)
	for i, value := range vector {
		binary.LittleEndian.PutUint32(blob[4*i:], math.Float32bits(value))
	}
	return blob
}

func firstRunes(value string, count int) string {
	if utf8.RuneCountInString(value) <= count {
		return value
	}
	for i := range value {
		if count == 0 {
			return value[:i]
		}
		count--
	}
	return value
}

func (w *Writer) Archive(ctx context.Context, input ArchiveInput) (receipt Receipt, err error) {
	if _, err = input.Scope.Key(); err != nil {
		return receipt, err
	}
	if err = validateProvenance(input.Provenance); err != nil {
		return receipt, err
	}
	if err = checkRequestID(input.RequestID); err != nil {
		return receipt, err
	}
	fingerprint, err := digest(input)
	if err != nil {
		return receipt, err
	}
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return receipt, err
	}
	defer commitOrRollback(tx, &err)
	if previous, replayErr := replay(ctx, tx, input.RequestID, fingerprint); replayErr != nil {
		return receipt, replayErr
	} else if previous != nil {
		return *previous, nil
	}
	old, err := getInTx(ctx, tx, input.Scope, input.ID, nil)
	if err != nil {
		return receipt, err
	}
	if old == nil {
		return receipt, errors.New("memory_not_found")
	}
	if old.Revision != input.ExpectedRevision {
		return receipt, fmt.Errorf("revision_conflict: current revision %d", old.Revision)
	}
	if err = snapshot(ctx, tx, *old); err != nil {
		return receipt, err
	}
	provenanceJSON, err := canonicalJSON(input.Provenance)
	if err != nil {
		return receipt, err
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.999999-07:00")
	result, err := tx.ExecContext(ctx, "UPDATE chunks SET archived=?,revision=revision+1,updated_at=?,provenance=? WHERE id=? AND revision=?", input.Archived, now, string(provenanceJSON), input.ID, old.Revision)
	if err != nil {
		return receipt, err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return receipt, errors.New("revision_conflict: concurrent update")
	}
	current, err := getInTx(ctx, tx, input.Scope, input.ID, nil)
	if err != nil {
		return receipt, err
	}
	if current == nil {
		return receipt, errors.New("memory_not_found")
	}
	if err = snapshot(ctx, tx, *current); err != nil {
		return receipt, err
	}
	return acknowledge(ctx, tx, input.RequestID, fingerprint, Receipt{input.ID, old.Revision + 1, false})
}
