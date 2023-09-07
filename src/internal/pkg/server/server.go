package server

import (
	"github.com/valyala/fasthttp"
	"html/template"
)

type Server struct {
	routeHandlers map[string]func(template *template.Template, ctx *fasthttp.RequestCtx)
	templates     *template.Template
}

func New(templateDirectory string) (*Server, error) {
	s := &Server{
		routeHandlers: make(map[string]func(template *template.Template, ctx *fasthttp.RequestCtx)),
	}
	templates, err := s.parseTemplates(templateDirectory, nil)
	if err != nil {
		return nil, err
	}
	s.templates = templates
	return s, nil
}

func (s *Server) AddRoute(route string, handler func(template *template.Template, ctx *fasthttp.RequestCtx)) {
	s.routeHandlers[route] = handler
}

func (s *Server) HandleRouter(ctx *fasthttp.RequestCtx) {
	// TODO add middlewares here

	handler, ok := s.routeHandlers[string(ctx.Path())]
	if !ok {
		ctx.SetStatusCode(404)
		s.templates.ExecuteTemplate(ctx, "pages/404", nil)
		//fmt.Fprintf(ctx, "Disgord boiizzz: %q, %q", ctx.Path(), ctx.RequestURI())
		return
	}
	handler(s.templates, ctx)
}

func (s *Server) Run() error {
	err := fasthttp.ListenAndServe(":8080", s.HandleRouter)
	if err != nil {
		return err
	}
	return nil
}
