package main

import (
	server "Disgord/src/internal/pkg/server"
	"github.com/valyala/fasthttp"
	"log"
)

func main() {
	srv, err := server.New()
	if err != nil {
		log.Fatal(err)
	}
	srv.AddRoute("/hp", func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(200)
		ctx.SetBody(nil)
	})
	err = srv.Run()
	if err != nil {
		log.Fatal(err)
	}
}
