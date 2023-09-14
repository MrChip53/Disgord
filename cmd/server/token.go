package main

import "github.com/valyala/fasthttp"

func createTokenCookies(sToken string, rToken string) (*fasthttp.Cookie, *fasthttp.Cookie) {
	sCookie := &fasthttp.Cookie{}
	sCookie.SetKey("SessionToken")
	sCookie.SetValue(sToken)
	sCookie.SetPath("/")
	sCookie.SetDomain("")
	sCookie.SetMaxAge(60)
	sCookie.SetSameSite(fasthttp.CookieSameSiteStrictMode)
	sCookie.SetSecure(true)

	rCookie := &fasthttp.Cookie{}
	rCookie.SetKey("RefreshToken")
	rCookie.SetValue(rToken)
	rCookie.SetPath("/")
	rCookie.SetDomain("")
	rCookie.SetMaxAge(60)
	rCookie.SetSameSite(fasthttp.CookieSameSiteStrictMode)
	rCookie.SetSecure(true)

	return sCookie, rCookie
}
