package httpapi

import (
	"crypto/subtle"
	"net/http"
)

// basicAuthMiddleware requires HTTP Basic Auth credentials matching
// username/password on every request except /api/health (left open for
// container health checks and uptime monitors). Comparisons go through
// subtle.ConstantTimeCompare to avoid leaking timing information about how
// much of the guess was correct.
//
// This exists for standalone deployments (bare Docker/binary) that expose
// the HTTP API directly on a LAN. It is not needed — and not wired up — when
// running as the Home Assistant add-on, since HA ingress already gates
// access via the user's HA session.
//
// It protects against casual/opportunistic access, not network-level
// eavesdropping: credentials travel base64-encoded, not encrypted. Put this
// behind a TLS-terminating reverse proxy for anything reachable over the
// open internet.
func basicAuthMiddleware(username, password string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/health" {
				next.ServeHTTP(w, r)
				return
			}
			u, p, ok := r.BasicAuth()
			if !ok ||
				subtle.ConstantTimeCompare([]byte(u), []byte(username)) != 1 ||
				subtle.ConstantTimeCompare([]byte(p), []byte(password)) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="digitalSTROM vDC Bridge"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
