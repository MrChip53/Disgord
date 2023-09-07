package main

import (
	server "Disgord/src/internal/pkg/server"
	"github.com/valyala/fasthttp"
	"html/template"
	"log"
)

func main() {
	srv, err := server.New("./src/internal/pkg/server/templates")
	if err != nil {
		log.Fatal(err)
	}
	srv.AddRoute("/hp", func(template *template.Template, ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(200)
		ctx.SetBody(nil)
	})
	srv.AddRoute("/", func(template *template.Template, ctx *fasthttp.RequestCtx) {
		template.ExecuteTemplate(ctx, "pages/index", nil)
	})
	err = srv.Run()
	if err != nil {
		log.Fatal(err)
	}
}
