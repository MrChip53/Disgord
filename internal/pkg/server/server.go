package server

import (
	"github.com/valyala/fasthttp"
	"html/template"
	"log"
	"strings"
	"sync"
)

type HandlerFunc = func(ctx *fasthttp.RequestCtx) error
type MiddlewareFunc = func(ctx *fasthttp.RequestCtx, next func() error) error

type Server struct {
	routeHandlers     map[string]HandlerFunc
	errorTemplates    map[int]string
	middlewares       []MiddlewareFunc
	mwLock            sync.Mutex
	templates         *template.Template
	templateDirectory string
}

func New(templateDirectory string) (*Server, error) {
	s := &Server{
		routeHandlers:     make(map[string]HandlerFunc),
		errorTemplates:    make(map[int]string),
		templateDirectory: templateDirectory,
	}
	return s, nil
}

func (s *Server) addRoute(route string, method string, handler func(ctx *fasthttp.RequestCtx) error) {
	s.routeHandlers[method+":"+route] = handler
}

func (s *Server) GET(route string, handler func(ctx *fasthttp.RequestCtx) error) {
	s.addRoute(route, "GET", handler)
}

func (s *Server) POST(route string, handler func(ctx *fasthttp.RequestCtx) error) {
	s.addRoute(route, "POST", handler)
}

func (s *Server) PUT(route string, handler func(ctx *fasthttp.RequestCtx) error) {
	s.addRoute(route, "PUT", handler)
}

func (s *Server) DELETE(route string, handler func(ctx *fasthttp.RequestCtx) error) {
	s.addRoute(route, "DELETE", handler)
}

func (s *Server) PATCH(route string, handler func(ctx *fasthttp.RequestCtx) error) {
	s.addRoute(route, "PATCH", handler)
}

func (s *Server) Use(middleware MiddlewareFunc) {
	s.middlewares = append(s.middlewares, middleware)
}

func (s *Server) SetErrorTemplate(statusCode int, templateName string) {
	s.errorTemplates[statusCode] = templateName
}

func (s *Server) getRouteHandler(method string, route string) (HandlerFunc, bool) {
	handler, ok := s.routeHandlers[method+":"+route]

	if !ok && strings.Contains(route, ".") {
		lastIndex := strings.LastIndex(route, "/")
		return s.getRouteHandler(method, route[:lastIndex+1]+"*")
	}

	return handler, ok
}

func (s *Server) handleRouter(ctx *fasthttp.RequestCtx) {
	s.mwLock.Lock()
	defer s.mwLock.Unlock()

	index := 0

	var next func() error
	next = func() error {
		index++
		if index < len(s.middlewares) {
			err := s.middlewares[index](ctx, next)
			if err != nil {
				index = len(s.middlewares)
				return nil
			}
		} else {
			// TODO Regex match strings for parameters in route
			method := string(ctx.Method())
			handler, ok := s.getRouteHandler(method, string(ctx.Path()))
			if !ok {
				ctx.SetStatusCode(404)
				templ, ok := s.errorTemplates[404]
				if ok {
					err := s.templates.ExecuteTemplate(ctx, templ, nil)
					if err != nil {
						log.Println("error executing template: ", err)
						ctx.SetStatusCode(404)
					}
				}
				return nil
			}
			err := handler(ctx)
			if err != nil {
				ctx.SetStatusCode(500)
				templ, ok := s.errorTemplates[500]
				if ok {
					err := s.templates.ExecuteTemplate(ctx, templ, nil)
					if err != nil {
						log.Println("error executing template: ", err)
						ctx.SetStatusCode(500)
					}
				}
				return nil
			}
		}
		return nil
	}

	err := next()
	if err != nil {

	}
}

func (s *Server) Run() error {
	err := fasthttp.ListenAndServe(":8080", s.handleRouter)
	if err != nil {
		return err
	}
	return nil
}
