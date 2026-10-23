package game

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type wechatExchangeStage uint8

const (
	wechatExchangeConfiguration wechatExchangeStage = iota + 1
	wechatExchangeInput
	wechatExchangeRequest
	wechatExchangeTransport
	wechatExchangeHTTP
	wechatExchangeResponse
	wechatExchangeAPI
)

// No upstream error, URL, response text or credentials may be retained here.
// Numeric API codes are diagnostic data; all printable stages are fixed below.
type wechatExchangeError struct {
	stage   wechatExchangeStage
	errCode int
}

func (e *wechatExchangeError) Stage() string {
	if e != nil {
		switch e.stage {
		case wechatExchangeConfiguration:
			return "configuration"
		case wechatExchangeInput:
			return "input"
		case wechatExchangeRequest:
			return "request"
		case wechatExchangeTransport:
			return "transport"
		case wechatExchangeHTTP:
			return "http"
		case wechatExchangeResponse:
			return "response"
		case wechatExchangeAPI:
			return "wechat"
		}
	}
	return "unknown"
}

// Zero means that no WeChat API error code was received.
func (e *wechatExchangeError) ErrCode() int {
	if e != nil && e.stage == wechatExchangeAPI {
		return e.errCode
	}
	return 0
}

func (e *wechatExchangeError) Error() string {
	return "WeChat login exchange failed (stage=" + e.Stage() + ", errcode=" + strconv.Itoa(e.ErrCode()) + ")"
}

func (e *wechatExchangeError) reason() string {
	switch e.ErrCode() {
	case 40013, 40125: // Invalid AppID or AppSecret.
		return "credential_rejected"
	case 40029, 40163: // Invalid or already-used login code.
		return "invalid_login_code"
	default:
		// Do not infer configuration problems from other API or transport errors.
		return "unavailable"
	}
}

// WeChatAPIExchanger calls WeChat's server-side code2Session API. Client can be
// supplied for controlled transports; deadline, redirect and size limits remain enforced.
type WeChatAPIExchanger struct {
	AppID     string
	AppSecret string
	Client    *http.Client
}

func (e WeChatAPIExchanger) Exchange(ctx context.Context, code string) (string, error) {
	if e.AppID == "" || e.AppSecret == "" {
		return "", &wechatExchangeError{stage: wechatExchangeConfiguration}
	}
	if strings.TrimSpace(code) == "" || len(code) > 1024 {
		return "", &wechatExchangeError{stage: wechatExchangeInput}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	query := url.Values{"appid": {e.AppID}, "secret": {e.AppSecret}, "js_code": {code}, "grant_type": {"authorization_code"}}
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.weixin.qq.com/sns/jscode2session?"+query.Encode(), nil)
	if err != nil {
		return "", &wechatExchangeError{stage: wechatExchangeRequest}
	}
	client := http.Client{Timeout: 10 * time.Second}
	if e.Client != nil {
		client = *e.Client
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(r)
	if err != nil {
		// net/http errors can include the URL, AppSecret and login code.
		return "", &wechatExchangeError{stage: wechatExchangeTransport}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", &wechatExchangeError{stage: wechatExchangeHTTP}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 8193))
	if err != nil || len(body) > 8192 {
		return "", &wechatExchangeError{stage: wechatExchangeResponse}
	}
	var session struct {
		OpenID  string `json:"openid"`
		ErrCode int    `json:"errcode"`
	}
	if json.Unmarshal(body, &session) != nil {
		return "", &wechatExchangeError{stage: wechatExchangeResponse}
	}
	if session.ErrCode != 0 {
		return "", &wechatExchangeError{stage: wechatExchangeAPI, errCode: session.ErrCode}
	}
	if strings.TrimSpace(session.OpenID) == "" || len(session.OpenID) > 256 {
		return "", &wechatExchangeError{stage: wechatExchangeResponse}
	}
	return session.OpenID, nil
}
