// Package extension embeds the optional browser extension so the app can
// unpack it for "Load unpacked" installs.
package extension

import "embed"

// FS holds the extension files.
//
//go:embed manifest.json background.js icon16.png icon48.png icon128.png firefox
var FS embed.FS
