package main

import "log"

func main() {
	app, err := newApp()
	if err != nil {
		log.Fatalf("create app: %v", err)
	}

	log.Printf("control panel listening on %s", app.controlAddr())
	if err := app.serve(); err != nil {
		log.Fatalf("serve control panel: %v", err)
	}
}
