package service

import (
	"net/http"
	"strings"
	"testing"
)

func TestVisualizerLinksToPublicSiteWithoutLocalWebsite(t *testing.T) {
	server, _ := testServer(t, true)
	response, page := visualizerRequest(t, http.MethodGet, server.URL+"/visualizer/", "")
	if response.StatusCode != http.StatusOK || !strings.Contains(string(page), `src="/visualizer/theme.js"`) || !strings.Contains(string(page), `id="app"`) {
		t.Fatal("memory view must load its app and its own theme asset")
	}
	// The page is drawn by app.js; Agents links the public setup guide and
	// offers the prompt that connects a new agent.
	_, app := visualizerRequest(t, http.MethodGet, server.URL+"/visualizer/app.js", "")
	if !strings.Contains(string(app), `https://usegrasshopper.com/setup/`) || !strings.Contains(string(app), `'Connect Grasshopper'`) || !strings.Contains(string(app), `/visualizer/api/pairings`) {
		t.Fatal("memory view needs a discoverable way to connect an agent")
	}
	response, theme := visualizerRequest(t, http.MethodGet, server.URL+"/visualizer/theme.js", "")
	if response.StatusCode != http.StatusOK || !strings.Contains(string(theme), "localStorage.setItem(themeKey, choice)") {
		t.Fatal("memory view theme switch is unavailable")
	}
	for _, path := range []string{"/about/", "/about/site.css", "/about/theme.js"} {
		response, _ := visualizerRequest(t, http.MethodGet, server.URL+path, testToken)
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("obsolete local page %s: status=%d", path, response.StatusCode)
		}
	}
}
