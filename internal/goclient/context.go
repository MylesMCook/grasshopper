package goclient

import "encoding/json"

func compactScope(scope Scope) map[string]any {
	result := map[string]any{}
	if scope.Project != nil {
		result["project"] = *scope.Project
	}
	if scope.Device != nil {
		result["device"] = *scope.Device
	}
	if scope.Platform != nil {
		result["platform"] = *scope.Platform
	}
	if scope.Legacy {
		result["legacy"] = true
	}
	return result
}

// compactContext changes hook presentation only. MCP records remain unchanged.
// Preserve record content and unknown record fields; omit redundant active-row metadata.
func compactContext(raw json.RawMessage) (json.RawMessage, error) {
	var page struct {
		Records        []map[string]json.RawMessage `json:"records"`
		Omitted        int                          `json:"omitted"`
		OmittedIDs     []int64                      `json:"omitted_ids,omitempty"`
		OmittedRecords []json.RawMessage            `json:"omitted_records,omitempty"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		return nil, err
	}
	for _, record := range page.Records {
		if string(record["archived"]) == "false" {
			delete(record, "archived")
		}
		if string(record["tags"]) == `""` {
			delete(record, "tags")
		}
		// updated_at is the content revision time; created_at adds no freshness signal.
		if len(record["updated_at"]) > 0 {
			delete(record, "created_at")
		}
		var scope Scope
		if err := json.Unmarshal(record["scope"], &scope); err != nil {
			return nil, err
		}
		record["scope"], _ = json.Marshal(compactScope(scope))
	}
	// Complete references already carry the same IDs plus revision and scope.
	if len(page.OmittedRecords) == len(page.OmittedIDs) {
		page.OmittedIDs = nil
	}
	return json.Marshal(page)
}
