package server

import (
	"fmt"
	"github.com/fsnotify/fsnotify"
	"html/template"
	"io/fs"
	"log"
	"path/filepath"
)

func (s *Server) parseTemplates(rootDir string, funcMap template.FuncMap) *template.Template {
	cleanRoot := filepath.Clean(rootDir)
	root := template.Must(template.ParseGlob(cleanRoot + "/*.html"))
	return root
}

func (s *Server) watchTemplates(rootDir string) {
	cleanRoot := filepath.Clean(rootDir)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Println("failed to start template watcher")
	}
	defer watcher.Close()
	err = filepath.WalkDir(cleanRoot, func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			return watcher.Add(path)
		}
		return nil
	})
	if err != nil {
		return
	}
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				fmt.Println("File modified: ", event.Name, " reloading templates")
				s.templates = s.parseTemplates(rootDir, nil)
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Println("Error:", err)
		}
	}
}
