package memory

import (
	"context"
	"fmt"
)

// SnapshotCounts verifies SQLite integrity and counts current memories and
// historical revisions using the reader's read-only connection.
func (r *Reader) SnapshotCounts(ctx context.Context) (records, revisions int64, quickCheck string, err error) {
	rows, err := r.db.QueryContext(ctx, "PRAGMA quick_check")
	if err != nil {
		return 0, 0, "", err
	}
	defer rows.Close()
	for rows.Next() {
		var check string
		if err := rows.Scan(&check); err != nil {
			return 0, 0, "", err
		}
		if check != "ok" {
			return 0, 0, "", fmt.Errorf("database quick_check failed: %s", check)
		}
		quickCheck = check
	}
	if err := rows.Err(); err != nil {
		return 0, 0, "", err
	}
	if err := rows.Close(); err != nil {
		return 0, 0, "", err
	}
	if quickCheck != "ok" {
		return 0, 0, "", fmt.Errorf("database quick_check returned no result")
	}
	if err := r.db.QueryRowContext(ctx, "SELECT count(*) FROM chunks WHERE kind='memory'").Scan(&records); err != nil {
		return 0, 0, "", err
	}
	if err := r.db.QueryRowContext(ctx, "SELECT count(*) FROM memory_revisions").Scan(&revisions); err != nil {
		return 0, 0, "", err
	}
	return records, revisions, quickCheck, nil
}
