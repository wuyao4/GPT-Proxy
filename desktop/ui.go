package main

import (
	"embed"
	"net/http"
)

//go:embed ui/index.html
var uiFiles embed.FS

func (a *app) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	page, err := uiFiles.ReadFile("ui/index.html")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load ui")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(page)
}
