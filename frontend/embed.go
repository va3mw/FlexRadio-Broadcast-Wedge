// Package frontend holds the UI, shared by the desktop app and the web page
// the headless service serves.
package frontend

import "embed"

//go:embed all:dist
var Assets embed.FS
