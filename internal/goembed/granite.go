// Package goembed runs local Granite inference. Database writes belong to gomemory.
package goembed

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/sugarme/tokenizer"
	"github.com/sugarme/tokenizer/pretrained"
	ort "github.com/yalue/onnxruntime_go"
)

// ModelName identifies the pinned embedding model stored with each vector.
const ModelName = "onnx-community/granite-embedding-small-english-r2-ONNX@1dc7835ba0cb9c76a3618d0bf0c427c97671b3c8"

// Dimensions is the length of a Granite document or query vector.
const Dimensions = 384

// The export supports 8192 tokens, but dense CPU attention at that length
// exceeds the service's inference deadline. Bound work to 2048 tokens while
// preserving the complete stored text (the previous model embedded 512).
const maxTokens = 2048
const modelSHA256 = "cddb145cd1147ec24a3908b2ca2602b98b20a3d198365cff270b7cb26c98179e"
const modelDataSHA256 = "86a3a705d4598615894d89540ea71a3d9bbdb17a315e79edcd5dfc737222834b"
const tokenizerSHA256 = "feeb83348dcb033bc6b9d2e1f7906ca9eb2d122845000c9416d894d7c2927149"

// ONNX Runtime's environment and library path are process-global. Only one
// Granite instance may own them at a time.
var environmentMu sync.Mutex
var environmentInUse bool

// Granite holds one ONNX session. Embed and Close calls are safe across
// goroutines but serialize on the instance mutex.
type Granite struct {
	mu          sync.Mutex
	tokenizer   *tokenizer.Tokenizer
	session     *ort.DynamicAdvancedSession
	inputNames  []string
	outputNames []string
	closed      bool
}

// NewGranite verifies the graph, adjacent model.onnx_data weights, and tokenizer
// before loading ONNX. The external weight filename is part of the pinned graph.
// Only one Granite may be open per process; callers must Close it when finished.
func NewGranite(libraryPath, modelPath, tokenizerPath string) (_ *Granite, err error) {
	if err := verifyDigest(modelPath, modelSHA256); err != nil {
		return nil, fmt.Errorf("Granite model: %w", err)
	}
	if err := verifyDigest(filepath.Join(filepath.Dir(modelPath), "model.onnx_data"), modelDataSHA256); err != nil {
		return nil, fmt.Errorf("Granite weights: %w", err)
	}
	if err := verifyDigest(tokenizerPath, tokenizerSHA256); err != nil {
		return nil, fmt.Errorf("Granite tokenizer: %w", err)
	}
	environmentMu.Lock()
	if environmentInUse {
		environmentMu.Unlock()
		return nil, errors.New("ONNX environment already in use")
	}
	environmentInUse = true
	environmentMu.Unlock()
	defer func() {
		if err != nil {
			ort.DestroyEnvironment()
			environmentMu.Lock()
			environmentInUse = false
			environmentMu.Unlock()
		}
	}()
	ort.SetSharedLibraryPath(libraryPath)
	if err = ort.InitializeEnvironment(); err != nil {
		return nil, err
	}
	modelTokenizer, err := pretrained.FromFile(tokenizerPath)
	if err != nil {
		return nil, err
	}
	modelTokenizer.WithTruncation(&tokenizer.TruncationParams{MaxLength: maxTokens, Strategy: tokenizer.LongestFirst})
	inputNames := []string{"input_ids", "attention_mask"}
	// Request the published sentence vector, not the larger token tensor.
	outputNames := []string{"sentence_embedding"}
	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, err
	}
	defer opts.Destroy()
	if err = opts.SetIntraOpNumThreads(runtime.NumCPU()); err != nil {
		return nil, err
	}
	if err = opts.SetInterOpNumThreads(1); err != nil {
		return nil, err
	}
	session, err := ort.NewDynamicAdvancedSession(modelPath, inputNames, outputNames, opts)
	if err != nil {
		return nil, err
	}
	return &Granite{tokenizer: modelTokenizer, session: session, inputNames: inputNames, outputNames: outputNames}, nil
}

func verifyDigest(path, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	if fmt.Sprintf("%x", hash.Sum(nil)) != expected {
		return errors.New("SHA-256 mismatch")
	}
	return nil
}

// Close releases the ONNX session and process-wide environment. It waits
// for any in-flight embedding on this Granite and is safe to call again.
func (b *Granite) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	sessionErr := b.session.Destroy()
	environmentErr := ort.DestroyEnvironment()
	environmentMu.Lock()
	environmentInUse = false
	environmentMu.Unlock()
	return errors.Join(sessionErr, environmentErr)
}

// EmbedDocument returns a normalized vector for stored memory text.
func (b *Granite) EmbedDocument(text string) ([]float32, error) { return b.embed(text) }

// EmbedQuery uses the same prefix-free encoding as documents for this model.
func (b *Granite) EmbedQuery(query string) ([]float32, error) { return b.embed(query) }

func (b *Granite) embed(text string) ([]float32, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, errors.New("Granite embedder is closed")
	}
	encoding, err := b.tokenizer.EncodeSingle(text, true)
	if err != nil {
		return nil, err
	}
	ids := encoding.GetIds()
	mask := encoding.GetAttentionMask()
	if len(ids) == 0 || len(mask) != len(ids) {
		return nil, errors.New("invalid Granite tokenization")
	}
	values := map[string][]int64{"input_ids": {}, "attention_mask": {}}
	for i, id := range ids {
		values["input_ids"] = append(values["input_ids"], int64(id))
		values["attention_mask"] = append(values["attention_mask"], int64(mask[i]))
	}
	shape := ort.NewShape(1, int64(len(ids)))
	inputs := make([]ort.Value, len(b.inputNames))
	defer func() {
		for _, input := range inputs {
			if input != nil {
				input.Destroy()
			}
		}
	}()
	for i, name := range b.inputNames {
		input, exists := values[name]
		if !exists {
			return nil, fmt.Errorf("unsupported Granite input %q", name)
		}
		tensor, err := ort.NewTensor(shape, input)
		if err != nil {
			return nil, err
		}
		inputs[i] = tensor
	}
	outputs := make([]ort.Value, len(b.outputNames))
	defer func() {
		for _, output := range outputs {
			if output != nil {
				output.Destroy()
			}
		}
	}()
	if err := b.session.Run(inputs, outputs); err != nil {
		return nil, err
	}
	tensor, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("unexpected Granite output %T", outputs[0])
	}
	dims := tensor.GetShape()
	if len(dims) != 2 || dims[0] != 1 || dims[1] != Dimensions {
		return nil, fmt.Errorf("unexpected Granite output shape %v", dims)
	}
	data := tensor.GetData()
	if len(data) != Dimensions {
		return nil, errors.New("short Granite output")
	}
	// Copy before destroying the output tensor, then normalize for cosine search.
	vector := append([]float32(nil), data...)
	norm := float64(0)
	for _, value := range vector {
		norm += float64(value * value)
	}
	norm = math.Sqrt(norm)
	if norm == 0 || math.IsNaN(norm) || math.IsInf(norm, 0) {
		return nil, errors.New("invalid Granite vector norm")
	}
	for i := range vector {
		vector[i] /= float32(norm)
	}
	return vector, nil
}

// Dot returns the dot product of two Granite-sized vectors and rejects other
// dimensions.
func Dot(a, b []float32) (float32, error) {
	if len(a) != Dimensions || len(b) != Dimensions {
		return 0, errors.New("Granite vector dimension mismatch")
	}
	var score float32
	for i := range a {
		score += a[i] * b[i]
	}
	return score, nil
}
