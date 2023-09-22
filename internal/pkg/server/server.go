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

type HandlerFunc = func(ctx *Context) error
type MiddlewareFunc = func(ctx *Context, next func())
type AuthFunc = func(ctx *Context) bool

type Handler struct {
	handler      HandlerFunc
	re           *regexp.Regexp
	variables    []string
	slashCount   int
	requiresAuth bool
}

type Server struct {
	routeHandlers  map[string]Handler
	errorTemplates map[int]string
	middlewares    []MiddlewareFunc
	auths          []AuthFunc
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

func (s *Server) addRoute(route string, method string, requiresAuth bool, handlerFunc HandlerFunc) {
	key := method + ":" + route
	re := regexp.MustCompile("{([^}]+)}")
	matches := re.FindAllStringSubmatch(route, -1)
	handler := Handler{
		handler:      handlerFunc,
		requiresAuth: requiresAuth,
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

func (s *Server) GET(route string, requiresAuth bool, handler HandlerFunc) {
	s.addRoute(route, "GET", requiresAuth, handler)
}

func (s *Server) POST(route string, requiresAuth bool, handler HandlerFunc) {
	s.addRoute(route, "POST", requiresAuth, handler)
}

func (s *Server) PUT(route string, requiresAuth bool, handler HandlerFunc) {
	s.addRoute(route, "PUT", requiresAuth, handler)
}

func (s *Server) DELETE(route string, requiresAuth bool, handler HandlerFunc) {
	s.addRoute(route, "DELETE", requiresAuth, handler)
}

func (s *Server) PATCH(route string, requiresAuth bool, handler HandlerFunc) {
	s.addRoute(route, "PATCH", requiresAuth, handler)
}

func (s *Server) UseMiddleware(middleware MiddlewareFunc) {
	s.middlewares = append(s.middlewares, middleware)
}

func (s *Server) UseAuth(authFunc AuthFunc) {
	s.auths = append(s.auths, authFunc)
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

func (s *Server) getRouteHandler(method string, route string, authed bool) (*Handler, bool) {
	handler, ok := s.routeHandlers[method+":"+route]
	if !ok {
		// TODO remove/fix this with better implementation
		if strings.Contains(route, ".") {
			lastIndex := strings.LastIndex(route, "/")
			return s.getRouteHandler(method, route[:lastIndex+1]+"*", authed)
		} else {
			return s.findHandlerRegex(method, route, authed)
		}
	}

	return &handler, ok
}

func (s *Server) findHandlerRegex(method string, route string, authed bool) (*Handler, bool) {
	key := method + ":"
	for k, v := range s.routeHandlers {
		if v.requiresAuth && !authed {
			continue
		}
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

	context := NewContext(ctx)

	index := -1

	for _, auth := range s.auths {
		authed := auth(context)
		if authed {
			context.authed = true
			break
		}
	}

	var next func()
	next = func() {
		index++
		if index < len(s.middlewares) {
			s.middlewares[index](context, next)
			return
		}
		method := string(ctx.Method())
		path := string(ctx.Path())
		handler, ok := s.getRouteHandler(method, path, context.Authed())
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
		err := handler.handler(context)
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
