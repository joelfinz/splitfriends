// Package clientip resolves the real client address and stores it in the
// request context for the store and audit log.
package clientip

import (
	"net"
	"net/http"
	"strings"

	"splitfriends/internal/store"
)

// Middleware picks the client IP. With trustProxy it honours CF-Connecting-IP,
// then the first X-Forwarded-For entry, then X-Real-IP; otherwise the socket.
func Middleware(trustProxy bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := Resolve(r, trustProxy)
			r.RemoteAddr = ip // so downstream loggers agree
			next.ServeHTTP(w, r.WithContext(store.WithIP(r.Context(), ip)))
		})
	}
}

func Resolve(r *http.Request, trustProxy bool) string {
	if trustProxy {
		for _, h := range []string{"CF-Connecting-IP", "X-Forwarded-For", "X-Real-IP"} {
			v := strings.TrimSpace(r.Header.Get(h))
			if v == "" {
				continue
			}
			if i := strings.IndexByte(v, ','); i >= 0 {
				v = strings.TrimSpace(v[:i])
			}
			if ip := net.ParseIP(strings.Trim(v, "[]")); ip != nil {
				return ip.String()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
