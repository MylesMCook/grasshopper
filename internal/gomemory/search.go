package gomemory

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"unicode"
)

// Search reads the existing FTS5 and vector columns. Scope is constrained by
// SQLite before either candidate list is ranked. This does not embed queries.
func (r *Reader) Search(ctx context.Context, scope Scope, query string, vector []float32, model string, limit, budget int) (Page, error) {
	if strings.TrimSpace(query) == "" || len(query) > 4096 {
		return Page{}, errors.New("query must be 1-4096 bytes")
	}
	keys, err := scope.applicableKeys()
	if err != nil {
		return Page{}, err
	}
	keyJSON, err := json.Marshal(keys)
	if err != nil {
		return Page{}, err
	}
	if limit < 1 {
		limit = 1
	} else if limit > 100 {
		limit = 100
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Page{}, err
	}
	defer tx.Rollback()
	scores := make(map[int64]float64)
	records := make(map[int64]Record)
	if prepared := prepareFTSQuery(query); prepared != "" {
		rows, err := tx.QueryContext(ctx, "SELECT "+prefixedRecordColumns("c")+` FROM chunks_fts JOIN chunks c ON c.id=chunks_fts.rowid
 WHERE chunks_fts MATCH ? AND c.kind='memory' AND c.archived=0
 AND c.memory_scope IN (SELECT value FROM json_each(?)) ORDER BY bm25(chunks_fts),c.id`, prepared, string(keyJSON))
		if err != nil {
			return Page{}, err
		}
		rank := 0
		for rows.Next() {
			record, err := scanRecord(rows)
			if err != nil {
				rows.Close()
				return Page{}, err
			}
			scores[record.ID] += 0.4 / float64(61+rank)
			records[record.ID] = record
			rank++
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return Page{}, err
		}
	}
	if vector != nil {
		if len(vector) == 0 || model == "" {
			return Page{}, errors.New("vector and model must be nonempty")
		}
		for _, value := range vector {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return Page{}, errors.New("invalid query vector")
			}
		}
		rows, err := tx.QueryContext(ctx, `SELECT c.id,c.memory_scope,c.embedding FROM chunks c WHERE c.kind='memory' AND c.archived=0
 AND c.memory_scope IN (SELECT value FROM json_each(?)) AND c.embedding_model=? AND c.embedding IS NOT NULL`, string(keyJSON), model)
		if err != nil {
			return Page{}, err
		}
		type hit struct {
			id         int64
			scope      Scope
			similarity float64
		}
		hits := []hit{}
		for rows.Next() {
			var id int64
			var scopeJSON string
			var blob []byte
			if err := rows.Scan(&id, &scopeJSON, &blob); err != nil {
				rows.Close()
				return Page{}, err
			}
			var recordScope Scope
			if err := json.Unmarshal([]byte(scopeJSON), &recordScope); err != nil {
				rows.Close()
				return Page{}, err
			}
			if len(blob) != len(vector)*4 {
				continue
			}
			var dot, queryNorm, rowNorm float64
			for i, queryValue := range vector {
				rowValue := math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:]))
				dot += float64(queryValue) * float64(rowValue)
				queryNorm += float64(queryValue) * float64(queryValue)
				rowNorm += float64(rowValue) * float64(rowValue)
			}
			similarity := dot / math.Sqrt(queryNorm*rowNorm)
			if !math.IsNaN(similarity) && !math.IsInf(similarity, 0) && similarity >= 0.3 {
				hits = append(hits, hit{id, recordScope, similarity})
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return Page{}, err
		}
		sort.Slice(hits, func(i, j int) bool {
			if hits[i].similarity != hits[j].similarity {
				return hits[i].similarity > hits[j].similarity
			}
			return hits[i].id < hits[j].id
		})
		for rank, hit := range hits {
			scores[hit.id] += 0.6 / float64(61+rank)
			if _, exists := records[hit.id]; !exists {
				scopeKey, err := hit.scope.Key()
				if err != nil {
					return Page{}, err
				}
				record, err := scanRecord(tx.QueryRowContext(ctx, "SELECT "+recordColumns+" FROM chunks WHERE id=? AND kind='memory' AND archived=0 AND memory_scope=?", hit.id, scopeKey))
				if err != nil {
					return Page{}, err
				}
				records[hit.id] = record
			}
		}
	}
	ids := make([]int64, 0, len(scores))
	for id := range scores {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if scores[ids[i]] != scores[ids[j]] {
			return scores[ids[i]] > scores[ids[j]]
		}
		return ids[i] < ids[j]
	})
	ranked := make([]Record, 0, len(ids))
	for _, id := range ids {
		if record, exists := records[id]; exists {
			ranked = append(ranked, record)
		}
	}
	page, err := boundRecords(ranked, budget, limit)
	if err != nil {
		return Page{}, err
	}
	if err := tx.Commit(); err != nil {
		return Page{}, err
	}
	return page, nil
}

func prefixedRecordColumns(prefix string) string {
	parts := strings.Split(strings.ReplaceAll(recordColumns, "\n", " "), ",")
	for i, part := range parts {
		parts[i] = prefix + "." + strings.TrimSpace(part)
	}
	return strings.Join(parts, ",")
}

func prepareFTSQuery(query string) string {
	terms := []string{}
	seen := map[string]bool{}
	add := func(term string) {
		term = strings.ToLower(term)
		if term != "" && !seen[term] {
			seen[term] = true
			terms = append(terms, "\""+term+"\"")
		}
	}
	words := strings.FieldsFunc(query, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	for _, word := range words {
		add(word)
	}
	for _, word := range words {
		for _, part := range splitCamel(word) {
			add(part)
		}
	}
	return strings.Join(terms, " OR ")
}

func splitCamel(word string) []string {
	runes := []rune(word)
	parts := []string{}
	start := 0
	for i := 1; i < len(runes); i++ {
		if unicode.IsUpper(runes[i]) && (unicode.IsLower(runes[i-1]) || (unicode.IsUpper(runes[i-1]) && i+1 < len(runes) && unicode.IsLower(runes[i+1]))) {
			parts = append(parts, string(runes[start:i]))
			start = i
		}
	}
	if start > 0 {
		parts = append(parts, string(runes[start:]))
	}
	return parts
}
