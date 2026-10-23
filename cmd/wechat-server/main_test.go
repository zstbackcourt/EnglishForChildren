package main

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestWeChatServerRequiresProductionCredentials(t *testing.T) {
	for _, env := range []map[string]string{
		{},
		{"WECHAT_APP_ID": "app"},
		{"WECHAT_APP_ID": "app", "WECHAT_APP_SECRET": "private-secret"},
		{"WECHAT_APP_ID": "app", "WECHAT_APP_SECRET": "private-secret", "WECHAT_SESSION_KEY": "short"},
	} {
		_, err := newServer(nil, func(key string) string { return env[key] })
		if err == nil || strings.Contains(err.Error(), "private-secret") {
			t.Fatalf("credential validation: %v", err)
		}
	}
}

func TestWeChatServerDefaultsAndExplicitLocalDevelopment(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	getenv := func(key string) string {
		return map[string]string{"WECHAT_APP_ID": "app", "WECHAT_APP_SECRET": "private-secret", "WECHAT_SESSION_KEY": "0123456789abcdef0123456789abcdef", "PORT": "8888"}[key]
	}
	server, err := newServer([]string{"-root", root}, getenv)
	if err != nil {
		t.Fatal(err)
	}
	if server.Addr != "127.0.0.1:8093" || server.ReadHeaderTimeout == 0 || server.WriteTimeout == 0 {
		t.Fatalf("server defaults: %+v", server)
	}
	for _, addr := range []string{"0.0.0.0:8093", ":8093", "[::]:8093"} {
		if _, err := newServer([]string{"-root", root, "-dev-login", "-addr", addr}, func(string) string { return "" }); err == nil {
			t.Fatalf("public development listener accepted: %s", addr)
		}
	}
	server, err = newServer([]string{"-root", root, "-dev-login", "-save-dir", t.TempDir()}, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/api/session", strings.NewReader(`{"code":"dev:local-family"}`))
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	server.Handler.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"token"`) {
		t.Fatalf("development login: %d %s", w.Code, w.Body)
	}
}
