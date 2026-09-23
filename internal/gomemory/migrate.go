package gomemory

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
)

// ReembedCopy makes a consistent, separate SQLite copy and replaces only
// memory embeddings in that copy. IDs, revisions, history, code rows, and the
// source database are preserved. Call it with the sole writer stopped before
// any eventual cutover so writes cannot be lost between snapshot and switch.
func ReembedCopy(ctx context.Context, source, destination, model string, embed func(string) ([]float32, error)) (_ *Writer, err error) {
	if model == "" || embed == nil {
		return nil, errors.New("model and document embedder are required")
	}
	w, err := OpenWritableCopy(ctx, source, destination)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = w.Close()
			_ = os.Remove(destination)
		}
	}()
	type memory struct {
		id      int64
		content string
	}
	rows, err := w.db.QueryContext(ctx, "SELECT id,content FROM chunks WHERE kind='memory' ORDER BY id")
	if err != nil {
		return nil, err
	}
	memories := []memory{}
	for rows.Next() {
		var item memory
		if err := rows.Scan(&item.id, &item.content); err != nil {
			rows.Close()
			return nil, err
		}
		memories = append(memories, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	type result struct {
		id     int64
		vector []float32
	}
	converted := make([]result, 0, len(memories))
	dimensions := 0
	for _, item := range memories {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		vector, err := embed(item.content)
		if err != nil {
			return nil, fmt.Errorf("embed memory %d: %w", item.id, err)
		}
		if len(vector) == 0 || len(vector) > 4096 || (dimensions != 0 && len(vector) != dimensions) {
			return nil, fmt.Errorf("invalid embedding dimensions for memory %d", item.id)
		}
		for _, value := range vector {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return nil, fmt.Errorf("non-finite embedding for memory %d", item.id)
			}
		}
		dimensions = len(vector)
		converted = append(converted, result{item.id, vector})
	}
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for _, item := range converted {
		blob := vectorBlob(item.vector)
		if _, err := tx.ExecContext(ctx, "UPDATE chunks SET embedding=?,embedding_model=? WHERE id=? AND kind='memory'", blob, model, item.id); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	var check string
	if err := w.db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&check); err != nil {
		return nil, err
	}
	if check != "ok" {
		return nil, fmt.Errorf("migrated database quick_check: %s", check)
	}
	return w, nil
}
