package main

import (
	"Disgord/internal/pkg/server"
	"errors"
	"github.com/valyala/fasthttp"
	"html/template"
	"log"
)

func main() {
	srv, err := server.New("./internal/pkg/server/templates")
	if err != nil {
		log.Fatal(err)
	}
	srv.GET("/hp", func(template *template.Template, ctx *fasthttp.RequestCtx) error {
		ctx.SetStatusCode(200)
		ctx.SetBody(nil)
		return nil
	})
	srv.GET("/", func(template *template.Template, ctx *fasthttp.RequestCtx) error {
		err := template.ExecuteTemplate(ctx, "indexPage", nil)
		if err != nil {
			log.Print(err)
			return err
		}
		return nil
	})
	srv.GET("/navbar", func(template *template.Template, ctx *fasthttp.RequestCtx) error {
		err := template.ExecuteTemplate(ctx, "navbar", nil)
		if err != nil {
			log.Print(err)
			return err
		}
		return nil
	})
	srv.GET("/error", func(template *template.Template, ctx *fasthttp.RequestCtx) error {
		return errors.New("failed to do this")
	})
	srv.AddErrorTemplate(404, "404Page")
	srv.AddErrorTemplate(500, "500Page")
	srv.Use(func(template *template.Template, ctx *fasthttp.RequestCtx) (bool, error) {
		if string(ctx.Path()) == "/stop" {
			ctx.SetBody([]byte("stopped"))
			return false, nil
		} else if string(ctx.Path()) == "/stop-error" {
			return false, errors.New("failed")
		}
		return true, nil
	})
	err = srv.Run()
	if err != nil {
		log.Fatal(err)
	}
}
