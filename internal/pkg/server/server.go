package server

import (
	"github.com/valyala/fasthttp"
	"html/template"
)

type Server struct {
	routeHandlers map[string]func(template *template.Template, ctx *fasthttp.RequestCtx) error
	templates     *template.Template
}

func New(templateDirectory string) (*Server, error) {
	s := &Server{
		routeHandlers: make(map[string]func(template *template.Template, ctx *fasthttp.RequestCtx) error),
	}
	templates := s.parseTemplates(templateDirectory, nil)
	s.templates = templates
	return s, nil
}

func (s *Server) AddRoute(route string, handler func(template *template.Template, ctx *fasthttp.RequestCtx) error) {
	s.routeHandlers[route] = handler
}

func (s *Server) errorWrapper(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.SetContentType("text/html")
	err := s.handleRouter(ctx)

	if err != nil {
		ctx.SetStatusCode(500)
		s.templates.ExecuteTemplate(ctx, "500Page", nil)
	}
}

func (s *Server) handleRouter(ctx *fasthttp.RequestCtx) error {
	// TODO add middlewares here
	handler, ok := s.routeHandlers[string(ctx.Path())]
	if !ok {
		ctx.SetStatusCode(404)
		s.templates.ExecuteTemplate(ctx, "404Page", nil)
		return nil
	}
	return handler(s.templates, ctx)
}

func (s *Server) Run() error {
	err := fasthttp.ListenAndServe(":8080", s.errorWrapper)
	if err != nil {
		return err
	}
	return nil
}
