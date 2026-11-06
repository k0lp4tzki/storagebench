//go:build embedui

package main

import (
	"embed"
	"io/fs"
)

// embeddedUI holds the built frontend bundle. It is populated by the frontend
// build step (web/ -> cmd/controller/webdist) before compiling with the
// `embedui` tag. The webdist directory must contain index.html and assets.
//
//go:embed all:webdist
var embeddedUI embed.FS

// uiFS returns the embedded frontend rooted at webdist so paths map to "/".
func uiFS() fs.FS {
	sub, err := fs.Sub(embeddedUI, "webdist")
	if err != nil {
		return nil
	}
	return sub
}
