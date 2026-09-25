package gomcp

import (
	"net/http"

	site "github.com/MylesMCook/grasshopper/docs"
)

func websiteAsset(w http.ResponseWriter, r *http.Request, styleHashes []string) bool {
	if r.URL.Path == "/" || r.URL.Path == "/about" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return true
		}
		http.Redirect(w, r, "/about/", http.StatusFound)
		return true
	}
	var filename, contentType string
	switch r.URL.Path {
	case "/about/":
		filename, contentType = "how-it-works.html", "text/html; charset=utf-8"
	case "/about/site.css":
		filename, contentType = "site.css", "text/css; charset=utf-8"
	case "/about/theme.js":
		filename, contentType = "theme.js", "text/javascript; charset=utf-8"
	case "/about/fonts/newsreader-latin.woff2":
		filename, contentType = "fonts/newsreader-latin.woff2", "font/woff2"
	case "/about/fonts/geist-mono-latin.woff2":
		filename, contentType = "fonts/geist-mono-latin.woff2", "font/woff2"
	default:
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", staticCSP(styleHashes, filename == "how-it-works.html"))
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return true
	}
	contents, err := site.Files.ReadFile(filename)
	if err != nil {
		http.Error(w, "website unavailable", http.StatusInternalServerError)
		return true
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(contents)
	return true
}
