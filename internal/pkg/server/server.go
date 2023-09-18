package server

import (
	"github.com/valyala/fasthttp"
	"html/template"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"
)

type HandlerFunc = func(ctx *fasthttp.RequestCtx) error
type MiddlewareFunc = func(ctx *fasthttp.RequestCtx, next func())

type Handler struct {
	handler    HandlerFunc
	re         *regexp.Regexp
	variables  []string
	slashCount int
}

type Server struct {
	routeHandlers  map[string]Handler
	errorTemplates map[int]string
	middlewares    []MiddlewareFunc
	mwLock         sync.Mutex
	templates      *template.Template
}

func New(templates *template.Template) (*Server, error) {
	s := &Server{
		routeHandlers:  make(map[string]Handler),
		errorTemplates: make(map[int]string),
		templates:      templates,
	}
	return s, nil
}

func (s *Server) addRoute(route string, method string, handlerFunc func(ctx *fasthttp.RequestCtx) error) {
	key := method + ":" + route
	re := regexp.MustCompile("{([^}]+)}")
	matches := re.FindAllStringSubmatch(route, -1)
	handler := Handler{
		handler: handlerFunc,
	}
	if len(matches) > 0 {
		variables := make([]string, len(matches))
		handler.slashCount = strings.Count(route, "/")
		for i, match := range matches {
			variables[i] = match[1]
		}
		handler.variables = variables

		newRoute := re.ReplaceAllString("^"+route+"$", "(.*?)")
		newRoute = strings.ReplaceAll(newRoute, "/", "\\/")
		key = method + ":" + newRoute
		handler.re = regexp.MustCompile(newRoute)
	}
	s.routeHandlers[key] = handler
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

func (s *Server) error(errorCode int, ctx *fasthttp.RequestCtx) {
	ctx.SetStatusCode(errorCode)
	templ, ok := s.errorTemplates[errorCode]
	if ok {
		err := s.templates.ExecuteTemplate(ctx, templ, nil)
		if err != nil {
			log.Println("error executing template: ", err)
		}
	}
}

func (s *Server) getRouteHandler(method string, route string) (*Handler, bool) {
	handler, ok := s.routeHandlers[method+":"+route]
	if !ok {
		// TODO remove/fix this with better implementation
		if strings.Contains(route, ".") {
			lastIndex := strings.LastIndex(route, "/")
			return s.getRouteHandler(method, route[:lastIndex+1]+"*")
		} else {
			return s.findHandlerRegex(method, route)
		}
	}

	return &handler, ok
}

func (s *Server) findHandlerRegex(method string, route string) (*Handler, bool) {
	key := method + ":"
	for k, v := range s.routeHandlers {
		count := strings.Count(route, "/")
		if v.re == nil || !strings.HasPrefix(k, key) || v.slashCount != count {
			continue
		}
		matchString := v.re.MatchString(route)
		if matchString {
			return &v, true
		}
	}
	return nil, false
}

func (s *Server) handleRouter(ctx *fasthttp.RequestCtx) {
	start := time.Now().UnixNano()
	s.mwLock.Lock()
	defer s.mwLock.Unlock()

	index := -1

	var next func()
	next = func() {
		index++
		if index < len(s.middlewares) {
			s.middlewares[index](ctx, next)
			return
		}
		method := string(ctx.Method())
		path := string(ctx.Path())
		handler, ok := s.getRouteHandler(method, path)
		if !ok {
			log.Println("no handler found for route: ", string(ctx.Path()))
			s.error(404, ctx)
			return
		}
		if len(handler.variables) > 0 {
			matches := handler.re.FindStringSubmatch(string(ctx.Path()))
			if len(matches) > 0 {
				for i, match := range matches[1:] {
					ctx.SetUserValue(handler.variables[i], match)
				}
			}
		}
		err := handler.handler(ctx)
		if err != nil {
			log.Println("error running handler: ", err)
			s.error(500, ctx)
		}
	}

	next()
	end := time.Now().UnixNano()
	log.Printf("Request for %s took: %f ms\n", ctx.Path(), float64(end-start)/1000000.0)
}

func (s *Server) Run() error {
	err := fasthttp.ListenAndServe(":8080", s.handleRouter)
	if err != nil {
		return err
	}
	return nil
}
