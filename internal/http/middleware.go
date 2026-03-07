package httpapi

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func managementAuthMiddleware(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			headers := []string{"X-Management-Key", "Authorization"}
			provided := ""
			for _, header := range headers {
				value := strings.TrimSpace(r.Header.Get(header))
				if value == "" {
					continue
				}
				if strings.HasPrefix(strings.ToLower(value), "bearer ") {
					value = strings.TrimSpace(value[7:])
				}
				provided = value
				break
			}

			if subtle.ConstantTimeCompare([]byte(provided), []byte(apiKey)) != 1 {
				writeError(w, http.StatusUnauthorized, "unauthorized", "management authentication failed", nil)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
