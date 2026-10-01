package service

import (
	"context"
	"encoding/json"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"strings"
	"testing"
)

// An agent sees where a saved memory applies and which memory_type values
// exist before it writes, and a wrong memory_type names the accepted values
// instead of failing opaquely.
func TestStoreGuidesScopeAndMemoryType(t *testing.T) {
	server, _ := testServer(t)
	session := clientSession(t, server, "2025-11-25")
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range listed.Tools {
		if tool.Name != "store" {
			continue
		}
		if !strings.Contains(tool.Description, "{} applies everywhere") || !strings.Contains(tool.Description, "only for facts about one machine") {
			t.Fatalf("store scope guidance missing: %s", tool.Description)
		}
		schema, _ := json.Marshal(tool.InputSchema)
		if !strings.Contains(string(schema), `"enum":["knowledge","identity","episode","procedure",null]`) {
			t.Fatalf("memory_type values missing: %s", schema)
		}
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "store", Arguments: map[string]any{
		"scope": map[string]any{}, "content": "Synthetic preference.", "memory_type": "preference", "purpose": "preference", "confirmed": true,
		"provenance": map[string]any{"harness": "test", "device": "synthetic", "source": "guidance test"}, "request_id": "wrong-memory-type",
	}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result.Content)
	if !result.IsError || !strings.Contains(string(encoded), "knowledge") {
		t.Fatalf("wrong memory_type did not name the accepted values: %s", encoded)
	}
}

func TestMissingMemoryGuidesSameScopeWithoutDisclosingOtherScopes(t *testing.T) {
	server, _ := testServer(t)
	session := clientSession(t, server, "2025-11-25")
	for _, id := range []int64{3, 999999} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get", Arguments: map[string]any{"scope": map[string]any{"project": "id:absent"}, "id": id}})
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(result.Content)
		if !result.IsError || !strings.Contains(string(encoded), "check the scope used for context") {
			t.Fatalf("missing scope guidance: %s", encoded)
		}
	}
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range listed.Tools {
		if tool.Name == "search" && !strings.Contains(tool.Description, "1 to 100") {
			t.Fatalf("search bounds missing: %s", tool.Description)
		}
	}
}
