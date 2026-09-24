package gomcp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestMCPThroughPrivateHTTPSProxyHost(t *testing.T) {
	fixtureServer, store := testServer(t)
	fixtureServer.Close()
	const proxyHost = "memory.example.test:8456"
	handler, err := NewHandler(Backend{Store: store, AllowedProxyHost: proxyHost}, testToken)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"context","arguments":{"scope":{}},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientInfo":{"name":"tailnet-test","version":"1"},"io.modelcontextprotocol/clientCapabilities":{}}}}`
	call := func(host, origin string, authenticated bool) (int, string) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, server.URL+"/mcp", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Host = host
		if authenticated {
			req.Header.Set("Authorization", "Bearer "+testToken)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("MCP-Protocol-Version", "2026-07-28")
		req.Header.Set("Mcp-Method", "tools/call")
		req.Header.Set("Mcp-Name", "context")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, string(data)
	}
	for _, origin := range []string{"", "https://" + proxyHost} {
		status, body := call(proxyHost, origin, true)
		if status != http.StatusOK || !strings.Contains(body, "structuredContent") {
			t.Fatalf("private proxy Host and Origin %q: status=%d body=%q", origin, status, body)
		}
	}
	for _, tc := range []struct {
		host, origin string
		auth         bool
		status       int
	}{
		{"untrusted.example:8456", "", true, http.StatusForbidden},
		{proxyHost, "https://untrusted.example", true, http.StatusForbidden},
		{proxyHost, "", false, http.StatusUnauthorized},
	} {
		status, _ := call(tc.host, tc.origin, tc.auth)
		if status != tc.status {
			t.Fatalf("Host %q Origin %q auth=%t: got %d, want %d", tc.host, tc.origin, tc.auth, status, tc.status)
		}
	}
	localHost := strings.TrimPrefix(server.URL, "http://")
	if status, _ := call(localHost, "http://"+localHost, true); status != http.StatusOK {
		t.Fatalf("local MCP request returned %d, want 200", status)
	}
}

func TestRejectMalformedProxyHost(t *testing.T) {
	_, store := testServer(t)
	for _, host := range []string{"https://example.com:8456", "example.com", "example.com:0", "example.com:99999", "example.com:8456/path", "example\n.com:8456"} {
		if _, err := NewHandler(Backend{Store: store, AllowedProxyHost: host}, testToken); err == nil {
			t.Fatalf("accepted invalid proxy Host %q", host)
		}
	}
}
