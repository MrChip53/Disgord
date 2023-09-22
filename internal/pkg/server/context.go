package server

import "github.com/valyala/fasthttp"

type Context struct {
	ctx    *fasthttp.RequestCtx
	authed bool
}

func NewContext(ctx *fasthttp.RequestCtx) *Context {
	return &Context{ctx: ctx}
}

func (c *Context) Authed() bool {
	return c.authed
}

func (c *Context) Context() *fasthttp.RequestCtx {
	return c.ctx
}
