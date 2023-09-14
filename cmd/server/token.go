package main

import (
	"github.com/valyala/fasthttp"
	"time"
)

func createTokenCookies(sToken string, rToken string, delete bool) (*fasthttp.Cookie, *fasthttp.Cookie) {
	maxAge := 10
	refreshAge := 24 * 30
	if delete {
		maxAge = -1
		refreshAge = -1
	}

	sCookie := &fasthttp.Cookie{}
	sCookie.SetKey("SessionToken")
	sCookie.SetValue(sToken)
	sCookie.SetPath("/")
	sCookie.SetDomain("")
	sCookie.SetExpire(time.Now().Add(time.Duration(maxAge) * time.Minute))
	sCookie.SetSameSite(fasthttp.CookieSameSiteStrictMode)
	sCookie.SetSecure(true)

	rCookie := &fasthttp.Cookie{}
	rCookie.SetKey("RefreshToken")
	rCookie.SetValue(rToken)
	rCookie.SetPath("/")
	rCookie.SetDomain("")
	rCookie.SetExpire(time.Now().Add(time.Duration(refreshAge) * time.Hour))
	rCookie.SetSameSite(fasthttp.CookieSameSiteStrictMode)
	rCookie.SetSecure(true)
	rCookie.SetHTTPOnly(true)

	return sCookie, rCookie
}
