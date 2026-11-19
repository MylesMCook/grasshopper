package client

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
// Preserve record content, unknown record fields and unknown page fields, such
// as the areas list; omit redundant active-row metadata.
func compactContext(raw json.RawMessage) (json.RawMessage, error) {
	var page map[string]json.RawMessage
	if err := json.Unmarshal(raw, &page); err != nil {
		return nil, err
	}
	var records []map[string]json.RawMessage
	if err := json.Unmarshal(page["records"], &records); err != nil && page["records"] != nil {
		return nil, err
	}
	for _, record := range records {
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
	if page["records"] != nil {
		page["records"], _ = json.Marshal(records)
	}
	// Complete references already carry the same IDs plus revision and scope.
	var omittedIDs, omittedRecords []json.RawMessage
	_ = json.Unmarshal(page["omitted_ids"], &omittedIDs)
	_ = json.Unmarshal(page["omitted_records"], &omittedRecords)
	if len(omittedRecords) == len(omittedIDs) {
		delete(page, "omitted_ids")
	}
	return json.Marshal(page)
}
