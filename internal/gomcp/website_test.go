package gomcp

import (
	"net/http"
	"strings"
	"testing"
)

func TestVisualizerLinksToPublicSiteWithoutLocalWebsite(t *testing.T) {
	server, _ := testServer(t, true)
	response, page := visualizerRequest(t, http.MethodGet, server.URL+"/visualizer/", "")
	if response.StatusCode != http.StatusOK || !strings.Contains(string(page), `href="https://usegrasshopper.com/"`) || !strings.Contains(string(page), `src="/visualizer/theme.js"`) {
		t.Fatal("memory view must link to the public site and load its own theme asset")
	}
	if !strings.Contains(string(page), `id="device-panel"`) || !strings.Contains(string(page), `id="server-address"`) {
		t.Fatal("memory view needs a discoverable device connection panel")
	}
	response, theme := visualizerRequest(t, http.MethodGet, server.URL+"/visualizer/theme.js", "")
	if response.StatusCode != http.StatusOK || !strings.Contains(string(theme), "localStorage.setItem(themeKey, next)") {
		t.Fatal("memory view theme switch is unavailable")
	}
	for _, path := range []string{"/about/", "/about/site.css", "/about/theme.js"} {
		response, _ := visualizerRequest(t, http.MethodGet, server.URL+path, testToken)
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("obsolete local page %s: status=%d", path, response.StatusCode)
		}
	}
}
