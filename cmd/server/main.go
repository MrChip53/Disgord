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
)

type User struct {
	username string
}

var templates *template.Template
var users map[string]User

func parseTemplates(directory string, funcMap template.FuncMap) *template.Template {
	return template.Must(template.New("").Funcs(funcMap).ParseGlob(directory + "/*.html"))
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
	users = make(map[string]User)
	templates = parseTemplates("./cmd/server/templates", nil)
	go watchTemplates("./cmd/server/templates")

	srv, err := server.New(templates)
	if err != nil {
		log.Fatal(err)
	}

	srv.Use(func(ctx *fasthttp.RequestCtx, next func()) {
		path := string(ctx.Path())
		if strings.HasPrefix(path, "/assets") {
			if strings.HasSuffix(path, ".css") {
				ctx.Response.Header.SetContentType("text/css")
				ctx.SendFile("./public/" + path[8:])
				return
			} else if strings.HasSuffix(path, ".js") {
				ctx.Response.Header.SetContentType("text/javascript")
				ctx.SendFile("./public/" + path[8:])
				return
			}
		}
		ctx.Response.Header.SetContentType("text/html")
		next()
	})
	srv.Use(func(ctx *fasthttp.RequestCtx, next func()) {
		sessionToken := ctx.Request.Header.Cookie("SessionToken")
		if len(sessionToken) == 0 {
			refreshToken := ctx.Request.Header.Cookie("RefreshToken")
			if len(refreshToken) == 0 {
				next()
				return
			}

			refreshPayload, err := auth.VerifyRefreshToken(string(refreshToken))
			if err != nil {
				log.Printf("failed to verify refresh token: %s\n", err)
				next()
				return
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
				next()
				return
			}

			sCookie, rCookie := createTokenCookies(sToken, rToken, false)
			ctx.Response.Header.SetCookie(sCookie)
			ctx.Response.Header.SetCookie(rCookie)
			ctx.SetUserValue("token", jwtPayload)
		}

		jwtToken, err := auth.VerifyJwtToken(string(sessionToken))
		if err != nil {
			log.Printf("failed to verify jwt token: %s\n", err)
			next()
			return
		}

		ctx.SetUserValue("token", jwtToken)

		next()
	})
	srv.Use(func(ctx *fasthttp.RequestCtx, next func()) {
		if ctx.UserValue("token") == nil && string(ctx.Path()) != "/login" {
			ctx.Redirect("/login", 302)
			return
		}
		next()
	})

	srv.GET("/hp", func(ctx *fasthttp.RequestCtx) error {
		ctx.SetStatusCode(200)
		ctx.SetBody(nil)
		return nil
	})
	srv.GET("/", func(ctx *fasthttp.RequestCtx) error {
		dataMap := make(map[string]any)
		token := ctx.UserValue("token")
		dataMap["authed"] = token != nil
		if token != nil {
			dataMap["username"] = token.(*auth.JwtPayload).Username
		}
		dataMap["title"] = "Disgord"

		curServer := make(map[string]any)
		channels := make(map[string][]string)
		generalChannels := []string{"general", "off-topic", "bot", "spam", "game"}
		textChannels := []string{"Test Text", "Test Text 2"}
		voiceChannels := []string{"Test Voice", "Test Voice 2"}
		channels["General"] = generalChannels
		channels["Text Channels"] = textChannels
		channels["Voice Channels"] = voiceChannels

		curServer["Channels"] = channels
		dataMap["Server"] = curServer

		err := templates.ExecuteTemplate(ctx, "indexPage", dataMap)
		if err != nil {
			log.Print(err)
			return err
		}
		return nil
	})
	srv.GET("/login", func(ctx *fasthttp.RequestCtx) error {
		err := templates.ExecuteTemplate(ctx, "loginPage", nil)
		if err != nil {
			log.Print(err)
			return err
		}
		return nil
	})
	srv.GET("/logout", func(ctx *fasthttp.RequestCtx) error {
		sCookie, rCookie := createTokenCookies("", "", true)
		ctx.Response.Header.SetCookie(sCookie)
		ctx.Response.Header.SetCookie(rCookie)
		ctx.Redirect("/", 302)
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

	srv.POST("/login", func(ctx *fasthttp.RequestCtx) error {
		args := ctx.PostArgs()
		if !args.Has("username") || !args.Has("password") {
			return nil
		}

		username := string(args.Peek("username"))
		password := string(args.Peek("password"))
		_ = password

		jwtPayload := &auth.JwtPayload{
			Username: username,
			Admin:    false,
			UserId:   1,
		}

		sToken, rToken, err := auth.GenerateTokens(jwtPayload)
		if err != nil {
			return err
		}

		sCookie, rCookie := createTokenCookies(sToken, rToken, false)
		ctx.Response.Header.SetCookie(sCookie)
		ctx.Response.Header.SetCookie(rCookie)

		ctx.Redirect("/", 302)
		return nil
	})

	srv.SetErrorTemplate(404, "404Page")
	srv.SetErrorTemplate(500, "500Page")
	err = srv.Run()
	if err != nil {
		log.Fatal(err)
	}
}
