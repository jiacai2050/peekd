package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewBasicAuth(t *testing.T) {
	tests := []struct {
		name           string
		username       string
		password       string
		setCredentials bool
		status         int
	}{
		{name: "disabled", status: http.StatusNoContent},
		{name: "valid", username: "admin", password: "secret", setCredentials: true, status: http.StatusNoContent},
		{name: "invalid", username: "admin", password: "secret", status: http.StatusUnauthorized},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			middleware, err := NewBasicAuth(test.username, test.password)
			if err != nil {
				t.Fatalf("new basic auth: %v", err)
			}
			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))

			request := httptest.NewRequest(http.MethodGet, "/", nil)
			if test.setCredentials {
				request.SetBasicAuth("admin", "secret")
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != test.status {
				t.Fatalf("status code = %d, want %d", response.Code, test.status)
			}
			if test.status == http.StatusUnauthorized &&
				response.Header().Get("WWW-Authenticate") != `Basic realm="Peekd"` {
				t.Fatalf("missing authentication challenge: %q", response.Header().Get("WWW-Authenticate"))
			}
		})
	}
}

func TestNewBasicAuthRequiresBothCredentials(t *testing.T) {
	for _, test := range []struct {
		username string
		password string
	}{
		{username: "admin"},
		{password: "secret"},
	} {
		if _, err := NewBasicAuth(test.username, test.password); err == nil {
			t.Fatalf("NewBasicAuth(%q, %q) returned nil error", test.username, test.password)
		}
	}
}
