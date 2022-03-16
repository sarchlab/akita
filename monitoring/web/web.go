// Package web includes the static web pages for the monitoring tool.
package web

import (
	"embed"
	"fmt"
	"net/http"
	"os"
	"path"
	"runtime"
	"strings"
)

//go:embed index.html dist/bundle.js
var staticAssets embed.FS

// GetAssets returns the static assets
func GetAssets() http.FileSystem {
	if isDevelopmentMode() {
		_, assestPath, _, ok := runtime.Caller(1)
		if !ok {
			panic("error getting path")
		}

		assestPath = path.Join(path.Dir(assestPath), "/web")

		// path := path.Join(path.Dir(filename), "../config/settings.toml")

		fmt.Printf("In monitoring tool development mode, serving assets from %s\n", assestPath)

		return http.Dir(assestPath)
	}

	return http.FS(staticAssets)
}

// isDevelopmentMode returns true if environment variable AKITA_MONITOR_DEV is
// set.
func isDevelopmentMode() bool {
	evName := "AKITA_MONITOR_DEV"
	evValue, exist := os.LookupEnv(evName)

	if !exist {
		return false
	}

	if strings.ToLower(evValue) == "true" || evValue == "1" {
		return true
	}

	return false
}
