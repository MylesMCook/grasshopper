package gomemory

import (
	"database/sql"
	"os"
	"path/filepath"
)

// CreateEmpty creates a memory-only database and never replaces an existing file.
func CreateEmpty(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(abs, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	completed := false
	defer func() {
		if !completed {
			_ = os.Remove(abs)
		}
	}()
	db, err := sql.Open("sqlite", fileURI(abs, "mode=rw"))
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	statements := []string{
		`CREATE TABLE chunks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			kind TEXT NOT NULL CHECK(kind='memory'),
			content TEXT NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			descriptors TEXT NOT NULL DEFAULT '',
			memory_type TEXT NOT NULL,
			purpose TEXT NOT NULL,
			confirmed INTEGER NOT NULL,
			provenance TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			memory_scope TEXT NOT NULL,
			preference_key TEXT,
			agent_id TEXT NOT NULL DEFAULT '',
			archived INTEGER NOT NULL DEFAULT 0,
			embedding BLOB,
			embedding_model TEXT NOT NULL DEFAULT '',
			revision INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE VIRTUAL TABLE chunks_fts USING fts5(title, content, snippet, symbol_name, descriptors, tokenize='porter unicode61')`,
		`CREATE TABLE memory_requests (request_id TEXT PRIMARY KEY, fingerprint TEXT NOT NULL, receipt TEXT NOT NULL)`,
		`CREATE TABLE memory_revisions (memory_id INTEGER NOT NULL REFERENCES chunks(id), revision INTEGER NOT NULL, record TEXT NOT NULL, embedding BLOB, embedding_model TEXT NOT NULL, PRIMARY KEY(memory_id, revision))`,
		`CREATE TABLE client_tokens (id INTEGER PRIMARY KEY, device TEXT NOT NULL CHECK(length(device) BETWEEN 1 AND 64), token_hash BLOB NOT NULL UNIQUE CHECK(length(token_hash)=32), created_at TEXT NOT NULL, revoked_at TEXT)`,
		`CREATE UNIQUE INDEX idx_memory_exact ON chunks(memory_scope, memory_type, purpose, confirmed, content_hash) WHERE kind='memory' AND archived=0 AND memory_scope != 'legacy'`,
		`CREATE UNIQUE INDEX idx_memory_key ON chunks(memory_scope, preference_key) WHERE kind='memory' AND preference_key IS NOT NULL AND memory_scope != 'legacy'`,
		`CREATE INDEX idx_memory_scope ON chunks(memory_scope, archived) WHERE kind='memory'`,
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if err := db.Close(); err != nil {
		return err
	}
	completed = true
	return nil
}
