package service

import (
	"mime"
	"net/http"
	"strconv"
	"strings"
)

// Browser navigation is public and contains no memory or connection state.
// Explicit SSE requests remain on the authenticated MCP transport.
func acceptsBrowserHTML(accept string) bool {
	html := false
	for _, item := range strings.Split(accept, ",") {
		kind, params, err := mime.ParseMediaType(strings.TrimSpace(item))
		if err != nil {
			continue
		}
		if kind == "text/event-stream" {
			return false
		}
		quality := 1.0
		if value, present := params["q"]; present {
			quality, err = strconv.ParseFloat(value, 64)
			if err != nil {
				continue
			}
		}
		if kind == "text/html" && quality > 0 && quality <= 1 {
			html = true
		}
	}
	return html
}

func serverNavigation(w http.ResponseWriter, r *http.Request, enabled bool) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if enabled {
		http.Redirect(w, r, "/visualizer/", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Grasshopper server</title><main><h1>Grasshopper server</h1><p>Memory view is unavailable on this server. The server operator can enable it with the <code>--visualizer</code> option.</p><p>Agents connect through the authenticated <code>/mcp</code> endpoint.</p></main></html>`))
}
