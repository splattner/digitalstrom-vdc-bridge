package httpapi

import (
	"embed"
	"io/fs"
	"net/http"
)

// webDist will be populated with the built web/dist contents at compile time.
// The go:embed directive is conditional: if web/dist is absent (dev mode),
// we serve a minimal placeholder instead.

//go:embed webdist
var embeddedDist embed.FS

func staticHandler() http.Handler {
	sub, err := fs.Sub(embeddedDist, "webdist")
	if err != nil {
		// Should never happen since webdist/ always exists in source.
		return http.NotFoundHandler()
	}
	return http.FileServer(http.FS(sub))
}
