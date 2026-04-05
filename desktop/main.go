package main

import (
	"log"
)

func main() {
	if err := runDesktop(); err != nil {
		log.Fatalf("run desktop: %v", err)
	}
}
