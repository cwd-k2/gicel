package protocol

import (
	"net/url"
	"runtime"
	"strings"
)

// URIToPath converts a file:// URI to a filesystem path.
func URIToPath(uri DocumentURI) string {
	u, err := url.Parse(string(uri))
	if err != nil {
		return string(uri)
	}
	path := u.Path
	// Windows: strip leading / from /C:/...
	if runtime.GOOS == "windows" && len(path) > 2 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return path
}

// PathToURI converts a filesystem path to a file:// URI.
func PathToURI(path string) DocumentURI {
	if runtime.GOOS == "windows" {
		path = "/" + strings.ReplaceAll(path, `\`, "/")
	}
	return DocumentURI("file://" + path)
}
