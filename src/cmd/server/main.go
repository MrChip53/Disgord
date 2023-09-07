package main

import (
	server "Disgord/src/internal/pkg/server"
	"log"
)

func main() {
	srv, err := server.New()
	if err != nil {
		log.Fatal(err)
	}
	err = srv.Run()
	if err != nil {
		log.Fatal(err)
	}
}
