package main

import "html/template"

func getFuncMap() template.FuncMap {
	return template.FuncMap{
		"getFirstLetter": getFirstLetter,
	}
}

func getFirstLetter(s string) string {
	return string(s[0])
}
