package main

import (
	"Disgord/internal/pkg/auth"
	"Disgord/internal/pkg/server"
	"errors"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/valyala/fasthttp"
	"html/template"
	"io/fs"
	"log"
	"path/filepath"
	"strings"
	"time"
)

var templates *template.Template

func parseTemplates(directory string, funcMap interface{}) *template.Template {
	return template.Must(template.ParseGlob(directory + "/*.html"))
}

func watchTemplates(rootDir string) {
	cleanRoot := filepath.Clean(rootDir)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Println("failed to start template watcher")
	}
	defer watcher.Close()
	err = filepath.WalkDir(cleanRoot, func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			return watcher.Add(path)
		}
		return nil
	})
	if err != nil {
		return
	}
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				fmt.Println("Reloading templates: ", event.Name, " reloading templates")
				templates = parseTemplates(rootDir, nil)
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Println("Error:", err)
		}
	}
}

func main() {
	templates := parseTemplates("./cmd/server/templates", nil)
	go watchTemplates("./cmd/server/templates")

	srv, err := server.New("./cmd/server/templates")
	if err != nil {
		log.Fatal(err)
	}

	srv.Use(func(ctx *fasthttp.RequestCtx, next func() error) error {
		start := time.Now().UnixNano()
		ctx.Response.Header.SetContentType("text/html")
		err := next()
		if err != nil {
			ctx.SetStatusCode(500)
			err := templates.ExecuteTemplate(ctx, "505Page", nil)
			if err != nil {
				ctx.SetStatusCode(500)
				ctx.SetBody([]byte("500 Internal Server Error"))
			}
			return err
		}
		log.Println("error running router: ", err)
		end := time.Now().UnixNano()
		log.Printf("Request for %s took: %f ms\n", ctx.Path(), float64(end-start)/1000000.0)
		return nil
	})
	srv.Use(func(ctx *fasthttp.RequestCtx, next func() error) error {
		ctx.SetStatusCode(200)
		ctx.SetBody(nil)
		return nil
	})
	srv.Use(func(ctx *fasthttp.RequestCtx, next func() error) error {
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
	srv.Use(func(ctx *fasthttp.RequestCtx, next func() error) error {
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
	srv.Use(func(ctx *fasthttp.RequestCtx, next func() error) error {
		if string(ctx.Path()) == "/stop" {
			ctx.SetBody([]byte("stopped"))
		} else if string(ctx.Path()) == "/stop-error" {
			return errors.New("failed")
		}
		return next()
	})

	srv.GET("/", func(ctx *fasthttp.RequestCtx) error {
		dataMap := make(map[string]any)
		token := ctx.UserValue("token")
		dataMap["authed"] = token != nil
		dataMap["title"] = "Disgord"
		err := templates.ExecuteTemplate(ctx, "indexPage", dataMap)
		if err != nil {
			log.Print(err)
			return err
		}
		return nil
	})
	srv.GET("/navbar", func(ctx *fasthttp.RequestCtx) error {
		if ctx.UserValue("token") == nil {
			ctx.Redirect("/", 302)
			return nil
		}

		err := templates.ExecuteTemplate(ctx, "navbar", nil)
		if err != nil {
			log.Print(err)
			return err
		}
		return nil
	})
	srv.GET("/error", func(ctx *fasthttp.RequestCtx) error {
		return errors.New("failed to do this")
	})

	srv.SetErrorTemplate(404, "404Page")
	err = srv.Run()
	if err != nil {
		log.Fatal(err)
	}
}
