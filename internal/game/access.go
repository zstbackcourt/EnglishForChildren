package game

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
)

// FamilyAccess adds one household password when hosted outside a trusted LAN.
// Individual characters remain selectable by all members of the household.
func FamilyAccess(next http.Handler, password string) http.Handler {
	if password == "" {
		return next
	}
	expected := sha256.Sum256([]byte(password))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" && (r.Method == "GET" || r.Method == "HEAD") {
			next.ServeHTTP(w, r)
			return
		}
		user, supplied, ok := r.BasicAuth()
		actual := sha256.Sum256([]byte(supplied))
		if !ok || user != "family" || subtle.ConstantTimeCompare(actual[:], expected[:]) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="English Adventure", charset="UTF-8"`)
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "Family password required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
