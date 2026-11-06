//go:build !embedui

package main

import "io/fs"

// uiFS returns nil when the UI is not embedded. The server then serves a small
// placeholder page. This is the default build so `go build ./...` works without a
// prebuilt frontend bundle. Production builds use `-tags embedui` after the
// frontend has been built into cmd/controller/webdist (see Makefile / CI).
func uiFS() fs.FS { return nil }
