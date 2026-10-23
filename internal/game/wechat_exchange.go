package game

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// WeChatAPIExchanger calls WeChat's server-side code2Session API. Client can be
// supplied for controlled transports; deadline, redirect and size limits remain enforced.
type WeChatAPIExchanger struct {
	AppID     string
	AppSecret string
	Client    *http.Client
}

func (e WeChatAPIExchanger) Exchange(ctx context.Context, code string) (string, error) {
	// Never return the transport error: net/http errors can include the URL and secret.
	failed := errors.New("WeChat login exchange failed")
	if e.AppID == "" || e.AppSecret == "" || strings.TrimSpace(code) == "" || len(code) > 1024 {
		return "", failed
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	query := url.Values{"appid": {e.AppID}, "secret": {e.AppSecret}, "js_code": {code}, "grant_type": {"authorization_code"}}
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.weixin.qq.com/sns/jscode2session?"+query.Encode(), nil)
	if err != nil {
		return "", failed
	}
	client := http.Client{Timeout: 10 * time.Second}
	if e.Client != nil {
		client = *e.Client
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(r)
	if err != nil {
		return "", failed
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", failed
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 8193))
	if err != nil || len(body) > 8192 {
		return "", failed
	}
	var session struct {
		OpenID  string `json:"openid"`
		ErrCode int    `json:"errcode"`
	}
	if json.Unmarshal(body, &session) != nil || session.ErrCode != 0 || strings.TrimSpace(session.OpenID) == "" || len(session.OpenID) > 256 {
		return "", failed
	}
	return session.OpenID, nil
}
