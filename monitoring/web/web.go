// Package web includes the static web pages for the monitoring tool.
package web

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"runtime"
	"strings"
)

//go:embed dist/*
var staticAssets embed.FS

// GetAssets returns the static assets
func GetAssets() http.FileSystem {
	if isDevelopmentMode() {
		_, assetPath, _, ok := runtime.Caller(1)
		if !ok {
			panic("error getting path")
		}

		assetPath = path.Join(path.Dir(assetPath), "/dist")

		fmt.Printf("In monitoring tool development mode, serving assets from %s\n", assetPath)

		return http.Dir(assetPath)
	}

	subFS, err := fs.Sub(staticAssets, "dist")
	if err != nil {
		panic(err)
	}

	return http.FS(subFS)
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
