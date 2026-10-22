package game

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFamilyAccessProtectsSaveAndMedia(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	for _, path := range []string{"/", "/api/backup", "/api/players", "/media/audio/1"} {
		for _, password := range []string{"", "wrong", "secret"} {
			r := httptest.NewRequest("GET", path, nil)
			if password != "" {
				r.SetBasicAuth("family", password)
			}
			w := httptest.NewRecorder()
			FamilyAccess(next, "secret").ServeHTTP(w, r)
			want := 401
			if password == "secret" {
				want = 204
			}
			if w.Code != want {
				t.Fatalf("%s: got %d want %d", path, w.Code, want)
			}
		}
	}
	for _, password := range []string{"", "secret"} {
		w := httptest.NewRecorder()
		FamilyAccess(next, password).ServeHTTP(w, httptest.NewRequest("GET", "/api/health", nil))
		if w.Code != 204 {
			t.Fatal("health check inaccessible")
		}
	}
	w := httptest.NewRecorder()
	FamilyAccess(next, "").ServeHTTP(w, httptest.NewRequest("GET", "/api/players", nil))
	if w.Code != 204 {
		t.Fatal("local mode should remain accessible")
	}
}
