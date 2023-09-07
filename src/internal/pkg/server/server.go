package server

import (
	"fmt"
	"github.com/valyala/fasthttp"
)

type Server struct {
}

func New() (*Server, error) {
	return &Server{}, nil
}

func (s *Server) HandleRouter(ctx *fasthttp.RequestCtx) {
	switch string(ctx.Path()) {
	case "/hp":
		ctx.SetStatusCode(200)
		ctx.SetBody(nil)
	default:
		fmt.Fprintf(ctx, "Disgord boiizzz: %q, %q", ctx.Path(), ctx.RequestURI())
	}
}

func (s *Server) Run() error {
	err := fasthttp.ListenAndServe(":8080", s.HandleRouter)
	if err != nil {
		return err
	}
	return nil
}
