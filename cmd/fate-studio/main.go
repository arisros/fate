package main

import (
	"log"
	"os"

	"github.com/arisros/fate/internal/demos"
	"github.com/arisros/fate/studio"
)

func main() {
	addr := os.Getenv("FATE_STUDIO_ADDR")
	if addr == "" {
		addr = ":8090"
	}

	srv := studio.NewServer("fate studio")
	for _, d := range demos.All() {
		srv.Register(d.Entry())
	}

	log.Printf("fate-studio listening on %s", addr)
	if err := srv.ListenAndServe(addr); err != nil {
		log.Fatalf("ListenAndServe: %v", err)
	}
}
