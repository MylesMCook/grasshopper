package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/MylesMCook/grasshopper/internal/memory"
)

func TestStartupPreviewMatchesAgentContextAndExplainsWhatIsLeftOut(t *testing.T) {
	server, store := testServer(t, true)
	ctx := context.Background()
	project := "id:startup"
	scope := memory.Scope{Project: &project}
	save := func(request, title, content, purpose string, confirmed bool) int64 {
		t.Helper()
		receipt, err := store.Write(ctx, memory.WriteInput{Scope: scope, Title: &title, Content: content, Purpose: purpose, Confirmed: confirmed,
			Provenance: memory.Provenance{Harness: "test", Device: "test", Source: "startup preview regression"}, RequestID: request}, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		return receipt.ID
	}
	loadedID := save("startup-pref", "Short answers", "Prefer short answers.", "preference", true)
	observation := save("startup-observation", "Maybe weekly", "Maybe send a weekly summary.", "observation", false)
	olderHandoff := save("startup-handoff-1", "Old handoff", "Earlier session note.", "handoff", false)
	newerHandoff := save("startup-handoff-2", "New handoff", "Latest session note.", "handoff", false)
	var heavy []int64
	for index := 0; index < 8; index++ {
		heavy = append(heavy, save("startup-heavy-"+jsonNumber(int64(index)), "Heavy "+jsonNumber(int64(index)), strings.Repeat("Detailed guidance. ", 60), "lesson", true))
	}

	for _, budget := range []int{12000, 3000} {
		status, data := ownerRead(t, server, "/visualizer/api/startup", `{"scope":{"project":"id:startup"},"budget":`+jsonNumber(int64(budget))+`}`)
		if status != http.StatusOK {
			t.Fatalf("budget %d status=%d body=%s", budget, status, data)
		}
		var preview memory.Startup
		if err := json.Unmarshal(data, &preview); err != nil {
			t.Fatal(err)
		}
		agent, err := store.Context(ctx, scope, budget)
		if err != nil {
			t.Fatal(err)
		}
		if len(preview.Records) != len(agent.Records) {
			t.Fatalf("budget %d: preview lists %d records but agents load %d", budget, len(preview.Records), len(agent.Records))
		}
		agentIDs := map[int64]bool{}
		for _, record := range agent.Records {
			agentIDs[record.ID] = true
		}
		for _, record := range preview.Records {
			if !agentIDs[record.ID] {
				t.Fatalf("budget %d: preview loads %d, agents do not", budget, record.ID)
			}
		}
		reasons := map[int64]string{}
		for _, item := range preview.NotLoaded {
			reasons[item.ID] = item.Reason
			if agentIDs[item.ID] {
				t.Fatalf("budget %d: %d is both loaded and not loaded", budget, item.ID)
			}
		}
		if reasons[observation] != memory.NotLoadedUnconfirmed || reasons[olderHandoff] != memory.NotLoadedOlderHandoff || !(agentIDs[newerHandoff] || reasons[newerHandoff] == memory.NotLoadedOverBudget) || !agentIDs[loadedID] {
			t.Fatalf("budget %d: wrong selection: loaded=%v reasons=%v", budget, agentIDs, reasons)
		}
		if budget == 3000 {
			over := 0
			for _, id := range heavy {
				if reasons[id] == memory.NotLoadedOverBudget {
					over++
				}
			}
			if over == 0 {
				t.Fatalf("a 3,000-byte budget must leave some heavy memories out: %v", reasons)
			}
		}
		if len(preview.Records) > 0 && len([]rune(preview.Records[len(preview.Records)-1].Content)) > 240 {
			t.Fatal("preview must carry list previews, not full text")
		}
	}
	for _, body := range []string{
		`{"scope":{"project":"id:startup"},"budget":5000}`,
		`{"scope":{"legacy":true},"budget":12000}`,
		`{"scope":{"platform":"darwin"},"budget":12000}`,
		`{"scope":{},"budget":12000,"extra":1}`,
		`{"scope":{"all_projects":true},"budget":12000}`,
		`{"scope":{"view":"review"},"budget":12000}`,
	} {
		if status, _ := ownerRead(t, server, "/visualizer/api/startup", body); status != http.StatusBadRequest {
			t.Fatalf("accepted %s: %d", body, status)
		}
	}
	anonymous, _ := http.NewRequest(http.MethodPost, server.URL+"/visualizer/api/startup", strings.NewReader(`{"scope":{},"budget":12000}`))
	anonymous.Header.Set("Origin", server.URL)
	response, err := http.DefaultClient.Do(anonymous)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous preview status=%d", response.StatusCode)
	}
}
