package gomcp

import (
	"net/http"
	"strings"
	"testing"
)

func TestWebsiteLinksToVisualizerWithoutLeakingMemory(t *testing.T) {
	server, _ := testServer(t, true)
	for _, path := range []string{"/about/", "/about/site.css", "/about/theme.js", "/about/fonts/newsreader-latin.woff2", "/about/fonts/geist-mono-latin.woff2"} {
		response, data := visualizerRequest(t, http.MethodGet, server.URL+path, "")
		if response.StatusCode != http.StatusOK || len(data) == 0 || response.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("website asset %s: status=%d bytes=%d", path, response.StatusCode, len(data))
		}
		if path == "/about/" {
			page := string(data)
			if !strings.Contains(page, "href=\"/visualizer/\"") || strings.Contains(page, "answer-style") {
				t.Fatal("website did not link to memory view or leaked a record")
			}
			if csp := response.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") || strings.Contains(csp, "unsafe-inline") {
				t.Fatalf("website document CSP: %q", csp)
			}
		}
		if path == "/about/theme.js" && (!strings.Contains(string(data), "localStorage.setItem(themeKey, next)") || !strings.Contains(string(data), "location.protocol === 'file:'")) {
			t.Fatal("website and memory view must share the theme preference")
		}
	}
	response, data := visualizerRequest(t, http.MethodGet, server.URL+"/visualizer/", "")
	if response.StatusCode != http.StatusOK || !strings.Contains(string(data), "href=\"/about/\"") || !strings.Contains(string(data), "src=\"/about/theme.js\"") {
		t.Fatal("memory view did not link back to website")
	}
	response, _ = visualizerRequest(t, http.MethodPost, server.URL+"/about/", "")
	if response.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("website accepted POST: %d", response.StatusCode)
	}
}
