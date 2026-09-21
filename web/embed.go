// Package web embeds the compiled Svelte app. Run `npm run build` in this
// directory (or let the Dockerfile do it) to populate dist/.
package web

import "embed"

//go:embed all:dist
var Dist embed.FS
