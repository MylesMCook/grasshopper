package memory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode"
)

// ClientToken contains metadata only. Raw tokens are never stored in SQLite.
type ClientToken struct {
	ID        int64   `json:"id"`
	Device    string  `json:"device"`
	CreatedAt string  `json:"created_at"`
	RevokedAt *string `json:"revoked_at"`
}

// EnsureClientTokenSchema adds the token table in a transaction without
// changing memory tables.
func (w *Writer) EnsureClientTokenSchema(ctx context.Context) error {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS client_tokens (
		id INTEGER PRIMARY KEY, device TEXT NOT NULL CHECK(length(device) BETWEEN 1 AND 64),
		token_hash BLOB NOT NULL UNIQUE CHECK(length(token_hash)=32),
		created_at TEXT NOT NULL, revoked_at TEXT)`); err != nil {
		return err
	}
	var columns int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info('client_tokens')
		WHERE name IN ('id','device','token_hash','created_at','revoked_at')`).Scan(&columns); err != nil || columns != 5 {
		return errors.New("client token schema is incompatible")
	}
	return tx.Commit()
}

// AddClientToken stores a new device token's SHA-256 digest and returns
// metadata only. The caller retains the raw token.
func (w *Writer) AddClientToken(ctx context.Context, device string, hash [sha256.Size]byte) (ClientToken, error) {
	if strings.TrimSpace(device) == "" || len(device) > 64 || strings.IndexFunc(device, unicode.IsControl) >= 0 {
		return ClientToken{}, errors.New("invalid device name")
	}
	created := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := w.db.ExecContext(ctx, `INSERT INTO client_tokens(device,token_hash,created_at) VALUES(?,?,?)`, device, hash[:], created)
	if err != nil {
		return ClientToken{}, err
	}
	id, err := result.LastInsertId()
	return ClientToken{ID: id, Device: device, CreatedAt: created}, err
}

// ClientTokenValid hashes a raw token and reports whether an unrevoked
// matching token exists.
func (r *Reader) ClientTokenValid(ctx context.Context, token string) (bool, error) {
	hash := sha256.Sum256([]byte(token))
	var exists bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM client_tokens WHERE token_hash=? AND revoked_at IS NULL)`, hash[:]).Scan(&exists)
	return exists, err
}

// ClientTokenDevice returns the registered device for an active credential.
// Empty means no matching active token; neither hashes nor other devices escape.
func (r *Reader) ClientTokenDevice(ctx context.Context, token string) (string, error) {
	hash := sha256.Sum256([]byte(token))
	var device string
	err := r.db.QueryRowContext(ctx, `SELECT device FROM client_tokens WHERE token_hash=? AND revoked_at IS NULL`, hash[:]).Scan(&device)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return device, err
}

// ListClientTokens returns token metadata without hashes or raw tokens.
func (r *Reader) ListClientTokens(ctx context.Context) ([]ClientToken, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,device,created_at,revoked_at FROM client_tokens ORDER BY revoked_at IS NOT NULL, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ClientToken{}
	for rows.Next() {
		var item ClientToken
		var revoked sql.NullString
		if err := rows.Scan(&item.ID, &item.Device, &item.CreatedAt, &revoked); err != nil {
			return nil, err
		}
		if revoked.Valid {
			item.RevokedAt = &revoked.String
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// RevokeClientToken marks an active token as revoked. It reports false for
// an unknown or already revoked ID.
func (w *Writer) RevokeClientToken(ctx context.Context, id int64) (bool, error) {
	if id <= 0 {
		return false, errors.New("invalid client token ID")
	}
	result, err := w.db.ExecContext(ctx, `UPDATE client_tokens SET revoked_at=? WHERE id=? AND revoked_at IS NULL`, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}
