package goembed

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/MylesMCook/grasshopper/internal/gomemory"
)

func TestDotRejectsWrongDimensions(t *testing.T) {
	if _, err := Dot([]float32{1}, make([]float32, Dimensions)); err == nil {
		t.Fatal("dimension mismatch accepted")
	}
}

func TestNewBGERejectsUnpinnedModelBeforeRuntimeLoad(t *testing.T) {
	model := filepath.Join(t.TempDir(), "model.onnx")
	if err := os.WriteFile(model, []byte("synthetic wrong model"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := NewBGE("missing-runtime", model, "missing-tokenizer")
	if err == nil || !strings.Contains(err.Error(), "model: SHA-256 mismatch") {
		t.Fatalf("unpinned model was accepted: %v", err)
	}
}

func TestActualBGERecall(t *testing.T) {
	root := os.Getenv("GRASSHOPPER_BGE_TEST_ROOT")
	if root == "" {
		t.Skip("set GRASSHOPPER_BGE_TEST_ROOT to run real ONNX recall")
	}
	library := filepath.Join(root, "go-probe", "onnxruntime-osx-arm64-1.30.0", "lib", "libonnxruntime.dylib")
	if configured := os.Getenv("GRASSHOPPER_ONNX_RUNTIME_LIBRARY"); configured != "" {
		library = configured
	}
	model := filepath.Join(root, "models", "bge-small-en-v1.5", "onnx", "model.onnx")
	tokenizer := filepath.Join(root, "models", "bge-small-en-v1.5", "tokenizer.json")
	b, err := NewBGE(library, model, tokenizer)
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
		t.Fatalf("BGE recall regressed: top1=%d/%d top3=%d/%d", top1, len(fixture.Queries), top3, len(fixture.Queries))
	}
	t.Logf("real BGE recall: top1=%d/%d top3=%d/%d", top1, len(fixture.Queries), top3, len(fixture.Queries))

	reader, err := gomemory.OpenReadOnly(filepath.Join("..", "..", "tests", "fixtures", "go-compat", "memory.db"))
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
		candidates, err := reader.Candidates(context.Background(), gomemory.Scope{Project: &project})
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

func TestBGEReembedCopySurvivesReopen(t *testing.T) {
	root := os.Getenv("GRASSHOPPER_BGE_TEST_ROOT")
	if root == "" {
		t.Skip("set GRASSHOPPER_BGE_TEST_ROOT to run real ONNX migration")
	}
	library := filepath.Join(root, "go-probe", "onnxruntime-osx-arm64-1.30.0", "lib", "libonnxruntime.dylib")
	if configured := os.Getenv("GRASSHOPPER_ONNX_RUNTIME_LIBRARY"); configured != "" {
		library = configured
	}
	b, err := NewBGE(library, filepath.Join(root, "models", "bge-small-en-v1.5", "onnx", "model.onnx"), filepath.Join(root, "models", "bge-small-en-v1.5", "tokenizer.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	ctx := context.Background()
	source := filepath.Join("..", "..", "tests", "fixtures", "go-compat", "memory.db")
	destination := filepath.Join(t.TempDir(), "bge-shadow.db")
	w, err := gomemory.ReembedCopy(ctx, source, destination, ModelName, b.EmbedDocument)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := gomemory.OpenReadOnly(destination)
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
		page, err := reopened.Search(ctx, gomemory.Scope{Project: &project}, scenario.query, vector, ModelName, 10, 16000)
		if err != nil || len(page.Records) == 0 || page.Records[0].ID != scenario.wantID {
			t.Fatalf("reopened %s recall: %+v %v", project, page, err)
		}
	}
}

func TestBGEWriterImmediateRecall(t *testing.T) {
	root := os.Getenv("GRASSHOPPER_BGE_TEST_ROOT")
	if root == "" {
		t.Skip("set GRASSHOPPER_BGE_TEST_ROOT to run real ONNX recall")
	}
	library := filepath.Join(root, "go-probe", "onnxruntime-osx-arm64-1.30.0", "lib", "libonnxruntime.dylib")
	if configured := os.Getenv("GRASSHOPPER_ONNX_RUNTIME_LIBRARY"); configured != "" {
		library = configured
	}
	model := filepath.Join(root, "models", "bge-small-en-v1.5", "onnx", "model.onnx")
	tokenizer := filepath.Join(root, "models", "bge-small-en-v1.5", "tokenizer.json")
	b, err := NewBGE(library, model, tokenizer)
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
	w, err := gomemory.OpenWritableCopy(ctx, source, filepath.Join(t.TempDir(), "bge-copy.db"))
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
	receipt, err := w.Write(ctx, gomemory.WriteInput{
		Scope: gomemory.Scope{Project: &project}, Content: content, Purpose: "decision", Confirmed: true,
		Provenance: gomemory.Provenance{Harness: "go-probe", Device: "synthetic-mac", Source: "BGE immediate recall test"},
		RequestID:  "go-bge-immediate-recall",
	}, documentVector, ModelName)
	if err != nil {
		t.Fatal(err)
	}
	query := "Which dependency manager should the third app run?"
	lexical, err := w.Search(ctx, gomemory.Scope{Project: &project}, query, nil, "", 10, 16000)
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
	semantic, err := w.Search(ctx, gomemory.Scope{Project: &project}, query, queryVector, ModelName, 10, 16000)
	if err != nil {
		t.Fatal(err)
	}
	if len(semantic.Records) == 0 || semantic.Records[0].ID != receipt.ID {
		t.Fatalf("new BGE record not recalled: %+v", semantic)
	}
	t.Logf("BGE write recalled without restart: lexical=%d hybrid=%d", len(lexical.Records), len(semantic.Records))
}
