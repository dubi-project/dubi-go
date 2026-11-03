package view

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type source struct {
	name  string
	asset bool
}

func validName(name string) bool {
	return name != "." && fs.ValidPath(name) && !strings.ContainsAny(name, "\\:\x00\r\n")
}

func (e *engine) find(name, site, language, relative string) (source, error) {
	if !validName(name) {
		return source{}, fmt.Errorf("view: invalid template name %q", name)
	}
	for _, scope := range []string{site, language} {
		if scope != "" && (!validName(scope) || strings.Contains(scope, "/")) {
			return source{}, fmt.Errorf("view: invalid site or language %q", scope)
		}
	}
	root := filepath.ToSlash(e.config.Root)
	directories := []string{}
	if relative != "" {
		directories = append(directories, relative)
	}
	add := func(parts ...string) {
		directory := path.Join(parts...)
		directories = append(directories, directory)
		if e.config.Shared != "" {
			directories = append(directories, path.Join(directory, e.config.Shared))
		}
	}
	if site != "" {
		if language != "" {
			add(root, site, language)
		}
		add(root, site)
	}
	if language != "" {
		add(root, language)
	}
	add(root)
	suffixes := []string{name}
	if path.Ext(name) == "" {
		suffixes = []string{name + ".html", path.Join(name, "index.html")}
	}
	seen := map[string]bool{}
	for _, directory := range directories {
		for _, suffix := range suffixes {
			filename := path.Join(directory, suffix)
			if seen[filename] {
				continue
			}
			seen[filename] = true
			if e.fsys != nil && fs.ValidPath(filename) {
				info, err := fs.Stat(e.fsys, filename)
				if err == nil && info.Mode().IsRegular() {
					return source{filename, true}, nil
				}
				if err != nil && !errors.Is(err, fs.ErrNotExist) {
					return source{}, fmt.Errorf("view: stat %s: %w", filename, err)
				}
			}
			info, err := os.Stat(filepath.FromSlash(filename))
			if err == nil && info.Mode().IsRegular() {
				return source{filename, false}, nil
			}
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return source{}, fmt.Errorf("view: stat %s: %w", filename, err)
			}
		}
	}
	return source{}, fmt.Errorf("view: template %q: %w", name, fs.ErrNotExist)
}

func (e *engine) read(file source) ([]byte, error) {
	if file.asset {
		return fs.ReadFile(e.fsys, file.name)
	}
	return os.ReadFile(filepath.FromSlash(file.name))
}
