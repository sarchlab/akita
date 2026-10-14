// Package sqlitepath encodes literal filenames for both SQLite drivers.
package sqlitepath

import (
	"net/url"
	"path/filepath"
	"strings"
)

// URI preserves filename characters that a raw DSN would interpret as options.
// Mode is a SQLite open mode: ro, rw, or rwc.
func URI(filename, mode string) string {
	absolute, err := filepath.Abs(filename)
	if err != nil {
		panic(err)
	}
	return fileURI(filepath.ToSlash(absolute), mode)
}

func fileURI(absolute, mode string) string {
	// Drive-letter paths need a leading slash so the drive is a path segment,
	// not the authority in file://C:/... . Unix and UNC paths already have it.
	if !strings.HasPrefix(absolute, "/") {
		absolute = "/" + absolute
	}
	return (&url.URL{Scheme: "file", Path: absolute, RawQuery: "mode=" + mode}).String()
}
