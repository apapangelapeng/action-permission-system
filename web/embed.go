// Package web embeds the built Svelte dashboard so the Go binary ships the UI.
// Run `npm run build` in web/ to regenerate dist/; the committed placeholder
// keeps the binary buildable before the real dashboard lands (milestone 2).
package web

import "embed"

//go:embed all:dist
var Dist embed.FS
