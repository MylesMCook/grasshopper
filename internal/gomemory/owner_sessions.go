package gomemory

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"
)

// EnsureOwnerSessionSchema adds only browser authentication state. Session
// hashes, not reusable cookies, are stored. Backup snapshots include revocation.
func (w *Writer) EnsureOwnerSessionSchema(ctx context.Context) error {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS owner_sessions (session_hash BLOB PRIMARY KEY CHECK(length(session_hash)=32),expires_at INTEGER NOT NULL)`); err != nil {
		return err
	}
	var columns int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info('owner_sessions') WHERE name IN ('session_hash','expires_at')`).Scan(&columns); err != nil || columns != 2 {
		return errors.New("owner session schema is incompatible")
	}
	return tx.Commit()
}

// AddOwnerSession removes expired sessions and caps persistent state at 256
// browsers. A full table fails sign-in explicitly rather than evicting sessions.
func (w *Writer) AddOwnerSession(ctx context.Context, value string, expires time.Time) error {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM owner_sessions WHERE expires_at<=?`, time.Now().Unix()); err != nil {
		return err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM owner_sessions`).Scan(&count); err != nil {
		return err
	}
	if count >= 256 {
		return errors.New("owner session capacity reached")
	}
	hash := sha256.Sum256([]byte(value))
	if _, err := tx.ExecContext(ctx, `INSERT INTO owner_sessions(session_hash,expires_at) VALUES(?,?)`, hash[:], expires.Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

// OwnerSessionValid rejects cookies absent from persistent state, including
// old stateless cookies, expired sessions, and copies of signed-out cookies.
func (r *Reader) OwnerSessionValid(ctx context.Context, value string, now time.Time) (bool, error) {
	hash := sha256.Sum256([]byte(value))
	var valid bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM owner_sessions WHERE session_hash=? AND expires_at>?)`, hash[:], now.Unix()).Scan(&valid)
	return valid, err
}

// RevokeOwnerSession removes one browser or, with everywhere, all browser
// sessions. Paired device tokens and the master bearer are independent.
func (w *Writer) RevokeOwnerSession(ctx context.Context, value string, everywhere bool) error {
	if everywhere {
		_, err := w.db.ExecContext(ctx, `DELETE FROM owner_sessions`)
		return err
	}
	hash := sha256.Sum256([]byte(value))
	_, err := w.db.ExecContext(ctx, `DELETE FROM owner_sessions WHERE session_hash=?`, hash[:])
	return err
}
