package main

import (
	"log"

	proxyshared "gptproxy/shared"
)

func main() {
	app, err := proxyshared.NewApp(proxyshared.AppOptions{
		DefaultControlListen: "127.0.0.1:0",
	})
	if err != nil {
		log.Fatalf("create app: %v", err)
	}

	if err := app.Serve(handleIndex, true); err != nil {
		log.Fatalf("serve control panel: %v", err)
	}
}
