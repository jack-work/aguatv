package main

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
)

var errNotAFile = errors.New("cast a files path or a files URL")

func filesURL(files *url.URL, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	p := ref
	u, err := url.Parse(ref)
	if err != nil {
		return "", errNotAFile
	}
	if u.Scheme != "" || u.Host != "" {
		if u.Scheme != files.Scheme || u.Host != files.Host || u.User != nil || u.RawQuery != "" {
			return "", fmt.Errorf("%w on %s", errNotAFile, files.Host)
		}
		p = u.Path
	}
	p = "/" + strings.TrimPrefix(p, "/")
	if p == "/" || strings.HasSuffix(p, "/") || path.Clean(p) != p || strings.ContainsAny(p, "\x00\r\n?#") {
		return "", errNotAFile
	}
	return (&url.URL{Scheme: files.Scheme, Host: files.Host, Path: p}).String(), nil
}
