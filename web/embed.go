// Package web holds the browser UI. Its build output in dist/ is embedded in
// the hikari binary.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Assets returns the built UI, rooted at index.html.
func Assets() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}

// Built reports whether the UI was built before the binary was compiled.
func Built() bool {
	_, err := fs.Stat(Assets(), "index.html")
	return err == nil
}
