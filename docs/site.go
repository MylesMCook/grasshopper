// Package site embeds Grasshopper's plain-language website for self-hosted use.
package site

import "embed"

// Files contains the website and its local fonts.
//
//go:embed how-it-works.html site.css theme.js fonts/newsreader-latin.woff2 fonts/geist-mono-latin.woff2
var Files embed.FS
