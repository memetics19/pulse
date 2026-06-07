package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFiles embed.FS

// StaticAssets serves the exported Next.js admin SPA and its assets.
// The export uses absolute paths (/admin/..., /_next/...), so files are
// served at their natural paths with no prefix stripping. Mount it on the
// /admin/* and /_next/* routes.
//
// Content-hashed build assets under /_next/static/ are cached immutably;
// everything else (the HTML entry points) is served no-cache so a new build
// is always picked up instead of a stale cached page.
func StaticAssets() http.Handler {
	sub, err := fs.Sub(distFiles, "dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/_next/static/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	})
}
