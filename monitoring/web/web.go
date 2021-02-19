// Package web includes the static web pages for the monitoring tool.
package web

import "embed"

//go:embed index.html dist/bundle.js
// Assets are the static web contents.
var Assets embed.FS
