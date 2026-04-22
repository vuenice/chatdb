package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// SPA returns the built Vue app filesystem rooted at dist/.
// It returns nil when the dist directory does not contain index.html
// (e.g. fresh checkout before the first frontend build), so the server
// can run as a JSON-only API.
func SPA() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil
	}
	return sub
}
