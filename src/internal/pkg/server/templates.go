package server

import (
	"html/template"
	"path/filepath"
)

func (s *Server) parseTemplates(rootDir string, funcMap template.FuncMap) *template.Template {
	cleanRoot := filepath.Clean(rootDir)
	root := template.Must(template.ParseGlob(cleanRoot + "/**/*.html"))
	return root
}
