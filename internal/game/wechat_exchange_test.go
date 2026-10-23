package game

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type wechatTransport func(*http.Request) (*http.Response, error)

func (fn wechatTransport) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func TestWeChatCode2SessionExchangesAndSanitizesErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		valid      bool
	}{
		{"valid", `{"openid":"account-a","session_key":"private-session-key","unionid":"private-union"}`, 200, true},
		{"bad-code", `{"errcode":40029,"errmsg":"private-code private-secret"}`, 200, false},
		{"missing-id", `{"session_key":"private-session-key"}`, 200, false},
		{"malformed", `{`, 200, false},
		{"multiple-values", `{"openid":"account-a"}{}`, 200, false},
		{"too-large", `{"openid":"account-a","padding":"` + strings.Repeat("x", 9000) + `"}`, 200, false},
		{"http-error", `private-code private-secret`, 500, false},
		{"redirect", ``, 302, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ex := WeChatAPIExchanger{AppID: "app-id", AppSecret: "private-secret", Client: &http.Client{Transport: wechatTransport(func(r *http.Request) (*http.Response, error) {
				q := r.URL.Query()
				if r.Method != "GET" || r.URL.Scheme != "https" || r.URL.Host != "api.weixin.qq.com" || r.URL.Path != "/sns/jscode2session" || q.Get("appid") != "app-id" || q.Get("secret") != "private-secret" || q.Get("js_code") != "private-code" || q.Get("grant_type") != "authorization_code" {
					t.Fatalf("wrong code2Session request: method/path/parameters")
				}
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) > 11*time.Second {
					t.Fatal("exchange lacks bounded timeout")
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: http.Header{"Location": []string{"https://evil.example/"}}, Request: r}, nil
			})}}
			openid, err := ex.Exchange(context.Background(), "private-code")
			if tc.valid {
				if err != nil || openid != "account-a" {
					t.Fatalf("valid exchange: %q %v", openid, err)
				}
			} else if err == nil || openid != "" || strings.Contains(err.Error(), "private-") {
				t.Fatalf("unsafe exchange: %q %v", openid, err)
			}
		})
	}
}

func TestWeChatCode2SessionTransportErrorsDoNotLeakCredentials(t *testing.T) {
	ex := WeChatAPIExchanger{AppID: "app-id", AppSecret: "private-secret", Client: &http.Client{Transport: wechatTransport(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("private-code private-secret")
	})}}
	if id, err := ex.Exchange(context.Background(), "private-code"); err == nil || id != "" || strings.Contains(err.Error(), "private-") {
		t.Fatalf("transport leak: %q %v", id, err)
	}
}
