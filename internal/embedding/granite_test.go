package embedding

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/MylesMCook/grasshopper/internal/memory"
)

func TestDotRejectsWrongDimensions(t *testing.T) {
	if _, err := Dot([]float32{1}, make([]float32, Dimensions)); err == nil {
		t.Fatal("dimension mismatch accepted")
	}
}

func TestGraniteFrozenHybridRecall(t *testing.T) {
	root := os.Getenv("GRASSHOPPER_EMBED_TEST_ROOT")
	if root == "" {
		t.Skip("set GRASSHOPPER_EMBED_TEST_ROOT for real ONNX recall")
	}
	b, err := NewGranite(os.Getenv("GRASSHOPPER_ONNX_RUNTIME_LIBRARY"), filepath.Join(root, "model.onnx"), filepath.Join(root, "tokenizer.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	data, err := os.ReadFile(filepath.Join("..", "..", "tests", "fixtures", "embedding-search-eval.json"))
	if err != nil {
		t.Fatal(err)
	}
	type corpus struct {
		Documents []struct {
			ID          int
			Text, Title string
			Scope       memory.Scope
			Archived    bool
		}
		Queries []struct {
			Text, Category string
			Expected       int
			Scope          memory.Scope
		}
	}
	var fixture struct{ Corpora []corpus }
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	top1, top3, total := 0, 0, 0
	longTop1, longTop3, longTotal := 0, 0, 0
	for _, corpus := range fixture.Corpora {
		database := filepath.Join(t.TempDir(), "recall.db")
		if err := memory.CreateEmpty(database); err != nil {
			t.Fatal(err)
		}
		w, err := memory.OpenWritableExisting(database, ModelName, Dimensions)
		if err != nil {
			t.Fatal(err)
		}
		defer w.Close()
		ctx := context.Background()
		ids := map[int]int64{}
		for _, document := range corpus.Documents {
			vector, err := b.EmbedDocument(document.Text)
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := w.Write(ctx, memory.WriteInput{Scope: document.Scope, Content: document.Text, Title: &document.Title, Purpose: "observation", RequestID: fmt.Sprintf("fixture-%d", document.ID), Provenance: memory.Provenance{Harness: "native-test", Device: "synthetic", Source: "frozen recall fixture"}}, vector, ModelName)
			if err != nil {
				t.Fatal(err)
			}
			ids[document.ID] = receipt.ID
			if document.Archived {
				_, err = w.Archive(ctx, memory.ArchiveInput{Scope: document.Scope, ID: receipt.ID, ExpectedRevision: receipt.Revision, Archived: true, RequestID: fmt.Sprintf("archive-%d", document.ID), Provenance: memory.Provenance{Harness: "native-test", Device: "synthetic", Source: "frozen recall fixture"}})
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		for _, query := range corpus.Queries {
			total++
			if query.Category == "long" {
				longTotal++
			}
			vector, err := b.EmbedQuery(query.Text)
			if err != nil {
				t.Fatal(err)
			}
			page, err := w.Search(ctx, query.Scope, query.Text, vector, ModelName, 10, 65536)
			if err != nil {
				t.Fatal(err)
			}
			for rank, record := range page.Records {
				if record.Archived || record.Scope.Legacy != query.Scope.Legacy {
					t.Fatal("archived or legacy record escaped its boundary")
				}
				for _, field := range [][2]*string{{record.Scope.Project, query.Scope.Project}, {record.Scope.Device, query.Scope.Device}, {record.Scope.Platform, query.Scope.Platform}} {
					if field[0] != nil && (field[1] == nil || *field[0] != *field[1]) {
						t.Fatal("search returned a record from another scope")
					}
				}
				if record.ID == ids[query.Expected] {
					if rank == 0 {
						top1++
						if query.Category == "long" {
							longTop1++
						}
					}
					if rank < 3 {
						top3++
						if query.Category == "long" {
							longTop3++
						}
					}
					break
				}
			}
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if total != 160 || top1 < 122 || top3 < 143 {
		t.Errorf("hybrid recall: top1=%d/%d top3=%d/%d; require 122/160 and 143/160", top1, total, top3, total)
	}
	if longTotal != 8 || longTop1 < 7 || longTop3 != 8 {
		t.Errorf("long-handoff recall: top1=%d/%d top3=%d/%d; require 7/8 and 8/8", longTop1, longTotal, longTop3, longTotal)
	}
	t.Logf("hybrid top1=%d/%d top3=%d/%d; long top1=%d/%d top3=%d/%d", top1, total, top3, total, longTop1, longTotal, longTop3, longTotal)
}

func TestNewGraniteRejectsUnpinnedModelBeforeRuntimeLoad(t *testing.T) {
	model := filepath.Join(t.TempDir(), "model.onnx")
	if err := os.WriteFile(model, []byte("synthetic wrong model"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := NewGranite("missing-runtime", model, "missing-tokenizer")
	if err == nil || !strings.Contains(err.Error(), "model: SHA-256 mismatch") {
		t.Fatalf("unpinned model was accepted: %v", err)
	}
}

func TestGraniteVerifiesWeightsAndTokenizerBeforeRuntimeLoad(t *testing.T) {
	root := os.Getenv("GRASSHOPPER_EMBED_TEST_ROOT")
	if root == "" {
		t.Skip("set GRASSHOPPER_EMBED_TEST_ROOT for pinned asset checks")
	}
	dir := t.TempDir()
	model, err := os.ReadFile(filepath.Join(root, "model.onnx"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "model.onnx")
	if err := os.WriteFile(path, model, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewGranite("missing-runtime", path, "missing-tokenizer"); err == nil || !strings.Contains(err.Error(), "Granite weights:") {
		t.Fatalf("missing external weights accepted: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "model.onnx_data"), []byte("corrupt weights"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewGranite("missing-runtime", path, "missing-tokenizer"); err == nil || !strings.Contains(err.Error(), "weights: SHA-256 mismatch") {
		t.Fatalf("corrupt external weights accepted: %v", err)
	}
	if _, err := NewGranite("missing-runtime", filepath.Join(root, "model.onnx"), path); err == nil || !strings.Contains(err.Error(), "tokenizer: SHA-256 mismatch") {
		t.Fatalf("wrong tokenizer accepted: %v", err)
	}
}

func TestGranitePublishedEmbeddingExamples(t *testing.T) {
	root := os.Getenv("GRASSHOPPER_EMBED_TEST_ROOT")
	if root == "" {
		t.Skip("set GRASSHOPPER_EMBED_TEST_ROOT for real ONNX examples")
	}
	b, err := NewGranite(os.Getenv("GRASSHOPPER_ONNX_RUNTIME_LIBRARY"), filepath.Join(root, "model.onnx"), filepath.Join(root, "tokenizer.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	// These four cosine scores are published by the pinned ONNX exporter.
	queries := []string{" Who made the song My achy breaky heart? ", "summit define"}
	documents := []string{
		"Achy Breaky Heart is a country song written by Don Von Tress. Originally titled Don't Tell My Heart and performed by The Marcy Brothers in 1991. ",
		"Definition of summit for English Language Learners. : 1 the highest point of a mountain : the top of a mountain. : 2 the highest level. : 3 a meeting or series of meetings between the leaders of two or more governments.",
	}
	want := [2][2]float32{{0.8931542634963989, 0.6678562164306641}, {0.712432324886322, 0.8434768915176392}}
	for i, query := range queries {
		q, err := b.EmbedQuery(query)
		if err != nil {
			t.Fatal(err)
		}
		for j, document := range documents {
			d, err := b.EmbedDocument(document)
			if err != nil {
				t.Fatal(err)
			}
			score, err := Dot(q, d)
			if err != nil || math.Abs(float64(score-want[i][j])) > 0.00001 {
				t.Fatalf("published cosine %d,%d: got %v want %v: %v", i, j, score, want[i][j], err)
			}
		}
	}
}

func TestGraniteLongMemoryFitsModelWindow(t *testing.T) {
	root := os.Getenv("GRASSHOPPER_EMBED_TEST_ROOT")
	if root == "" {
		t.Skip("set GRASSHOPPER_EMBED_TEST_ROOT to run real ONNX inference")
	}
	library := os.Getenv("GRASSHOPPER_ONNX_RUNTIME_LIBRARY")
	b, err := NewGranite(library,
		filepath.Join(root, "model.onnx"),
		filepath.Join(root, "tokenizer.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	// The service accepts up to 32768 bytes. This input exceeds the token window
	// with valid UTF-8; truncation must still produce a normalized vector.
	longMemory := strings.Repeat("a ", 16384)
	encoding, err := b.tokenizer.EncodeSingle(longMemory, true)
	if err != nil || len(encoding.GetIds()) != maxTokens {
		t.Fatalf("token window bound failed: %v", err)
	}
	vector, err := b.EmbedDocument(longMemory)
	if err != nil || len(vector) != Dimensions {
		t.Fatalf("long memory should remain storable: dimensions=%d error=%v", len(vector), err)
	}
	self, err := Dot(vector, vector)
	if err != nil || math.Abs(float64(self-1)) > 0.0001 {
		t.Fatalf("long vector norm: %v %v", self, err)
	}
}

func TestActualGraniteRecall(t *testing.T) {
	root := os.Getenv("GRASSHOPPER_EMBED_TEST_ROOT")
	if root == "" {
		t.Skip("set GRASSHOPPER_EMBED_TEST_ROOT to run real ONNX recall")
	}
	library := os.Getenv("GRASSHOPPER_ONNX_RUNTIME_LIBRARY")
	model := filepath.Join(root, "model.onnx")
	tokenizer := filepath.Join(root, "tokenizer.json")
	b, err := NewGranite(library, model, tokenizer)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := b.Close(); err != nil {
			t.Error(err)
		}
	}()
	fixtureBytes, err := os.ReadFile(filepath.Join("..", "..", "tests", "fixtures", "embedding-eval.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Documents []struct {
			ID   int    `json:"id"`
			Text string `json:"text"`
		} `json:"documents"`
		Queries []struct {
			Expected int    `json:"expected"`
			Text     string `json:"text"`
		} `json:"queries"`
	}
	if err := json.Unmarshal(fixtureBytes, &fixture); err != nil {
		t.Fatal(err)
	}
	vectors := make([][]float32, len(fixture.Documents))
	for i, document := range fixture.Documents {
		vectors[i], err = b.EmbedDocument(document.Text)
		if err != nil {
			t.Fatal(err)
		}
		if len(vectors[i]) != Dimensions {
			t.Fatalf("document vector has %d dimensions", len(vectors[i]))
		}
		self, err := Dot(vectors[i], vectors[i])
		if err != nil || math.Abs(float64(self-1)) > 0.0001 {
			t.Fatalf("document vector is not normalized: %v %v", self, err)
		}
	}
	top1, top3 := 0, 0
	for _, query := range fixture.Queries {
		vector, err := b.EmbedQuery(query.Text)
		if err != nil {
			t.Fatal(err)
		}
		type hit struct {
			id    int
			score float32
		}
		hits := make([]hit, len(vectors))
		for i, document := range fixture.Documents {
			score, err := Dot(vector, vectors[i])
			if err != nil {
				t.Fatal(err)
			}
			hits[i] = hit{document.ID, score}
		}
		sort.Slice(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
		for rank, result := range hits {
			if result.id != query.Expected {
				continue
			}
			if rank == 0 {
				top1++
			}
			if rank < 3 {
				top3++
			}
			break
		}
	}
	if top1 < 7 || top3 != len(fixture.Queries) {
		t.Fatalf("Granite recall regressed: top1=%d/%d top3=%d/%d", top1, len(fixture.Queries), top3, len(fixture.Queries))
	}
	t.Logf("real Granite recall: top1=%d/%d top3=%d/%d", top1, len(fixture.Queries), top3, len(fixture.Queries))

	reader, err := memory.OpenReadOnly(filepath.Join("..", "..", "tests", "fixtures", "go-compat", "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, scenario := range []struct {
		project string
		query   string
		wantID  int64
	}{
		{"id:project-a", "Which tool should install dependencies in project A?", 2},
		{"id:project-b", "Which tool should install dependencies in project B?", 3},
	} {
		project := scenario.project
		candidates, err := reader.Candidates(context.Background(), memory.Scope{Project: &project})
		if err != nil {
			t.Fatal(err)
		}
		queryVector, err := b.EmbedQuery(scenario.query)
		if err != nil {
			t.Fatal(err)
		}
		var topID int64
		var topScore float32 = -2
		for _, candidate := range candidates {
			documentVector, err := b.EmbedDocument(candidate.Content)
			if err != nil {
				t.Fatal(err)
			}
			score, err := Dot(queryVector, documentVector)
			if err != nil {
				t.Fatal(err)
			}
			if score > topScore {
				topScore, topID = score, candidate.ID
			}
		}
		if topID != scenario.wantID {
			t.Errorf("%s semantic top result: got record %d, want %d", scenario.project, topID, scenario.wantID)
		}
	}
}

func TestGraniteReembedCopySurvivesReopen(t *testing.T) {
	root := os.Getenv("GRASSHOPPER_EMBED_TEST_ROOT")
	if root == "" {
		t.Skip("set GRASSHOPPER_EMBED_TEST_ROOT to run real ONNX migration")
	}
	library := os.Getenv("GRASSHOPPER_ONNX_RUNTIME_LIBRARY")
	b, err := NewGranite(library, filepath.Join(root, "model.onnx"), filepath.Join(root, "tokenizer.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	ctx := context.Background()
	source := filepath.Join("..", "..", "tests", "fixtures", "go-compat", "memory.db")
	destination := filepath.Join(t.TempDir(), "granite-shadow.db")
	w, err := memory.ReembedCopy(ctx, source, destination, ModelName, b.EmbedDocument)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := memory.OpenReadOnly(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	for _, scenario := range []struct {
		project, query string
		wantID         int64
	}{
		{"id:project-a", "Which dependency manager should project A use?", 2},
		{"id:project-b", "Which dependency manager should project B use?", 3},
	} {
		vector, err := b.EmbedQuery(scenario.query)
		if err != nil {
			t.Fatal(err)
		}
		project := scenario.project
		page, err := reopened.Search(ctx, memory.Scope{Project: &project}, scenario.query, vector, ModelName, 10, 16000)
		if err != nil || len(page.Records) == 0 || page.Records[0].ID != scenario.wantID {
			t.Fatalf("reopened %s recall: %+v %v", project, page, err)
		}
	}
}

func TestGraniteWriterImmediateRecall(t *testing.T) {
	root := os.Getenv("GRASSHOPPER_EMBED_TEST_ROOT")
	if root == "" {
		t.Skip("set GRASSHOPPER_EMBED_TEST_ROOT to run real ONNX recall")
	}
	library := os.Getenv("GRASSHOPPER_ONNX_RUNTIME_LIBRARY")
	model := filepath.Join(root, "model.onnx")
	tokenizer := filepath.Join(root, "tokenizer.json")
	b, err := NewGranite(library, model, tokenizer)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := b.Close(); err != nil {
			t.Error(err)
		}
	}()
	ctx := context.Background()
	source := filepath.Join("..", "..", "tests", "fixtures", "go-compat", "memory.db")
	w, err := memory.OpenWritableCopy(ctx, source, filepath.Join(t.TempDir(), "granite-copy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	project := "id:project-c"
	content := "Use pnpm for JavaScript packages in project C."
	documentVector, err := b.EmbedDocument(content)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := w.Write(ctx, memory.WriteInput{
		Scope: memory.Scope{Project: &project}, Content: content, Purpose: "decision", Confirmed: true,
		Provenance: memory.Provenance{Harness: "go-probe", Device: "synthetic-mac", Source: "Granite immediate recall test"},
		RequestID:  "go-granite-immediate-recall",
	}, documentVector, ModelName)
	if err != nil {
		t.Fatal(err)
	}
	query := "Which dependency manager should the third app run?"
	lexical, err := w.Search(ctx, memory.Scope{Project: &project}, query, nil, "", 10, 16000)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range lexical.Records {
		if record.ID == receipt.ID {
			t.Fatalf("new record also matched lexically: %+v", lexical)
		}
	}
	queryVector, err := b.EmbedQuery(query)
	if err != nil {
		t.Fatal(err)
	}
	semantic, err := w.Search(ctx, memory.Scope{Project: &project}, query, queryVector, ModelName, 10, 16000)
	if err != nil {
		t.Fatal(err)
	}
	if len(semantic.Records) == 0 || semantic.Records[0].ID != receipt.ID {
		t.Fatalf("new Granite record not recalled: %+v", semantic)
	}
	t.Logf("Granite write recalled without restart: lexical=%d hybrid=%d", len(lexical.Records), len(semantic.Records))
}
