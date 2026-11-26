package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gogf/gf/v2/net/ghttp"
)

func bindWeb(server *ghttp.Server, root string) error {
	index := filepath.Join(root, "index.html")
	if _, err := os.Stat(index); err != nil {
		return fmt.Errorf("read frontend entry: %w", err)
	}
	assets := filepath.Join(root, "assets")
	server.BindHandler("GET:/", func(r *ghttp.Request) {
		r.Response.Header().Set("Cache-Control", "no-cache")
		r.Response.ServeFile(index)
	})
	server.BindHandler("GET:/assets/*asset", func(r *ghttp.Request) {
		name := r.GetRouter("asset").String()
		if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
			r.Response.WriteStatus(http.StatusNotFound)
			return
		}
		file := filepath.Join(assets, name)
		stat, err := os.Stat(file)
		if err != nil || !stat.Mode().IsRegular() {
			r.Response.WriteStatus(http.StatusNotFound)
			return
		}
		r.Response.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		r.Response.ServeFile(file)
	})
	return nil
}
