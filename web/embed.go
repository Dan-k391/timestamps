// Package web embeds the user interface.
package web

import "embed"

// FS holds the UI files.
//
//go:embed index.html app.css icon.png js
var FS embed.FS
