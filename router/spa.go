package router

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// MountSPA serves the built React app from the given filesystem (typically an
// embed.FS rooted at web/dist). API and /health routes are already registered,
// so this only handles everything else: it serves static assets when they exist
// and falls back to index.html for client-side routes (history API).
//
// If dist is nil/empty (no frontend built), it is a no-op so the API still runs.
func MountSPA(r *gin.Engine, dist fs.FS) {
	if dist == nil {
		return
	}
	indexHTML, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		// No build present; skip mounting.
		return
	}
	fileServer := http.FileServer(http.FS(dist))

	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		// Never let the SPA shadow the API.
		if strings.HasPrefix(p, "/api/") || p == "/health" {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "not found"})
			return
		}
		// Serve a real static asset if it exists.
		if p != "/" {
			if f, err := dist.Open(strings.TrimPrefix(p, "/")); err == nil {
				_ = f.Close()
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}
		}
		// Otherwise return the SPA shell.
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
	})
}
