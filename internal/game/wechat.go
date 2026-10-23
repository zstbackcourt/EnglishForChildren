package game

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// WeChatCodeExchanger exchanges a short-lived wx.login code for a server-only identity.
type WeChatCodeExchanger interface {
	Exchange(context.Context, string) (string, error)
}

type WeChatConfig struct {
	Backend    MySQLAccountBackend
	SaveDir    string
	ReleaseID  string
	SessionKey []byte
	Exchanger  WeChatCodeExchanger
	SessionTTL time.Duration
	Now        func() time.Time
	DevLogin   bool
	ListenAddr string
}

type wechatClaims struct {
	Subject string `json:"sub"`
	Expires int64  `json:"exp"`
}

type wechatGateway struct {
	root     string
	catalog  *Catalog
	config   WeChatConfig
	mu       sync.Mutex
	accounts map[string]http.Handler
}

// NewWeChatHandler provides a separate authenticated API without the web family store.
// Each identity has its own game handler, quiz cache and durable save file.
func NewWeChatHandler(root string, catalog *Catalog, config WeChatConfig) (http.Handler, error) {
	if len(config.SessionKey) < 32 {
		return nil, errors.New("WECHAT_SESSION_KEY must contain at least 32 bytes")
	}
	if config.DevLogin {
		if !wechatLoopback(config.ListenAddr) {
			return nil, errors.New("development login requires a literal loopback listen address")
		}
	} else if config.Exchanger == nil {
		return nil, errors.New("WeChat code exchanger is required")
	}
	if catalog == nil {
		return nil, errors.New("WeChat catalog is required")
	}
	if config.SaveDir == "" {
		config.SaveDir = filepath.Join("data", "wechat")
	}
	if !filepath.IsAbs(config.SaveDir) {
		config.SaveDir = filepath.Join(root, config.SaveDir)
	}
	if config.SessionTTL == 0 {
		config.SessionTTL = 7 * 24 * time.Hour
	}
	if config.SessionTTL < time.Second {
		return nil, errors.New("session lifetime must be at least one second")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	config.SessionKey = append([]byte(nil), config.SessionKey...)
	return &wechatGateway{root: root, catalog: catalog, config: config, accounts: make(map[string]http.Handler)}, nil
}

func (g *wechatGateway) sign(payload string) string {
	mac := hmac.New(sha256.New, g.config.SessionKey)
	mac.Write([]byte("wechat-session-v1." + payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (g *wechatGateway) identity(token string) (string, bool) {
	if len(token) > 1024 {
		return "", false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return "", false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", false
	}
	expected, _ := base64.RawURLEncoding.DecodeString(g.sign(parts[0]))
	if !hmac.Equal(sig, expected) {
		return "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	var claims wechatClaims
	if json.Unmarshal(payload, &claims) != nil || claims.Expires <= g.config.Now().Unix() {
		return "", false
	}
	subject, err := hex.DecodeString(claims.Subject)
	if err != nil || len(subject) != sha256.Size || claims.Subject != hex.EncodeToString(subject) {
		return "", false
	}
	return claims.Subject, true
}

func wechatUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	reply(w, http.StatusUnauthorized, map[string]string{"error": "登录已失效，请重新登录"})
}

func wechatLoopback(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (g *wechatGateway) session(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		reply(w, http.StatusMethodNotAllowed, map[string]string{"error": "请使用 POST 登录"})
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Code) == "" || len(body.Code) > 1024 {
		wechatUnauthorized(w)
		return
	}
	var openid string
	var err error
	if g.config.DevLogin {
		if body.Code == "dev:local-family" {
			openid = "dev:local-family"
		}
	} else {
		openid, err = g.config.Exchanger.Exchange(r.Context(), body.Code)
	}
	if err != nil || openid == "" || len(openid) > 256 {
		wechatUnauthorized(w)
		return
	}
	digest := sha256.Sum256([]byte(openid))
	expires := g.config.Now().Add(g.config.SessionTTL).UTC().Truncate(time.Second)
	payload, _ := json.Marshal(wechatClaims{Subject: hex.EncodeToString(digest[:]), Expires: expires.Unix()})
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	reply(w, http.StatusOK, map[string]string{"token": encoded + "." + g.sign(encoded), "expiresAt": expires.Format(time.RFC3339)})
}

func (g *wechatGateway) account(subject string) (http.Handler, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if handler, ok := g.accounts[subject]; ok {
		return handler, nil
	}
	store, err := OpenStore(filepath.Join(g.config.SaveDir, subject, "save.json"))
	if err != nil {
		return nil, err
	}
	handler := NewHandler(g.root, g.catalog, store)
	g.accounts[subject] = handler
	return handler, nil
}

func (g *wechatGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if g.config.DevLogin && !wechatLoopback(r.RemoteAddr) {
		reply(w, http.StatusForbidden, map[string]string{"error": "开发模式只允许本机请求"})
		return
	}
	if path.Clean(r.URL.Path) != r.URL.Path || strings.Contains(r.URL.Path, "\\") {
		http.NotFound(w, r)
		return
	}
	if r.URL.Path == "/healthz" {
		if g.config.Backend != nil {
			if err := g.config.Backend.Ready(r.Context()); err != nil {
				reply(w, 503, map[string]string{"status": "unavailable"})
				return
			}
		}
		response := map[string]string{"status": "ok"}
		if g.config.ReleaseID != "" {
			response["releaseId"] = g.config.ReleaseID
		}
		reply(w, 200, response)
		return
	}
	if r.URL.Path == "/api/session" {
		g.session(w, r)
		return
	}
	if g.media(w, r) {
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		wechatUnauthorized(w)
		return
	}
	subject, ok := g.identity(strings.TrimPrefix(header, "Bearer "))
	if !ok {
		wechatUnauthorized(w)
		return
	}
	if g.config.Backend != nil {
		g.durableAccount(w, r, subject)
		return
	}
	handler, err := g.account(subject)
	if err != nil {
		reply(w, http.StatusInternalServerError, map[string]string{"error": "存档暂时不可用"})
		return
	}
	handler.ServeHTTP(w, r)
}
