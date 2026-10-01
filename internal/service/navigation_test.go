package service

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestBrowserServerNavigation(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		server, _ := testServer(t, enabled)
		client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		for _, path := range []string{"/", "/mcp"} {
			req, _ := http.NewRequest(http.MethodGet, server.URL+path, nil)
			req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*;q=0.8")
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if enabled {
				if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/visualizer/" {
					t.Fatalf("enabled %s: status=%d location=%q", path, resp.StatusCode, resp.Header.Get("Location"))
				}
			} else if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "Memory view is unavailable") || !strings.Contains(string(body), "--visualizer") || strings.Contains(string(body), `href="/visualizer/"`) {
				t.Fatalf("disabled %s: status=%d body=%s", path, resp.StatusCode, body)
			}
		}
		for _, path := range []string{"/favicon.ico", "/visualizer/missing.js", "/visualizer/missing/"} {
			resp, _ := visualizerRequest(t, http.MethodGet, server.URL+path, "")
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("unknown %s enabled=%t: %d", path, enabled, resp.StatusCode)
			}
		}
		for _, accept := range []string{"", "application/json", "text/event-stream", "text/html;q=0", "text/html, text/event-stream"} {
			req, _ := http.NewRequest(http.MethodGet, server.URL+"/mcp", nil)
			req.Header.Set("Accept", accept)
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("agent GET Accept=%q: %d", accept, resp.StatusCode)
			}
		}
		for _, method := range []string{http.MethodPost, http.MethodDelete, http.MethodHead} {
			req, _ := http.NewRequest(method, server.URL+"/mcp", nil)
			req.Header.Set("Accept", "text/html")
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("agent %s: %d", method, resp.StatusCode)
			}
		}
		resp, _ := visualizerRequest(t, http.MethodPost, server.URL+"/visualizer/api/context", "", "{}")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("owner API enabled=%t: %d", enabled, resp.StatusCode)
		}
	}
}
