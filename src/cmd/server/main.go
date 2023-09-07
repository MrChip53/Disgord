package main

import (
	server "Disgord/src/internal/pkg/server"
	"errors"
	"github.com/valyala/fasthttp"
	"html/template"
	"log"
)

func main() {
	srv, err := server.New("./src/internal/pkg/server/templates")
	if err != nil {
		log.Fatal(err)
	}
	srv.AddRoute("/hp", func(template *template.Template, ctx *fasthttp.RequestCtx) error {
		ctx.SetStatusCode(200)
		ctx.SetBody(nil)
		return nil
	})
	srv.AddRoute("/", func(template *template.Template, ctx *fasthttp.RequestCtx) error {
		err := template.ExecuteTemplate(ctx, "indexPage", nil)
		if err != nil {
			log.Print(err)
			return err
		}
		return nil
	})
	srv.AddRoute("/navbar", func(template *template.Template, ctx *fasthttp.RequestCtx) error {
		err := template.ExecuteTemplate(ctx, "navbar", nil)
		if err != nil {
			log.Print(err)
			return err
		}
		return nil
	})
	srv.AddRoute("/error", func(template *template.Template, ctx *fasthttp.RequestCtx) error {
		return errors.New("failed to do this")
	})
	err = srv.Run()
	if err != nil {
		log.Fatal(err)
	}
}
