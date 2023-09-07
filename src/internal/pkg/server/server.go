package server

import (
	"fmt"
	"github.com/valyala/fasthttp"
)

type Server struct {
	routes map[string]func(ctx *fasthttp.RequestCtx)
}

func New() (*Server, error) {
	return &Server{
		routes: make(map[string]func(ctx *fasthttp.RequestCtx)),
	}, nil
}

func (s *Server) AddRoute(route string, handler func(ctx *fasthttp.RequestCtx)) {
	s.routes[route] = handler
}

func (s *Server) HandleRouter(ctx *fasthttp.RequestCtx) {
	handler, ok := s.routes[string(ctx.Path())]
	if !ok {
		fmt.Fprintf(ctx, "Disgord boiizzz: %q, %q", ctx.Path(), ctx.RequestURI())
		return
	}
	handler(ctx)
}

func (s *Server) Run() error {
	err := fasthttp.ListenAndServe(":8080", s.HandleRouter)
	if err != nil {
		return err
	}
	return nil
}
