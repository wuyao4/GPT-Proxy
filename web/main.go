package main

import "log"

func main() {
	app, err := newApp()
	if err != nil {
		log.Fatalf("create app: %v", err)
	}

	if err := app.serve(); err != nil {
		log.Fatalf("serve control panel: %v", err)
	}
}
