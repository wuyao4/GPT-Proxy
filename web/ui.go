package main

import (
	"net/http"

	"gptproxy/shared/controlui"
)

func handleIndex(w http.ResponseWriter, r *http.Request) {
	controlui.HandleIndex(w, r)
}
