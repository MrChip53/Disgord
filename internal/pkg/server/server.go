package server

import (
	"github.com/valyala/fasthttp"
	"html/template"
	"log"
	"strings"
	"time"
)

type HandlerFunc = func(template *template.Template, ctx *fasthttp.RequestCtx) error
type MiddlewareFunc = func(template *template.Template, ctx *fasthttp.RequestCtx) (bool, error)

type Server struct {
	routeHandlers     map[string]HandlerFunc
	errorTemplates    map[int]string
	middlewares       []MiddlewareFunc
	templates         *template.Template
	templateDirectory string
}

func New(templateDirectory string) (*Server, error) {
	s := &Server{
		routeHandlers:     make(map[string]HandlerFunc),
		errorTemplates:    make(map[int]string),
		middlewares:       make([]MiddlewareFunc, 0),
		templateDirectory: templateDirectory,
	}
	return s, nil
}

func (s *Server) addRoute(route string, method string, handler func(template *template.Template, ctx *fasthttp.RequestCtx) error) {
	s.routeHandlers[method+":"+route] = handler
}

func (s *Server) AddErrorTemplate(errorCode int, templateName string) {
	s.errorTemplates[errorCode] = templateName
}

func (s *Server) GET(route string, handler func(template *template.Template, ctx *fasthttp.RequestCtx) error) {
	s.addRoute(route, "GET", handler)
}

func (s *Server) POST(route string, handler func(template *template.Template, ctx *fasthttp.RequestCtx) error) {
	s.addRoute(route, "POST", handler)
}

func (s *Server) PUT(route string, handler func(template *template.Template, ctx *fasthttp.RequestCtx) error) {
	s.addRoute(route, "PUT", handler)
}

func (s *Server) DELETE(route string, handler func(template *template.Template, ctx *fasthttp.RequestCtx) error) {
	s.addRoute(route, "DELETE", handler)
}

func (s *Server) PATCH(route string, handler func(template *template.Template, ctx *fasthttp.RequestCtx) error) {
	s.addRoute(route, "PATCH", handler)
}

func (s *Server) Use(middleware MiddlewareFunc) {
	s.middlewares = append(s.middlewares, middleware)
}

func (s *Server) errorWrapper(ctx *fasthttp.RequestCtx) {
	start := time.Now().UnixNano()
	ctx.Response.Header.SetContentType("text/html")
	err := s.handleRouter(ctx)

	if err != nil {
		ctx.SetStatusCode(500)
		templ, ok := s.errorTemplates[500]
		if ok {
			s.templates.ExecuteTemplate(ctx, templ, nil)
		}
	}
	end := time.Now().UnixNano()
	log.Printf("Request for %s took: %f ms\n", ctx.Path(), float64(end-start)/1000000.0)
}

func (s *Server) getRouteHandler(method string, route string) (HandlerFunc, bool) {
	handler, ok := s.routeHandlers[method+":"+route]

	if !ok && strings.Contains(route, ".") {
		lastIndex := strings.LastIndex(route, "/")
		return s.getRouteHandler(method, route[:lastIndex+1]+"*")
	}

	return handler, ok
}

func (s *Server) handleRouter(ctx *fasthttp.RequestCtx) error {
	for _, mw := range s.middlewares {
		shouldContinue, err := mw(s.templates, ctx)
		if err != nil {
			return err
		}
		if !shouldContinue {
			return nil
		}
	}

	method := string(ctx.Method())
	handler, ok := s.getRouteHandler(method, string(ctx.Path()))
	if !ok {
		ctx.SetStatusCode(404)
		templ, ok := s.errorTemplates[404]
		if ok {
			s.templates.ExecuteTemplate(ctx, templ, nil)
		}
		return nil
	}
	return handler(s.templates, ctx)
}

func (s *Server) LoadTemplates(templateDirectory string, watch bool) {
	if watch {
		go s.watchTemplates(templateDirectory)
	}
	s.templates = s.parseTemplates(templateDirectory, nil)
}

func (s *Server) Run(watchTemplates bool) error {
	s.LoadTemplates(s.templateDirectory, watchTemplates)
	err := fasthttp.ListenAndServe(":8080", s.errorWrapper)
	if err != nil {
		return err
	}
	return nil
}
