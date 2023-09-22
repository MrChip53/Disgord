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

type ServerMemberships struct {
	ServerId primitive.ObjectID `bson:"serverId"`
	IsOwner  bool               `bson:"isOwner"`
}

type Message struct {
	ID             primitive.ObjectID `bson:"_id"`
	Server         primitive.ObjectID `bson:"server"`
	Channel        primitive.ObjectID `bson:"channel"`
	AuthorId       primitive.ObjectID `bson:"authorId"`
	Username       string             `bson:"username"`
	AvatarObjectId string             `bson:"avatarObjectId"`
	Timestamp      time.Time          `bson:"timestamp"`
	Type           int                `bson:"type"`
	Message        string             `bson:"message"`
	Command        string             `bson:"command"`
}

type Server struct {
	ID       primitive.ObjectID `bson:"_id"`
	Name     string             `bson:"name"`
	Channels []Channel          `bson:"channels,omitempty"`
}

type Channel struct {
	ID       primitive.ObjectID `bson:"_id"`
	Name     string             `bson:"name"`
	ServerId primitive.ObjectID `bson:"serverId"`
	Type     int                `bson:"type"`
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
	defer func() {
		_ = watcher.Close()
	}()
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
	defer func() {
		_ = client.Close()
	}()

	mongoClient := NewMongo()
	defer mongoClient.Close()
	sseServer := sse.New()

	srv, err := server.New(templates)
	if err != nil {
		log.Fatal(err)
	}

	srv.UseAuth(func(ctx *server.Context) bool {
		sessionToken := ctx.Context().Request.Header.Cookie("SessionToken")
		if len(sessionToken) == 0 {
			refreshToken := ctx.Context().Request.Header.Cookie("RefreshToken")
			if len(refreshToken) == 0 {
				return false
			}

			refreshPayload, err := auth.VerifyRefreshToken(string(refreshToken))
			if err != nil {
				log.Printf("failed to verify refresh token: %s\n", err)
				return false
			}

			objectID, err := primitive.ObjectIDFromHex(refreshPayload.UserId)
			if err != nil {
				log.Printf("failed to parse object id: %s\n", err)
				return false
			}

			coll := mongoClient.client.Database("disgord").Collection("users")
			filter := bson.D{{"_id", objectID}}
			var user User
			err = coll.FindOne(context.TODO(), filter).Decode(&user)
			if err != nil {
				return false
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
				return false
			}

			sCookie, rCookie := createTokenCookies(sToken, rToken, false)
			ctx.Context().Response.Header.SetCookie(sCookie)
			ctx.Context().Response.Header.SetCookie(rCookie)
			ctx.Context().SetUserValue("token", jwtPayload)
		}

		jwtToken, err := auth.VerifyJwtToken(string(sessionToken))
		if err != nil {
			log.Printf("failed to verify jwt token: %s\n", err)
			return false
		}

		ctx.Context().SetUserValue("token", jwtToken)
		return true
	})

	srv.UseMiddleware(func(ctx *server.Context, next func()) {
		path := string(ctx.Context().Path())
		if strings.HasPrefix(path, "/assets") {
			if strings.HasSuffix(path, ".css") {
				ctx.Context().Response.Header.SetContentType("text/css")
				ctx.Context().SendFile("./public/" + path[8:])
				return
			} else if strings.HasSuffix(path, ".js") {
				ctx.Context().Response.Header.SetContentType("text/javascript")
				ctx.Context().SendFile("./public/" + path[8:])
				return
			}
		}
		ctx.Context().Response.Header.SetContentType("text/html")
		next()
	})
	srv.UseMiddleware(func(ctx *server.Context, next func()) {
		if ctx.Context().UserValue("token") == nil && string(ctx.Context().Path()) != "/login" &&
			string(ctx.Context().Path()) != "/hp" && string(ctx.Context().Path()) != "/favicon.ico" {
			redirect("/login", 302, ctx.Context())
			return
		}
		next()
	})
	srv.UseMiddleware(func(ctx *server.Context, next func()) {
		ctx.Context().SetUserValue("isHXRequest", string(ctx.Context().Request.Header.Peek("HX-Request")) == "true")
		next()
	})

	srv.GET("/hp", false, func(ctx *server.Context) error {
		ctx.Context().SetStatusCode(200)
		ctx.Context().SetBody(nil)
		return nil
	})
	srv.GET("/", true, func(ctx *server.Context) error {
		token := ctx.Context().UserValue("token")
		userId := token.(*auth.JwtPayload).UserId
		user, err := mongoClient.GetUser(userId)
		if err != nil {
			return err
		}
		uri := fmt.Sprintf("/server/%s/channel/%s", user.LastActiveServer.Hex(), user.LastActiveChannel.Hex())
		redirect(uri, 302, ctx.Context())
		return nil
	})
	srv.GET("/messages", true, func(ctx *server.Context) error {
		dataMap := make(map[string]any)
		msgs, err := mongoClient.GetMessages("", "")
		if err == nil {
			dataMap["Messages"] = msgs
		}
		ctx.Context().Response.Header.Set("HX-Retarget", "main-window_messageWindow")
		ctx.Context().Response.Header.Set("HX-Reswap", "innerHTML")
		return templates.ExecuteTemplate(ctx.Context(), "messages", addHXRequest(dataMap, ctx.Context()))
	})
	srv.GET("/login", false, func(ctx *server.Context) error {
		if ctx.Context().UserValue("token") != nil {
			redirect("/", 302, ctx.Context())
			return nil
		}

		dataMap := make(map[string]any)
		dataMap["title"] = "Login - Disgord"
		err := templates.ExecuteTemplate(ctx.Context(), "loginPage", addHXRequest(dataMap, ctx.Context()))
		if err != nil {
			log.Print(err)
			return err
		}
		return nil
	})
	srv.GET("/logout", true, func(ctx *server.Context) error {
		sCookie, rCookie := createTokenCookies("", "", true)
		ctx.Context().Response.Header.SetCookie(sCookie)
		ctx.Context().Response.Header.SetCookie(rCookie)
		redirect("/", 302, ctx.Context())
		return nil
	})
	srv.GET("/settings", true, func(ctx *server.Context) error {
		dataMap := make(map[string]any)
		token := ctx.Context().UserValue("token")
		dataMap["avatarUrl"] = fmt.Sprintf("https://storage.googleapis.com/disgord-files/%s", token.(*auth.JwtPayload).AvatarObjectId)
		err := templates.ExecuteTemplate(ctx.Context(), "userSettingsModal", dataMap)
		if err != nil {
			log.Print(err)
			return err
		}
		return nil
	})
	srv.GET("/server/{serverId}", true, func(ctx *server.Context) error {
		serverId := ctx.Context().UserValue("serverId").(string)
		s, err := mongoClient.GetServer(serverId)
		if err != nil {
			return err
		}
		uri := fmt.Sprintf("/server/%s/channel/%s", serverId, s.Channels[0].ID.Hex())
		redirect(uri, 302, ctx.Context())
		return nil
	})

	// TODO dont like how bloated this func feels
	srv.GET("/server/{serverId}/channel/{channelId}", true, func(ctx *server.Context) error {
		token := ctx.Context().UserValue("token")
		serverId := ctx.Context().UserValue("serverId").(string)
		channelId := ctx.Context().UserValue("channelId").(string)
		userId := token.(*auth.JwtPayload).UserId
		user, err := mongoClient.GetUser(userId)
		if err != nil {
			return err
		}
		valid := user.HasServerAccess(serverId)
		if !valid {
			uri := fmt.Sprintf("/server/%s", user.Servers[0].ServerId.Hex())
			redirect(uri, 302, ctx.Context())
			return nil
		}
		user.UpdateLastServerAndChannel(mongoClient, serverId, channelId)
		dataMap := make(map[string]any)
		dataMap["Username"] = user.Username
		dataMap["AvatarObjectId"] = user.AvatarObjectId
		dataMap["title"] = "Disgord"
		dataMap["ServerId"] = serverId
		dataMap["ChannelId"] = channelId
		msgs, err := mongoClient.GetMessages(serverId, channelId)
		dataMap["MessagesError"] = err != nil
		if err == nil {
			dataMap["Messages"] = msgs
		}
		servers := user.GetServers(mongoClient)
		dataMap["Servers"] = servers
		for _, s := range servers {
			if s.ID.Hex() == serverId {
				dataMap["Server"] = s
				dataMap["title"] = s.Name + " - Disgord"
				break
			}
		}
		return templates.ExecuteTemplate(ctx.Context(), "indexPage", addHXRequest(dataMap, ctx.Context()))
	})

	srv.GET("/ws", true, func(ctx *server.Context) error {
		return nil
	})
	srv.GET("/messages/sse/{serverId}/{channelId}", true, func(ctx *server.Context) error {
		notify := ctx.Context().Done()
		username := ctx.Context().UserValue("token").(*auth.JwtPayload).Username
		serverId := ctx.Context().UserValue("serverId").(string)
		channelId := ctx.Context().UserValue("channelId").(string)
		ctx.Context().HijackSetNoResponse(true)
		ctx.Context().Hijack(func(c net.Conn) {
			client := sseServer.MakeClient(username, serverId, channelId)
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

	srv.POST("/server/{serverId}/channel/{channelId}/message/new", true, func(ctx *server.Context) error {
		serverId := ctx.Context().UserValue("serverId").(string)
		channelId := ctx.Context().UserValue("channelId").(string)
		serverObj, err := primitive.ObjectIDFromHex(serverId)
		if err != nil {
			return err
		}
		channelObj, err := primitive.ObjectIDFromHex(channelId)
		if err != nil {
			return err
		}

		args := ctx.Context().PostArgs()
		if !args.Has("message") {
			ctx.Context().SetStatusCode(400)
			return nil
		}

		msgStr := strings.Trim(string(args.Peek("message")), " ")
		cmd := ""
		if strings.HasPrefix(msgStr, "/") {
			msgStr, cmd = handleCommand(msgStr)
		}

		authId, err := primitive.ObjectIDFromHex(ctx.Context().UserValue("token").(*auth.JwtPayload).UserId)
		if err != nil {
			return err
		}

		msg := Message{
			Message:        msgStr,
			AuthorId:       authId,
			Command:        cmd,
			Username:       ctx.Context().UserValue("token").(*auth.JwtPayload).Username,
			Timestamp:      time.Now().UTC(),
			Type:           0,
			AvatarObjectId: ctx.Context().UserValue("token").(*auth.JwtPayload).AvatarObjectId,
			Channel:        channelObj,
			Server:         serverObj,
		}

		if len(msg.Message) < 1 {
			ctx.Context().SetStatusCode(400)
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

		ctx.Context().Response.Header.Set("HX-Trigger", "clearMsgTextarea")
		ctx.Context().Response.Header.Set("HX-Reswap", "none")

		var buf bytes.Buffer
		err = templates.ExecuteTemplate(&buf, "message", addHXRequest(dataMap, ctx.Context()))
		_, _ = ctx.Context().Write(buf.Bytes())
		if err == nil {
			html := string(buf.Bytes())
			html = strings.ReplaceAll(html, "\r", "")
			html = strings.ReplaceAll(html, "\n", "")
			sseBytes := []byte(html)
			sseServer.SendMessage("1", "newMessage", sseBytes, serverId, channelId)
		}

		return err
	})
	srv.POST("/message/new", true, func(ctx *server.Context) error {
		args := ctx.Context().PostArgs()
		if !args.Has("message") {
			ctx.Context().SetStatusCode(400)
			return nil
		}

		msgStr := strings.Trim(string(args.Peek("message")), " ")
		cmd := ""
		if strings.HasPrefix(msgStr, "/") {
			msgStr, cmd = handleCommand(msgStr)
		}

		authId, err := primitive.ObjectIDFromHex(ctx.Context().UserValue("token").(*auth.JwtPayload).UserId)
		if err != nil {
			return err
		}

		msg := Message{
			Message:        msgStr,
			AuthorId:       authId,
			Command:        cmd,
			Username:       ctx.Context().UserValue("token").(*auth.JwtPayload).Username,
			Timestamp:      time.Now().UTC(),
			Type:           0,
			AvatarObjectId: ctx.Context().UserValue("token").(*auth.JwtPayload).AvatarObjectId,
		}

		if len(msg.Message) < 1 {
			ctx.Context().SetStatusCode(400)
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

		ctx.Context().Response.Header.Set("HX-Trigger", "clearMsgTextarea")
		ctx.Context().Response.Header.Set("HX-Reswap", "none")

		var buf bytes.Buffer
		err = templates.ExecuteTemplate(&buf, "message", addHXRequest(dataMap, ctx.Context()))
		_, _ = ctx.Context().Write(buf.Bytes())
		if err == nil {
			html := string(buf.Bytes())
			html = strings.ReplaceAll(html, "\r", "")
			html = strings.ReplaceAll(html, "\n", "")
			sseBytes := []byte(html)
			sseServer.SendBytes("1", "newMessage", sseBytes)
		}

		return err
	})
	srv.POST("/login", false, func(ctx *server.Context) error {
		args := ctx.Context().PostArgs()
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
					ctx.Context().Response.Header.Set("HX-Trigger", "loginFailed")
					return sendErrorToast(ctx.Context(), "Failed to login or create account")
				}
				oId, err := primitive.ObjectIDFromHex("65072e04a3eec0a2c2eefc6b")
				if err != nil {
					ctx.Context().Response.Header.Set("HX-Trigger", "loginFailed")
					return sendErrorToast(ctx.Context(), "Failed to login or create account")
				}

				user = User{
					Username:      username,
					LowerUsername: strings.ToLower(username),
					Password:      passHash,
					Servers: []ServerMemberships{
						{
							ServerId: oId,
							IsOwner:  false,
						},
					},
				}
				id, err := mongoClient.CreateUser(&user)
				if err != nil {
					ctx.Context().Response.Header.Set("HX-Trigger", "loginFailed")
					return sendErrorToast(ctx.Context(), "Failed to login or create account")
				}
				user.ID = id
			} else {
				ctx.Context().Response.Header.Set("HX-Trigger", "loginFailed")
				return sendErrorToast(ctx.Context(), "Failed to login or create account")
			}
		}

		if b, err := auth.VerifyPassword(password, user.Password); b == false || err != nil {
			ctx.Context().Response.Header.Set("HX-Trigger", "loginFailed")
			return sendErrorToast(ctx.Context(), "Failed to login or create account")
		}

		if len(user.Servers) == 0 {
			err := user.AddServer("65072e04a3eec0a2c2eefc6b", mongoClient)
			if err != nil {
				ctx.Context().Response.Header.Set("HX-Trigger", "loginFailed")
				return sendErrorToast(ctx.Context(), "Failed to login or create account")
			}
		}

		jwtPayload := &auth.JwtPayload{
			Username:       user.Username,
			Admin:          false,
			UserId:         user.ID.Hex(),
			AvatarObjectId: user.AvatarObjectId,
		}

		sToken, rToken, err := auth.GenerateTokens(jwtPayload)
		if err != nil {
			ctx.Context().Response.Header.Set("HX-Trigger", "loginFailed")
			return sendErrorToast(ctx.Context(), "Failed to login or create account")
		}

		sCookie, rCookie := createTokenCookies(sToken, rToken, false)
		ctx.Context().Response.Header.SetCookie(sCookie)
		ctx.Context().Response.Header.SetCookie(rCookie)
		redirect("/", 302, ctx.Context())
		return nil
	})
	srv.POST("/user/settings", true, func(ctx *server.Context) error {
		// TODO goroutine the file upload/image conversion and compression
		m, err := ctx.Context().Request.MultipartForm()

		formFile, ok := m.File["avatar"]
		if !ok || len(formFile) != 1 {
			return sendErrorToast(ctx.Context(), "Failed to update settings")
		}

		file, err := formFile[0].Open()
		if err != nil {
			return sendErrorToast(ctx.Context(), "Failed to update settings")
		}
		defer func() {
			_ = file.Close()
		}()

		fileBytes := make([]byte, formFile[0].Size)
		_, err = file.Read(fileBytes)
		if err != nil {
			return sendErrorToast(ctx.Context(), "Failed to update settings")
		}
		userId := ctx.Context().UserValue("token").(*auth.JwtPayload).UserId
		objName := "avatar-" + userId
		bucket := client.Bucket("disgord-files")
		obj := bucket.Object(objName)
		wc := obj.NewWriter(ctx.Context())
		if _, err = wc.Write(fileBytes); err != nil {
			_ = wc.Close()
			return sendErrorToast(ctx.Context(), "Failed to update settings")
		}
		if err := wc.Close(); err != nil {
			return sendErrorToast(ctx.Context(), "Failed to update settings")
		}
		_, err = obj.Attrs(ctx.Context())
		if err != nil {
			//log.Fatalf("Failed to get object attributes: %v", err)
			return sendErrorToast(ctx.Context(), "Failed to update settings")
		}
		if _, err := obj.Update(ctx.Context(), storage.ObjectAttrsToUpdate{
			ACL: []storage.ACLRule{
				{Entity: storage.AllUsers, Role: storage.RoleReader},
			},
			CacheControl: "public, max-age=15",
		}); err != nil {
			//log.Fatalf("Failed to update object ACL: %v", err)
			return sendErrorToast(ctx.Context(), "Failed to update settings")
		}
		objectID, err := primitive.ObjectIDFromHex(userId)
		if err != nil {
			return sendErrorToast(ctx.Context(), "Failed to update settings")
		}
		filter := bson.M{"_id": objectID} // Filter to identify the document(s) to update

		update := bson.M{
			"$set": bson.M{"AvatarObjectId": objName},
		}
		coll := mongoClient.client.Database("disgord").Collection("users")
		if _, err := coll.UpdateOne(context.Background(), filter, update); err != nil {
			return sendErrorToast(ctx.Context(), "Failed to update settings")
		}
		ctx.Context().Response.Header.Set("HX-Trigger", "closeModal")
		return sendErrorToast(ctx.Context(), "User settings saved")
	})

	srv.DELETE("/message/{messageId}", true, func(ctx *server.Context) error {
		mId := ctx.Context().UserValue("messageId").(string)
		message, err := mongoClient.GetMessage(mId)
		if err != nil {
			return sendErrorToast(ctx.Context(), "Failed to delete message")
		}
		hex := message.AuthorId.Hex()
		if hex != ctx.Context().UserValue("token").(*auth.JwtPayload).UserId && strings.ReplaceAll(hex, "0", "") != "" {
			return sendErrorToast(ctx.Context(), "Failed to delete message")
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
