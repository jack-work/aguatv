package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type noSource struct{}

func (noSource) Open(context.Context, *Cast, http.Header) (*http.Response, error) {
	return nil, errors.New("casting has no storage configured")
}

type keyServer interface {
	ServeKey(w http.ResponseWriter, r *http.Request, key string)
}

type dirSource string

func (d dirSource) Open(context.Context, *Cast, http.Header) (*http.Response, error) {
	return nil, errors.New("dirSource serves directly")
}

func (d dirSource) ServeKey(w http.ResponseWriter, r *http.Request, key string) {
	if strings.Contains(key, "..") {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(filepath.Join(string(d), filepath.FromSlash(key)))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || fi.IsDir() {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, fi.Name(), fi.ModTime(), f)
}
