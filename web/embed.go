package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist
var assets embed.FS

func Handler() http.Handler {
	root, e := fs.Sub(assets, "dist")
	if e != nil {
		panic(e)
	}
	return http.FileServer(http.FS(root))
}
