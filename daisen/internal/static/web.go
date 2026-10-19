// Package static includes the static web pages for Daisen.
package static

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist/*
var staticAssets embed.FS

// Assets returns the static assets
func Assets() http.FileSystem {
	subFS, err := fs.Sub(staticAssets, "dist")
	if err != nil {
		panic(err)
	}

	return http.FS(subFS)
}
