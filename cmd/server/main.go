package main

import (
	"Disgord/internal/pkg/auth"
	"Disgord/internal/pkg/server"
	"Disgord/internal/pkg/sse"
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/valyala/fasthttp"
	"html/template"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type User struct {
	username string
	password string
	userId   uint
}

type Message struct {
	Username string
	Message  string
}

type MessageList struct {
	messages []Message
	lock     sync.RWMutex
}

var templates *template.Template
var users map[uint]User
var userIds map[string]uint
var messages MessageList

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

func addHXRequest(data map[string]any, ctx *fasthttp.RequestCtx) map[string]any {
	data["isHXRequest"] = ctx.UserValue("isHXRequest").(bool)
	return data
}

func redirect(uri string, code int, ctx *fasthttp.RequestCtx) {
	u := ctx.URI()
	if os.Getenv("PRODUCTION") == "true" {
		u.SetScheme("https")
	}
	u.Update(uri)
	ctx.Response.Header.Add("Location", string(u.FullURI()))
	ctx.SetStatusCode(code)
}

func main() {
	users = make(map[uint]User)
	userIds = make(map[string]uint)
	messages = MessageList{messages: make([]Message, 0)}
	templates = parseTemplates("./cmd/server/templates", nil)
	go watchTemplates("./cmd/server/templates")

	sseServer := sse.New()

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

			user, ok := users[refreshPayload.UserId]
			if !ok {
				next()
				return
			}

			jwtPayload := &auth.JwtPayload{
				Username: user.username,
				Admin:    false,
				UserId:   user.userId,
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
		if ctx.UserValue("token") == nil && string(ctx.Path()) != "/login" && string(ctx.Path()) != "/hp" {
			redirect("/login", 302, ctx)
			return
		}
		next()
	})
	srv.Use(func(ctx *fasthttp.RequestCtx, next func()) {
		ctx.SetUserValue("isHXRequest", string(ctx.Request.Header.Peek("HX-Request")) == "true")
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
		dataMap["username"] = token.(*auth.JwtPayload).Username
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

		func() {
			messages.lock.RLock()
			defer messages.lock.RUnlock()
			dataMap["Messages"] = messages.messages
		}()

		err := templates.ExecuteTemplate(ctx, "indexPage", addHXRequest(dataMap, ctx))
		if err != nil {
			log.Print(err)
			return err
		}
		return nil
	})
	srv.GET("/login", func(ctx *fasthttp.RequestCtx) error {
		dataMap := make(map[string]any)
		err := templates.ExecuteTemplate(ctx, "loginPage", addHXRequest(dataMap, ctx))
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
		redirect("/", 302, ctx)
		return nil
	})
	srv.GET("/navbar", func(ctx *fasthttp.RequestCtx) error {
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

	srv.GET("/ws", func(ctx *fasthttp.RequestCtx) error {
		return nil
	})

	srv.GET("/messages/sse", func(ctx *fasthttp.RequestCtx) error {
		ctx.Response.Header.Set("Cache-Control", "no-cache")
		ctx.Response.Header.Set("Connection", "keep-alive")
		ctx.Response.Header.Set("Content-Type", "text/event-stream")

		ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
			client := sseServer.MakeClient()
			defer sseServer.DestroyClient(client)

			for {
				select {
				case event := <-client:
					w.Write([]byte("id: 1\n"))
					w.Write([]byte("event: newMessage\n"))
					str := string(event)
					_ = str
					_, err := w.Write(event)
					if err != nil {
						return
					}
					w.Flush()
				}
			}
		})
		return nil
	})

	srv.POST("/message/new", func(ctx *fasthttp.RequestCtx) error {
		args := ctx.PostArgs()
		if !args.Has("message") {
			ctx.SetStatusCode(400)
			return nil
		}

		msg := Message{
			Message:  strings.Trim(string(args.Peek("message")), " "),
			Username: ctx.UserValue("token").(*auth.JwtPayload).Username,
		}

		if len(msg.Message) < 1 {
			ctx.SetStatusCode(400)
			return nil
		}

		dataMap := make(map[string]any)
		dataMap["Message"] = msg.Message
		dataMap["Username"] = msg.Username

		ctx.Response.Header.Add("HX-Trigger", "clearMsgTextarea")

		var buf bytes.Buffer
		err := templates.ExecuteTemplate(&buf, "message", addHXRequest(dataMap, ctx))
		ctx.Write(buf.Bytes())
		func() {
			messages.lock.Lock()
			defer messages.lock.Unlock()
			messages.messages = append(messages.messages, msg)
		}()
		if err == nil {
			html := string(buf.Bytes())
			html = strings.ReplaceAll(html, "\r", "")
			html = strings.ReplaceAll(html, "\n", "")
			sseBytes := []byte(html)
			sseServer.SendBytes(sseBytes)
		}

		return err
	})

	srv.POST("/login", func(ctx *fasthttp.RequestCtx) error {
		args := ctx.PostArgs()
		if !args.Has("username") || !args.Has("password") {
			return nil
		}

		username := string(args.Peek("username"))
		password := string(args.Peek("password"))

		passHash, err := auth.HashPassword(password)
		if err != nil {
			return err
		}

		userId := uint(len(users) + 1)
		id, ok := userIds[strings.ToLower(username)]
		if ok {
			userId = id
		} else {
			users[userId] = User{
				username: username,
				password: passHash,
				userId:   userId,
			}
		}

		user := users[userId]
		if user.password != passHash {
			return errors.New("invalid password")
		}

		jwtPayload := &auth.JwtPayload{
			Username: username,
			Admin:    false,
			UserId:   uint(userId),
		}

		sToken, rToken, err := auth.GenerateTokens(jwtPayload)
		if err != nil {
			return err
		}

		sCookie, rCookie := createTokenCookies(sToken, rToken, false)
		ctx.Response.Header.SetCookie(sCookie)
		ctx.Response.Header.SetCookie(rCookie)
		redirect("/", 302, ctx)
		return nil
	})

	srv.SetErrorTemplate(404, "404Page")
	srv.SetErrorTemplate(500, "500Page")
	err = srv.Run()
	if err != nil {
		log.Fatal(err)
	}
}
