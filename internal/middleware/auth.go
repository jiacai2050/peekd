package middleware

import (
	"crypto/subtle"
	"fmt"
	"net/http"
)

func NewBasicAuth(username, password string) (func(http.Handler) http.Handler, error) {
	if username == "" && password == "" {
		return func(next http.Handler) http.Handler {
			return next
		}, nil
	}
	if username == "" || password == "" {
		return nil, fmt.Errorf("basic auth requires both username and password")
	}

	expectedUsername := []byte(username)
	expectedPassword := []byte(password)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, providedPassword, ok := r.BasicAuth()
			if !ok ||
				subtle.ConstantTimeCompare([]byte(user), expectedUsername) != 1 ||
				subtle.ConstantTimeCompare([]byte(providedPassword), expectedPassword) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="Peekd"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}
