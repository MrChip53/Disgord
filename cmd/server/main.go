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
	srv.GET("/hp", func(template *template.Template, ctx *fasthttp.RequestCtx) error {
		ctx.SetStatusCode(200)
		ctx.SetBody(nil)
		return nil
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
	srv.GET("/assets/*", func(template *template.Template, ctx *fasthttp.RequestCtx) error {
		file := string(ctx.Path())[8:]
		if strings.HasSuffix(file, ".css") {
			ctx.Response.Header.Set("Content-Type", "text/css")
		} else if strings.HasSuffix(file, ".js") {
			ctx.Response.Header.Set("Content-Type", "text/javascript")
		}
		ctx.SendFile("./public/" + file)
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
		sessionToken := ctx.Request.Header.Cookie("SessionToken")
		if len(sessionToken) == 0 {
			refreshToken := ctx.Request.Header.Cookie("RefreshToken")
			if len(refreshToken) == 0 {
				return true, nil
			}

			refreshPayload, err := auth.VerifyRefreshToken(string(refreshToken))
			if err != nil {
				return true, err
			}

			// TODO fetch user info

			jwtPayload := &auth.JwtPayload{
				Username: "",
				Admin:    false,
				UserId:   refreshPayload.UserId,
			}

			sToken, rToken, err := auth.GenerateTokens(jwtPayload)
			if err != nil {
				return true, err
			}

			sCookie, rCookie := createTokenCookies(sToken, rToken)
			ctx.Response.Header.SetCookie(sCookie)
			ctx.Response.Header.SetCookie(rCookie)
			ctx.SetUserValue("token", jwtPayload)
		}

		jwtToken, err := auth.VerifyJwtToken(string(sessionToken))
		if err != nil {
			return true, nil
		}

		ctx.SetUserValue("token", jwtToken)

		return true, nil
	})
	srv.Use(func(template *template.Template, ctx *fasthttp.RequestCtx) (bool, error) {
		path := string(ctx.Path())
		if path != "/" && path != "/favicon.ico" &&
			path != "/hp" && !strings.HasPrefix(path, "/assets/") &&
			ctx.UserValue("token") == nil {
			ctx.Redirect("/", 302)
			return false, nil
		}
		return true, nil
	})
	srv.Use(func(template *template.Template, ctx *fasthttp.RequestCtx) (bool, error) {
		if string(ctx.Path()) == "/stop" {
			ctx.SetBody([]byte("stopped"))
			return false, nil
		} else if string(ctx.Path()) == "/stop-error" {
			return false, errors.New("failed")
		}
		return true, nil
	})
	err = srv.Run(true)
	if err != nil {
		log.Fatal(err)
	}
}
