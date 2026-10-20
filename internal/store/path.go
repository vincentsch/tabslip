package store

import (
	"net/url"
	"path/filepath"
	"strings"
)

// SQLite file URIs need forward slashes and an empty authority for drive paths.
func databaseURL(path string) url.URL {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return url.URL{Scheme: "file", Path: path}
}
