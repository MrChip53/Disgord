package main

import (
	"Disgord/internal/pkg/auth"
	"Disgord/internal/pkg/server"
	"errors"
	"github.com/valyala/fasthttp"
	"html/template"
	"log"
	"strings"
)

func main() {
	srv, err := server.New("./internal/pkg/server/templates")
	if err != nil {
		log.Fatal(err)
	}

	srv.Use(func(template *template.Template, ctx *fasthttp.RequestCtx, next func() error) error {
		ctx.SetStatusCode(200)
		ctx.SetBody(nil)
		return nil
	})
	srv.Use(func(template *template.Template, ctx *fasthttp.RequestCtx, next func() error) error {
		path := string(ctx.Path())
		if strings.HasPrefix(path, "/assets") {
			if strings.HasSuffix(path, ".css") {
				ctx.Response.Header.SetContentType("text/css")
				ctx.SendFile("./public/" + path[8:])
				return nil
			} else if strings.HasSuffix(path, ".js") {
				ctx.Response.Header.SetContentType("text/javascript")
				ctx.SendFile("./public/" + path[8:])
				return nil
			}
		}

		return next()
	})
	srv.Use(func(template *template.Template, ctx *fasthttp.RequestCtx, next func() error) error {
		sessionToken := ctx.Request.Header.Cookie("SessionToken")
		if len(sessionToken) == 0 {
			refreshToken := ctx.Request.Header.Cookie("RefreshToken")
			if len(refreshToken) == 0 {
				return next()
			}

			refreshPayload, err := auth.VerifyRefreshToken(string(refreshToken))
			if err != nil {
				log.Printf("failed to verify refresh token: %s\n", err)
				return next()
			}

			// TODO fetch user info

			jwtPayload := &auth.JwtPayload{
				Username: "",
				Admin:    false,
				UserId:   refreshPayload.UserId,
			}

			sToken, rToken, err := auth.GenerateTokens(jwtPayload)
			if err != nil {
				log.Printf("failed to generate tokens: %s\n", err)
				return err
			}

			sCookie, rCookie := createTokenCookies(sToken, rToken)
			ctx.Response.Header.SetCookie(sCookie)
			ctx.Response.Header.SetCookie(rCookie)
			ctx.SetUserValue("token", jwtPayload)
		}

		jwtToken, err := auth.VerifyJwtToken(string(sessionToken))
		if err != nil {
			log.Printf("failed to verify jwt token: %s\n", err)
			return next()
		}

		ctx.SetUserValue("token", jwtToken)

		return next()
	})
	srv.Use(func(template *template.Template, ctx *fasthttp.RequestCtx, next func() error) error {
		if string(ctx.Path()) == "/stop" {
			ctx.SetBody([]byte("stopped"))
		} else if string(ctx.Path()) == "/stop-error" {
			return errors.New("failed")
		}
		return next()
	})

	srv.GET("/", func(template *template.Template, ctx *fasthttp.RequestCtx) error {
		dataMap := make(map[string]any)
		token := ctx.UserValue("token")
		dataMap["authed"] = token != nil
		dataMap["title"] = "Disgord"
		err := template.ExecuteTemplate(ctx, "indexPage", dataMap)
		if err != nil {
			log.Print(err)
			return err
		}
		return nil
	})
	srv.GET("/navbar", func(template *template.Template, ctx *fasthttp.RequestCtx) error {
		if ctx.UserValue("token") == nil {
			ctx.Redirect("/", 302)
			return nil
		}

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
	err = srv.Run(true)
	if err != nil {
		log.Fatal(err)
	}
}
