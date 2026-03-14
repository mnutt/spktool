package templates

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
)

//go:embed assets/**
var assets embed.FS

type Repository struct {
	fs fs.FS
}

func New() *Repository {
	sub, _ := fs.Sub(assets, "assets")
	return &Repository{fs: sub}
}

func (r *Repository) ReadFile(name string) ([]byte, error) {
	data, err := fs.ReadFile(r.fs, name)
	if err != nil {
		return nil, fmt.Errorf("read template %s: %w", name, err)
	}
	return data, nil
}

func (r *Repository) StackNames() ([]string, error) {
	entries, err := fs.ReadDir(r.fs, "stacks")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func (r *Repository) StackFile(stack, file string) ([]byte, error) {
	return r.ReadFile(filepath.ToSlash(filepath.Join("stacks", stack, file)))
}

func (r *Repository) BoxFile(file string) ([]byte, error) {
	return r.ReadFile(filepath.ToSlash(filepath.Join("box", file)))
}

func (r *Repository) HelperFile(file string) ([]byte, error) {
	return r.ReadFile(filepath.ToSlash(filepath.Join("helpers", file)))
}
