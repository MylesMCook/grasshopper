package gomcp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func TestModernDiscoveryAndStatelessCalls(t *testing.T) {
	server, _ := testServer(t)
	metadata := map[string]any{
		"io.modelcontextprotocol/protocolVersion":    "2026-07-28",
		"io.modelcontextprotocol/clientInfo":         map[string]any{"name": "grasshopper-test", "version": "1.0"},
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
	}
	request := func(method, name string, params map[string]any) map[string]any {
		t.Helper()
		params["_meta"] = metadata
		body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(http.MethodPost, server.URL+"/mcp", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+testToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("MCP-Protocol-Version", "2026-07-28")
		req.Header.Set("Mcp-Method", method)
		if name != "" {
			req.Header.Set("Mcp-Name", name)
		}
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		raw, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("%s status %d: %s", method, response.StatusCode, raw)
		}
		if strings.HasPrefix(string(raw), "event:") {
			var data []string
			for _, line := range strings.Split(string(raw), "\n") {
				if strings.HasPrefix(line, "data:") {
					data = append(data, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
				}
			}
			raw = []byte(strings.Join(data, "\n"))
		}
		var rpc map[string]any
		if err := json.Unmarshal(raw, &rpc); err != nil {
			t.Fatalf("%s invalid JSON: %s: %v", method, raw, err)
		}
		if rpc["error"] != nil {
			t.Fatalf("%s RPC error: %v", method, rpc["error"])
		}
		result, ok := rpc["result"].(map[string]any)
		if !ok || result["resultType"] != "complete" {
			t.Fatalf("%s missing modern complete result: %v", method, rpc)
		}
		return result
	}
	discovered := request("server/discover", "", map[string]any{})
	versions, ok := discovered["supportedVersions"].([]any)
	if !ok || !slices.Contains(versions, any("2026-07-28")) {
		t.Fatalf("modern version not advertised: %v", discovered)
	}
	listed := request("tools/list", "", map[string]any{})
	tools, ok := listed["tools"].([]any)
	if !ok || len(tools) != 5 {
		t.Fatalf("modern tool list: %v", listed)
	}
	called := request("tools/call", "context", map[string]any{"name": "context", "arguments": map[string]any{"scope": map[string]any{}}})
	if called["structuredContent"] == nil || called["content"] == nil {
		t.Fatalf("modern context result incomplete: %v", called)
	}
}
