package static

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist
var assets embed.FS

// Assets returns the static assets.
func Assets() http.FileSystem {
	sub, err := fs.Sub(assets, "dist")
	if err != nil {
		panic(err)
	}

	return http.FS(sub)
}
