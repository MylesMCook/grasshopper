package client

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestBridgeAnswersAMalformedLineAndKeepsServing(t *testing.T) {
	_, config, _ := testClientServer(t)
	in := strings.NewReader("\n[{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"ping\"}]\n{\"jsonrpc\":\"2.0\",\"id\":7,\"method\":\"tools/list\"}\n")
	var out bytes.Buffer
	if err := Bridge(context.Background(), config, in, &out); err != nil {
		t.Fatalf("the bridge stopped on a malformed line: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want a parse error and the tools/list answer, got %d lines: %s", len(lines), out.String())
	}
	var first, second map[string]any
	_ = json.Unmarshal([]byte(lines[0]), &first)
	_ = json.Unmarshal([]byte(lines[1]), &second)
	if first["id"] != nil || first["error"] == nil {
		t.Fatalf("the batch line should get a parse error with id null: %s", lines[0])
	}
	if second["id"] != float64(7) {
		t.Fatalf("the next request was not served: %s", lines[1])
	}
}
