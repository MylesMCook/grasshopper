// Package goembed runs local BGE inference for the Go memory pilot. Database
// writes are owned by gomemory, not the embedder.
package goembed

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"sync"

	"github.com/sugarme/tokenizer"
	"github.com/sugarme/tokenizer/pretrained"
	ort "github.com/yalue/onnxruntime_go"
)

const ModelName = "BAAI/bge-small-en-v1.5@5c38ec7c405ec4b44b94cc5a9bb96e735b38267a"
const Dimensions = 384
const QueryPrefix = "Represent this sentence for searching relevant passages: "
const modelSHA256 = "828e1496d7fabb79cfa4dcd84fa38625c0d3d21da474a00f08db0f559940cf35"
const tokenizerSHA256 = "d241a60d5e8f04cc1b2b3e9ef7a4921b27bf526d9f6050ab90f9267a1f9e5c66"

// ONNX Runtime's environment and library path are process-global. Use one
// embedder per process until the full service lifecycle is implemented.
var environmentMu sync.Mutex
var environmentInUse bool

type BGE struct {
	mu          sync.Mutex
	tokenizer   *tokenizer.Tokenizer
	session     *ort.DynamicAdvancedSession
	inputNames  []string
	outputNames []string
	closed      bool
}

func NewBGE(libraryPath, modelPath, tokenizerPath string) (_ *BGE, err error) {
	if err := verifyDigest(modelPath, modelSHA256); err != nil {
		return nil, fmt.Errorf("BGE model: %w", err)
	}
	if err := verifyDigest(tokenizerPath, tokenizerSHA256); err != nil {
		return nil, fmt.Errorf("BGE tokenizer: %w", err)
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
	tokenizer, err := pretrained.FromFile(tokenizerPath)
	if err != nil {
		return nil, err
	}
	inputs, outputs, err := ort.GetInputOutputInfo(modelPath)
	if err != nil {
		return nil, err
	}
	inputNames := make([]string, len(inputs))
	for i, input := range inputs {
		inputNames[i] = input.Name
	}
	outputNames := make([]string, len(outputs))
	for i, output := range outputs {
		outputNames[i] = output.Name
	}
	if len(outputNames) == 0 {
		return nil, errors.New("BGE model has no output")
	}
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
	return &BGE{tokenizer: tokenizer, session: session, inputNames: inputNames, outputNames: outputNames}, nil
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

func (b *BGE) Close() error {
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

func (b *BGE) EmbedDocument(text string) ([]float32, error) { return b.embed(text) }

func (b *BGE) EmbedQuery(query string) ([]float32, error) { return b.embed(QueryPrefix + query) }

func (b *BGE) embed(text string) ([]float32, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, errors.New("BGE embedder is closed")
	}
	encoding, err := b.tokenizer.EncodeSingle(text, true)
	if err != nil {
		return nil, err
	}
	ids := encoding.GetIds()
	mask := encoding.GetAttentionMask()
	types := encoding.GetTypeIds()
	if len(ids) == 0 || len(mask) != len(ids) || len(types) != len(ids) {
		return nil, errors.New("invalid BGE tokenization")
	}
	values := map[string][]int64{"input_ids": {}, "attention_mask": {}, "token_type_ids": {}}
	for i, id := range ids {
		values["input_ids"] = append(values["input_ids"], int64(id))
		values["attention_mask"] = append(values["attention_mask"], int64(mask[i]))
		values["token_type_ids"] = append(values["token_type_ids"], int64(types[i]))
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
			return nil, fmt.Errorf("unsupported BGE input %q", name)
		}
		tensor, err := ort.NewTensor(shape, input)
		if err != nil {
			return nil, err
		}
		inputs[i] = tensor
	}
	outputs := make([]ort.Value, len(b.outputNames))
	if err := b.session.Run(inputs, outputs); err != nil {
		return nil, err
	}
	defer func() {
		for _, output := range outputs {
			if output != nil {
				output.Destroy()
			}
		}
	}()
	tensor, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("unexpected BGE output %T", outputs[0])
	}
	dims := tensor.GetShape()
	if len(dims) != 3 || dims[0] != 1 || dims[2] != Dimensions {
		return nil, fmt.Errorf("unexpected BGE output shape %v", dims)
	}
	data := tensor.GetData()
	if len(data) < Dimensions {
		return nil, errors.New("short BGE output")
	}
	// BGE's model pooling configuration selects the raw CLS hidden state.
	vector := append([]float32(nil), data[:Dimensions]...)
	norm := float64(0)
	for _, value := range vector {
		norm += float64(value * value)
	}
	norm = math.Sqrt(norm)
	if norm == 0 || math.IsNaN(norm) || math.IsInf(norm, 0) {
		return nil, errors.New("invalid BGE vector norm")
	}
	for i := range vector {
		vector[i] /= float32(norm)
	}
	return vector, nil
}

func Dot(a, b []float32) (float32, error) {
	if len(a) != Dimensions || len(b) != Dimensions {
		return 0, errors.New("BGE vector dimension mismatch")
	}
	var score float32
	for i := range a {
		score += a[i] * b[i]
	}
	return score, nil
}
