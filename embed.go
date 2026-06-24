package main

import (
	"embed"
	"io/fs"
)

// webDist embeds the built frontend. The directory must exist at build time;
// a .gitkeep keeps it present even before the first `npm run build`.
//
//go:embed all:web/dist
var webDist embed.FS

// distFS returns the embedded frontend rooted at web/dist, or nil if no real
// build is present (only the placeholder), so the server still runs API-only.
func distFS() fs.FS {
	sub, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		return nil
	}
	// If index.html is absent, treat as "no frontend built".
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil
	}
	return sub
}
