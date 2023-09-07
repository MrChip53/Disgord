package server

import (
	"html/template"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) parseTemplates(rootDir string, funcMap template.FuncMap) (*template.Template, error) {
	cleanRoot := filepath.Clean(rootDir)
	pfx := len(cleanRoot) + 1
	root := template.New("")

	err := filepath.Walk(cleanRoot, func(path string, info os.FileInfo, e1 error) error {
		if !info.IsDir() && strings.HasSuffix(path, ".html") {
			if e1 != nil {
				return e1
			}
			templateBytes, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			templateName := path[pfx : len(path)-5]
			t := root.New(templateName).Funcs(funcMap)
			_, err = t.Parse(string(templateBytes))
			if err != nil {
				return err
			}
		}
		return nil
	})

	return root, err
}
