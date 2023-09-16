package main

import (
	"Disgord/internal/pkg/auth"
	"Disgord/internal/pkg/server"
	"Disgord/internal/pkg/sse"
	"bytes"
	"cloud.google.com/go/storage"
	"context"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/valyala/fasthttp"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"google.golang.org/api/option"
	"html/template"
	"io/fs"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type User struct {
	ID             primitive.ObjectID `bson:"_id"`
	Username       string
	LowerUsername  string
	Password       string
	AvatarObjectId string
}

type Message struct {
	ID             primitive.ObjectID `bson:"_id"`
	Server         primitive.ObjectID
	Channel        primitive.ObjectID
	AuthorId       primitive.ObjectID
	Username       string
	AvatarObjectId string
	Timestamp      time.Time `bson:"timestamp"`
	Type           int
	Message        string
	Command        string
}

var templates *template.Template

func sendErrorToast(ctx *fasthttp.RequestCtx, message string) error {
	errorToast := make(map[string]any)
	errorToast["toastId"] = "toast-" + uuid.New().String()
	errorToast["toast"] = message

	ctx.Response.Header.Set("HX-Reswap", "beforeend")
	ctx.Response.Header.Set("HX-Retarget", "#toastContainer")

	return templates.ExecuteTemplate(ctx, "toast", errorToast)
}

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
				templates = parseTemplates(rootDir, getFuncMap())
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
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file")
	}

	templates = parseTemplates("./cmd/server/templates", getFuncMap())
	go watchTemplates("./cmd/server/templates")

	ctx := context.Background()
	client, err := storage.NewClient(ctx, option.WithCredentialsFile("gcs.json"))
	if err != nil {
		fmt.Printf("Failed to create Google Cloud Storage client: %v\n", err)
		return
	}
	defer client.Close()

	mongoClient := NewMongo()
	defer mongoClient.Close()
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

			objectID, err := primitive.ObjectIDFromHex(refreshPayload.UserId)
			if err != nil {
				log.Printf("failed to parse object id: %s\n", err)
				next()
				return
			}

			coll := mongoClient.client.Database("disgord").Collection("users")
			filter := bson.D{{"_id", objectID}}
			var user User
			err = coll.FindOne(context.TODO(), filter).Decode(&user)
			if err != nil {
				next()
				return
			}

			jwtPayload := &auth.JwtPayload{
				Username:       user.Username,
				Admin:          false,
				UserId:         user.ID.Hex(),
				AvatarObjectId: user.AvatarObjectId,
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
		dataMap["hasAvatar"] = token.(*auth.JwtPayload).AvatarObjectId != ""
		dataMap["avatarUrl"] = fmt.Sprintf("https://storage.googleapis.com/disgord-files/%s", token.(*auth.JwtPayload).AvatarObjectId)
		dataMap["title"] = "Home - Disgord"

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

		msgs, err := mongoClient.GetMessages("", "")
		if err == nil {
			dataMap["Messages"] = msgs
		}

		err = templates.ExecuteTemplate(ctx, "indexPage", addHXRequest(dataMap, ctx))
		if err != nil {
			log.Print(err)
			return err
		}
		return nil
	})
	srv.GET("/messages", func(ctx *fasthttp.RequestCtx) error {
		dataMap := make(map[string]any)
		msgs, err := mongoClient.GetMessages("", "")
		if err == nil {
			dataMap["Messages"] = msgs
		}
		ctx.Response.Header.Set("HX-Retarget", "main-window_messageWindow")
		ctx.Response.Header.Set("HX-Reswap", "innerHTML")
		return templates.ExecuteTemplate(ctx, "messages", addHXRequest(dataMap, ctx))
	})
	srv.GET("/login", func(ctx *fasthttp.RequestCtx) error {
		if ctx.UserValue("token") != nil {
			redirect("/", 302, ctx)
			return nil
		}

		dataMap := make(map[string]any)
		dataMap["title"] = "Login - Disgord"
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
	srv.GET("/settings", func(ctx *fasthttp.RequestCtx) error {
		dataMap := make(map[string]any)
		token := ctx.UserValue("token")
		dataMap["avatarUrl"] = fmt.Sprintf("https://storage.googleapis.com/disgord-files/%s", token.(*auth.JwtPayload).AvatarObjectId)
		err := templates.ExecuteTemplate(ctx, "userSettingsModal", dataMap)
		if err != nil {
			log.Print(err)
			return err
		}
		return nil
	})

	srv.GET("/ws", func(ctx *fasthttp.RequestCtx) error {
		return nil
	})
	srv.GET("/messages/sse", func(ctx *fasthttp.RequestCtx) error {
		notify := ctx.Done()
		username := ctx.UserValue("token").(*auth.JwtPayload).Username
		ctx.HijackSetNoResponse(true)
		ctx.Hijack(func(c net.Conn) {
			client := sseServer.MakeClient(username)
			defer sseServer.DestroyClient(client)

			httpMsg := []byte("HTTP/1.1 200 OK\r\n")
			httpMsg = append(httpMsg, []byte("Content-Type: text/event-stream\r\n")...)
			httpMsg = append(httpMsg, []byte("Cache-Control: no-cache\r\n")...)
			httpMsg = append(httpMsg, []byte("Connection: keep-alive\r\n")...)
			httpMsg = append(httpMsg, []byte("Keep-Alive: timeout=15\r\n")...)
			httpMsg = append(httpMsg, []byte("\r\n")...)
			if _, err := c.Write(httpMsg); err != nil {
				return
			}

			for {
				select {
				case event := <-client.Channel:
					msg := event.String()
					if err := c.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
						return
					}
					if _, err := c.Write([]byte(msg)); err != nil {
						return
					}
				case <-notify:
					return
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

		msgStr := strings.Trim(string(args.Peek("message")), " ")
		cmd := ""
		if strings.HasPrefix(msgStr, "/") {
			msgStr, cmd = handleCommand(msgStr)
		}

		authId, err := primitive.ObjectIDFromHex(ctx.UserValue("token").(*auth.JwtPayload).UserId)
		if err != nil {
			return err
		}

		msg := Message{
			Message:        msgStr,
			AuthorId:       authId,
			Command:        cmd,
			Username:       ctx.UserValue("token").(*auth.JwtPayload).Username,
			Timestamp:      time.Now().UTC(),
			Type:           0,
			AvatarObjectId: ctx.UserValue("token").(*auth.JwtPayload).AvatarObjectId,
		}

		if len(msg.Message) < 1 {
			ctx.SetStatusCode(400)
			return nil
		}

		id, err := mongoClient.CreateMessage(&msg)
		if err != nil {
			return err
		}

		dataMap := make(map[string]any)
		dataMap["ID"] = id
		dataMap["AuthorId"] = msg.AuthorId
		dataMap["Message"] = msg.Message
		dataMap["Command"] = msg.Command
		dataMap["Username"] = msg.Username
		dataMap["AvatarObjectId"] = msg.AvatarObjectId
		dataMap["Timestamp"] = msg.Timestamp
		dataMap["Type"] = msg.Type

		ctx.Response.Header.Set("HX-Trigger", "clearMsgTextarea")
		ctx.Response.Header.Set("HX-Reswap", "none")

		var buf bytes.Buffer
		err = templates.ExecuteTemplate(&buf, "message", addHXRequest(dataMap, ctx))
		ctx.Write(buf.Bytes())
		if err == nil {
			html := string(buf.Bytes())
			html = strings.ReplaceAll(html, "\r", "")
			html = strings.ReplaceAll(html, "\n", "")
			sseBytes := []byte(html)
			sseServer.SendBytes("1", "newMessage", sseBytes)
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

		coll := mongoClient.client.Database("disgord").Collection("users")
		filter := bson.D{{"lowerUsername", strings.ToLower(username)}}
		var user User
		err = coll.FindOne(context.TODO(), filter).Decode(&user)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				passHash, err := auth.HashPassword(password)
				if err != nil {
					ctx.Response.Header.Set("HX-Trigger", "loginFailed")
					return sendErrorToast(ctx, "Failed to login or create account")
				}
				user = User{
					Username:      username,
					LowerUsername: strings.ToLower(username),
					Password:      passHash,
				}
				err = mongoClient.CreateUser(&user)
				if err != nil {
					ctx.Response.Header.Set("HX-Trigger", "loginFailed")
					return sendErrorToast(ctx, "Failed to login or create account")
				}
			} else {
				ctx.Response.Header.Set("HX-Trigger", "loginFailed")
				return sendErrorToast(ctx, "Failed to login or create account")
			}
		}

		if b, err := auth.VerifyPassword(password, user.Password); b == false || err != nil {
			ctx.Response.Header.Set("HX-Trigger", "loginFailed")
			return sendErrorToast(ctx, "Failed to login or create account")
		}

		jwtPayload := &auth.JwtPayload{
			Username:       user.Username,
			Admin:          false,
			UserId:         user.ID.Hex(),
			AvatarObjectId: user.AvatarObjectId,
		}

		sToken, rToken, err := auth.GenerateTokens(jwtPayload)
		if err != nil {
			ctx.Response.Header.Set("HX-Trigger", "loginFailed")
			return sendErrorToast(ctx, "Failed to login or create account")
		}

		sCookie, rCookie := createTokenCookies(sToken, rToken, false)
		ctx.Response.Header.SetCookie(sCookie)
		ctx.Response.Header.SetCookie(rCookie)
		redirect("/", 302, ctx)
		return nil
	})
	srv.POST("/user/settings", func(ctx *fasthttp.RequestCtx) error {
		// TODO goroutine the file upload/image conversion and compression
		m, err := ctx.Request.MultipartForm()

		formFile, ok := m.File["avatar"]
		if !ok || len(formFile) != 1 {
			return sendErrorToast(ctx, "Failed to update settings")
		}

		file, err := formFile[0].Open()
		if err != nil {
			return sendErrorToast(ctx, "Failed to update settings")
		}
		defer file.Close()

		fileBytes := make([]byte, formFile[0].Size)
		_, err = file.Read(fileBytes)
		if err != nil {
			return sendErrorToast(ctx, "Failed to update settings")
		}
		userId := ctx.UserValue("token").(*auth.JwtPayload).UserId
		objName := "avatar-" + userId
		bucket := client.Bucket("disgord-files")
		obj := bucket.Object(objName)
		wc := obj.NewWriter(ctx)
		if _, err = wc.Write(fileBytes); err != nil {
			wc.Close()
			return sendErrorToast(ctx, "Failed to update settings")
		}
		if err := wc.Close(); err != nil {
			return sendErrorToast(ctx, "Failed to update settings")
		}
		_, err = obj.Attrs(ctx)
		if err != nil {
			log.Fatalf("Failed to get object attributes: %v", err)
		}
		if _, err := obj.Update(ctx, storage.ObjectAttrsToUpdate{
			ACL: []storage.ACLRule{
				{Entity: storage.AllUsers, Role: storage.RoleReader},
			},
			CacheControl: "public, max-age=15",
		}); err != nil {
			log.Fatalf("Failed to update object ACL: %v", err)
		}
		objectID, err := primitive.ObjectIDFromHex(userId)
		if err != nil {
			return sendErrorToast(ctx, "Failed to update settings")
		}
		filter := bson.M{"_id": objectID} // Filter to identify the document(s) to update

		update := bson.M{
			"$set": bson.M{"AvatarObjectId": objName},
		}
		coll := mongoClient.client.Database("disgord").Collection("users")
		if _, err := coll.UpdateOne(context.Background(), filter, update); err != nil {
			return sendErrorToast(ctx, "Failed to update settings")
		}
		ctx.Response.Header.Set("HX-Trigger", "closeModal")
		return sendErrorToast(ctx, "User settings saved")
	})

	srv.DELETE("/message/.*?", func(ctx *fasthttp.RequestCtx) error {
		mId := string(ctx.Path())[9:]
		message, err := mongoClient.GetMessage(mId)
		if err != nil {
			return sendErrorToast(ctx, "Failed to delete message")
		}
		hex := message.AuthorId.Hex()
		if hex != ctx.UserValue("token").(*auth.JwtPayload).UserId && strings.ReplaceAll(hex, "0", "") != "" {
			return sendErrorToast(ctx, "Failed to delete message")
		}
		sseServer.SendBytes("1", "deleteMessage", []byte("message-"+mId))
		return mongoClient.DeleteMessage(mId)
	})

	srv.SetErrorTemplate(404, "404Page")
	srv.SetErrorTemplate(500, "500Page")
	err = srv.Run()
	if err != nil {
		log.Fatal(err)
	}
}
