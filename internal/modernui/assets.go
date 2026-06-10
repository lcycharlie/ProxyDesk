package modernui

import (
	"embed"
	"io/fs"
)

// Assets contains the modern WebView UI shell.
//
//go:embed dist
var Assets embed.FS

func FS() fs.FS {
	dist, err := fs.Sub(Assets, "dist")
	if err != nil {
		panic(err)
	}
	return dist
}
