package game

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type wechatTestExchanger struct{}

func (wechatTestExchanger) Exchange(_ context.Context, code string) (string, error) {
	switch code {
	case "alice-code", "alice-code-again":
		return "openid-alice", nil
	case "bob-code":
		return "openid-bob", nil
	default:
		return "", errors.New("upstream secret must never be exposed")
	}
}

func wechatRequest(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:1234"
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func wechatLogin(t *testing.T, h http.Handler, code string) string {
	t.Helper()
	w := wechatRequest(h, "POST", "/api/session", "", `{"code":"`+code+`"}`)
	var result struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expiresAt"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Token == "" {
		t.Fatalf("login: %d %s", w.Code, w.Body)
	}
	if _, err := time.Parse(time.RFC3339, result.ExpiresAt); err != nil {
		t.Fatalf("expiresAt must be RFC3339: %q", result.ExpiresAt)
	}
	if strings.Contains(w.Body.String(), "openid") || strings.Contains(w.Body.String(), "session_key") {
		t.Fatal("login exposed upstream credentials")
	}
	return result.Token
}

func wechatHandler(t *testing.T, c *Catalog, root, save string, now func() time.Time) http.Handler {
	t.Helper()
	h, err := NewWeChatHandler(root, c, WeChatConfig{
		SaveDir: save, SessionKey: []byte("0123456789abcdef0123456789abcdef"),
		Exchanger: wechatTestExchanger{}, Now: now, SessionTTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestWeChatRejectsInvalidLoginAndTokens(t *testing.T) {
	c, _, root := fixture(t)
	clock := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	save := filepath.Join(root, "wechat")
	h := wechatHandler(t, c, root, save, func() time.Time { return clock })
	for _, body := range []string{`{"code":"bad"}`, `{"code":""}`, `{"code":"dev:local-family"}`} {
		w := wechatRequest(h, "POST", "/api/session", "", body)
		if w.Code != 401 || strings.Contains(w.Body.String(), "upstream") {
			t.Fatalf("bad code: %d %s", w.Code, w.Body)
		}
	}
	if _, err := os.Stat(save); !os.IsNotExist(err) {
		t.Fatal("failed login created account data")
	}
	token := wechatLogin(t, h, "alice-code")
	for _, bad := range []string{"", "forged", token + "x"} {
		for _, path := range []string{"/api/health", "/api/lessons", "/api/players", "/api/backup"} {
			if w := wechatRequest(h, "GET", path, bad, ""); w.Code != 401 {
				t.Fatalf("unauthorized %s: %d", path, w.Code)
			}
		}
	}
	if w := wechatRequest(h, "GET", "/api/lessons", token, ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	clock = clock.Add(time.Hour)
	if w := wechatRequest(h, "GET", "/api/players", token, ""); w.Code != 401 {
		t.Fatalf("expired token: %d", w.Code)
	}
}

func TestWeChatAccountsIsolateProgressAwardsBackupsAndPersist(t *testing.T) {
	c, _, root := fixture(t)
	save := filepath.Join(root, "wechat")
	h := wechatHandler(t, c, root, save, time.Now)
	alice, bob := wechatLogin(t, h, "alice-code"), wechatLogin(t, h, "bob-code")
	w := wechatRequest(h, "POST", "/api/players", alice, `{"name":"Alice","avatar":"fox"}`)
	var player Player
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &player) != nil {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	p := "/api/players/" + player.ID
	w = wechatRequest(h, "POST", p+"/attempts", alice, `{"lessonId":1,"difficulty":"easy"}`)
	var attempt Attempt
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &attempt) != nil {
		t.Fatalf("attempt: %d %s", w.Code, w.Body)
	}
	answers := make([]string, len(attempt.Questions))
	spoken := map[float64]string{1: "Excuse me!", 2: "Is this your handbag?", 3: "Yes it is.", 4: "Thank you very much."}
	for i, q := range attempt.Questions {
		answers[i] = spoken[q.Start]
	}
	body, _ := json.Marshal(map[string]any{"answers": answers})
	for _, tc := range []struct{ method, path, body string }{
		{"GET", p, ""},
		{"PUT", p + "/progress", `{"lessonId":1,"cursor":2,"difficulty":"easy"}`},
		{"POST", p + "/attempts", `{"lessonId":1,"difficulty":"easy"}`},
		{"POST", p + "/attempts/" + attempt.ID, string(body)},
	} {
		if w := wechatRequest(h, tc.method, tc.path, bob, tc.body); w.Code != 404 {
			t.Fatalf("cross-account %s %s: %d %s", tc.method, tc.path, w.Code, w.Body)
		}
	}
	if w := wechatRequest(h, "PUT", p+"/progress", alice, `{"lessonId":1,"cursor":2,"difficulty":"easy"}`); w.Code != 200 {
		t.Fatal(w.Code)
	}
	w = wechatRequest(h, "POST", p+"/attempts/"+attempt.ID, alice, string(body))
	var award Result
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &award) != nil || award.StarsEarned != 10 {
		t.Fatalf("award: %d %s", w.Code, w.Body)
	}
	for _, path := range []string{"/api/players", "/api/backup"} {
		w := wechatRequest(h, "GET", path, bob, "")
		if w.Code != 200 || strings.Contains(w.Body.String(), player.ID) || strings.Contains(w.Body.String(), "Alice") {
			t.Fatalf("backup leak: %d %s", w.Code, w.Body)
		}
	}
	entries, err := os.ReadDir(save)
	if err != nil || len(entries) != 1 {
		t.Fatalf("account storage: %v %v", entries, err)
	}
	if strings.Contains(entries[0].Name(), "alice") {
		t.Fatal("openid used as storage path")
	}
	h = wechatHandler(t, c, root, save, time.Now)
	alice = wechatLogin(t, h, "alice-code-again")
	w = wechatRequest(h, "GET", p, alice, "")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &player) != nil || player.Stars != 10 || player.Progress[1].Cursor != 2 {
		t.Fatalf("restart: %d %s", w.Code, w.Body)
	}
	w = wechatRequest(h, "POST", p+"/attempts/"+attempt.ID, alice, string(body))
	if w.Code != 200 {
		t.Fatalf("persisted retry: %d %s", w.Code, w.Body)
	}
	w = wechatRequest(h, "GET", p, alice, "")
	if json.Unmarshal(w.Body.Bytes(), &player) != nil || player.Stars != 10 || player.Progress[1].Levels["easy"].Attempts != 1 {
		t.Fatalf("duplicate award after restart: %s", w.Body)
	}
}

func TestWeChatPublicMediaIsReadOnlyAndConfined(t *testing.T) {
	c, _, root := fixture(t)
	for _, dir := range []string{"scenes", "pages"} {
		base := filepath.Join(root, "web", "assets", dir)
		if err := os.MkdirAll(base, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(base, "lesson.png"), []byte("public-image"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(root, "save.json"), filepath.Join(base, "leak.png")); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "save.json"), []byte("private-save"), 0600); err != nil {
		t.Fatal(err)
	}
	h := wechatHandler(t, c, root, filepath.Join(root, "wechat"), time.Now)
	for _, path := range []string{"/assets/scenes/lesson.png", "/assets/pages/lesson.png", "/media/audio/1"} {
		if w := wechatRequest(h, "GET", path, "", ""); w.Code != 200 {
			t.Fatalf("public %s: %d", path, w.Code)
		}
		if w := wechatRequest(h, "HEAD", path, "", ""); w.Code != 200 || w.Body.Len() != 0 {
			t.Fatalf("HEAD %s: %d %s", path, w.Code, w.Body)
		}
		if w := wechatRequest(h, "POST", path, "", "overwrite"); w.Code != 405 {
			t.Fatalf("write %s: %d", path, w.Code)
		}
	}
	r := httptest.NewRequest("GET", "/media/audio/1", nil)
	r.Header.Set("Range", "bytes=2-5")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 206 || w.Body.String() != "2345" || w.Header().Get("Content-Range") != "bytes 2-5/10" {
		t.Fatalf("range: %d %s", w.Code, w.Body)
	}
	for _, path := range []string{"/", "/index.html", "/data/wechat/save.json", "/save.json", "/assets/scenes/", "/assets/scenes/leak.png", "/assets/pages/leak.png", "/assets/scenes/../../save.json", "/assets/scenes/%2e%2e/%2e%2e/save.json", "/assets/scenes/%2f..%2f..%2fsave.json", "/media/audio/999", "/media/audio/2", "/media/audio/1/../save.json"} {
		w := wechatRequest(h, "GET", path, "", "")
		if w.Code != 404 || strings.Contains(w.Body.String(), "private-save") {
			t.Fatalf("path %s: %d %s", path, w.Code, w.Body)
		}
	}
}

func TestWeChatAudioCatalogCannotEscapeRoot(t *testing.T) {
	c, _, root := fixture(t)
	outside := filepath.Join(t.TempDir(), "private.mp3")
	if err := os.WriteFile(outside, []byte("private-save"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "leak.mp3")); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{outside, "../private.mp3", "leak.mp3", "save.json"} {
		c.Lessons[0].AudioFile = file
		h := wechatHandler(t, c, root, filepath.Join(root, "wechat"), time.Now)
		if w := wechatRequest(h, "GET", "/media/audio/1", "", ""); w.Code != 404 {
			t.Fatalf("audio path %s: %d %s", file, w.Code, w.Body)
		}
	}
}

func TestWeChatDevLoginRequiresLoopbackListenerAndRequest(t *testing.T) {
	c, _, root := fixture(t)
	config := WeChatConfig{SessionKey: []byte("0123456789abcdef0123456789abcdef"), DevLogin: true, ListenAddr: "127.0.0.1:8093"}
	for _, addr := range []string{"", ":8093", "0.0.0.0:8093", "[::]:8093", "192.168.1.2:8093", "localhost:8093"} {
		config.ListenAddr = addr
		if _, err := NewWeChatHandler(root, c, config); err == nil {
			t.Fatalf("unsafe dev listener accepted: %s", addr)
		}
	}
	for _, addr := range []string{"127.0.0.1:8093", "[::1]:8093"} {
		config.ListenAddr = addr
		h, err := NewWeChatHandler(root, c, config)
		if err != nil {
			t.Fatal(err)
		}
		token := wechatLogin(t, h, "dev:local-family")
		if w := wechatRequest(h, "POST", "/api/session", "", `{"code":"alice-code"}`); w.Code != 401 {
			t.Fatalf("dev accepted other identity: %d", w.Code)
		}
		for _, path := range []string{"/api/session", "/api/players", "/media/audio/1"} {
			r := httptest.NewRequest("POST", path, strings.NewReader(`{"code":"dev:local-family"}`))
			r.RemoteAddr = "192.0.2.10:1234"
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set("X-Forwarded-For", "127.0.0.1")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 403 {
				t.Fatalf("remote dev request %s: %d", path, w.Code)
			}
		}
	}
}

func TestWeChatRejectsWeakSessionConfiguration(t *testing.T) {
	c, _, root := fixture(t)
	for _, key := range []string{"", "short"} {
		if _, err := NewWeChatHandler(root, c, WeChatConfig{SessionKey: []byte(key), Exchanger: wechatTestExchanger{}}); err == nil {
			t.Fatal("weak signing key accepted")
		}
	}
}

func TestWeChatConcurrentRequestsUseOneAccountStore(t *testing.T) {
	c, _, root := fixture(t)
	h := wechatHandler(t, c, root, filepath.Join(root, "wechat"), time.Now)
	token := wechatLogin(t, h, "alice-code")
	var workers sync.WaitGroup
	for i := 0; i < 12; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if w := wechatRequest(h, "POST", "/api/players", token, `{"name":"Alice","avatar":"fox"}`); w.Code != 201 {
				t.Errorf("concurrent create: %d %s", w.Code, w.Body)
			}
		}()
	}
	workers.Wait()
	w := wechatRequest(h, "GET", "/api/players", token, "")
	var players []Player
	if json.Unmarshal(w.Body.Bytes(), &players) != nil || len(players) != 12 {
		t.Fatalf("lost concurrent writes: %s", w.Body)
	}
}

func TestWeChatAllowsExplicitCourseArchiveSymlink(t *testing.T) {
	c, _, root := fixture(t)
	archive := t.TempDir()
	if err := os.WriteFile(filepath.Join(archive, "lesson.mp3"), []byte("course-audio"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(archive, filepath.Join(root, "New-Concept-English")); err != nil {
		t.Fatal(err)
	}
	c.Lessons[0].AudioFile = "New-Concept-English/lesson.mp3"
	h := wechatHandler(t, c, root, filepath.Join(root, "wechat"), time.Now)
	if w := wechatRequest(h, "GET", "/media/audio/1", "", ""); w.Code != 200 || w.Body.String() != "course-audio" {
		t.Fatalf("course archive: %d %s", w.Code, w.Body)
	}
}
