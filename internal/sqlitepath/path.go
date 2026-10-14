// Package sqlitepath encodes literal filenames for both SQLite drivers.
package sqlitepath

import (
	"net/url"
	"path/filepath"
)

// URI preserves filename characters that a raw DSN would interpret as options.
// Mode is a SQLite open mode: ro, rw, or rwc.
func URI(filename, mode string) string {
	absolute, err := filepath.Abs(filename)
	if err != nil {
		panic(err)
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(absolute), RawQuery: "mode=" + mode}).String()
}
