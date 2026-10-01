package service

import (
	"context"
	"encoding/json"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"strings"
	"testing"
)

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
