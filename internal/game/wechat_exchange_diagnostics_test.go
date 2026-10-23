package game

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

const wechatDiagnosticSecret = "https://private.example/?secret=private-secret&js_code=private-code&openid=private-openid"

func TestWeChatCode2SessionSafeDiagnosticStages(t *testing.T) {
	for _, tc := range []struct {
		name, appID, code, stage, body string
		status, errCode                int
		transportError, readError      bool
	}{
		{name: "configuration", code: "private-code", stage: "configuration"},
		{name: "input", appID: "app-id", stage: "input"},
		{name: "transport", appID: "app-id", code: "private-code", stage: "transport", transportError: true},
		{name: "http", appID: "app-id", code: "private-code", stage: "http", status: 503, body: wechatDiagnosticSecret},
		{name: "read", appID: "app-id", code: "private-code", stage: "response", status: 200, readError: true},
		{name: "malformed", appID: "app-id", code: "private-code", stage: "response", status: 200, body: wechatDiagnosticSecret},
		{name: "missing-openid", appID: "app-id", code: "private-code", stage: "response", status: 200, body: `{"session_key":"private-session-key"}`},
		{name: "oversized", appID: "app-id", code: "private-code", stage: "response", status: 200, body: strings.Repeat("x", 8193)},
		{name: "wechat", appID: "app-id", code: "private-code", stage: "wechat", status: 200, errCode: 40125, body: `{"errcode":40125,"errmsg":"` + wechatDiagnosticSecret + `"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ex := WeChatAPIExchanger{AppID: tc.appID, AppSecret: "private-secret", Client: &http.Client{Transport: wechatTransport(func(r *http.Request) (*http.Response, error) {
				if tc.transportError {
					return nil, errors.New(wechatDiagnosticSecret)
				}
				var reader io.Reader = strings.NewReader(tc.body)
				if tc.readError {
					reader = iotest.ErrReader(errors.New(wechatDiagnosticSecret))
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(reader), Header: make(http.Header), Request: r}, nil
			})}}
			openid, err := ex.Exchange(context.Background(), tc.code)
			if openid != "" || err == nil {
				t.Fatal("failed exchange authorized an identity")
			}
			var diagnostic interface {
				error
				Stage() string
				ErrCode() int
			}
			if !errors.As(err, &diagnostic) {
				t.Fatal("exchange error lacks safe typed diagnostics")
			}
			if diagnostic.Stage() != tc.stage || diagnostic.ErrCode() != tc.errCode {
				t.Fatalf("diagnostics: stage=%q errcode=%d", diagnostic.Stage(), diagnostic.ErrCode())
			}
			if strings.Contains(err.Error(), "private-") || strings.Contains(err.Error(), "https:") || errors.Unwrap(err) != nil {
				t.Fatal("exchange error retained secret-bearing upstream information")
			}
		})
	}
}

func captureWeChatDiagnosticLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	previousWriter, previousFlags, previousPrefix := log.Writer(), log.Flags(), log.Prefix()
	log.SetOutput(&output)
	log.SetFlags(0)
	log.SetPrefix("")
	t.Cleanup(func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
		log.SetPrefix(previousPrefix)
	})
	return &output
}

func TestWeChatSessionReportsOnlySafeExchangeDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		code   int
		reason string
	}{
		{40013, "credential_rejected"},
		{40125, "credential_rejected"},
		{40029, "invalid_login_code"},
		{40163, "invalid_login_code"},
		{-1, "unavailable"},
		{45011, "unavailable"},
		{40226, "unavailable"},
		{40001, "unavailable"},
		{98765, "unavailable"},
	} {
		t.Run(fmt.Sprint(tc.code), func(t *testing.T) {
			logs := captureWeChatDiagnosticLog(t)
			c, _, root := fixture(t)
			g := wechatHandler(t, c, root, filepath.Join(root, "wechat"), time.Now).(*wechatGateway)
			g.config.Exchanger = WeChatAPIExchanger{AppID: "app-id", AppSecret: "private-secret", Client: &http.Client{Transport: wechatTransport(func(r *http.Request) (*http.Response, error) {
				body := fmt.Sprintf(`{"errcode":%d,"errmsg":%q,"openid":"private-openid"}`, tc.code, wechatDiagnosticSecret)
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}, nil
			})}}
			w := wechatRequest(g, http.MethodPost, "/api/session", "", `{"code":"private-code"}`)
			var body map[string]string
			if w.Code != http.StatusUnauthorized || w.Header().Get("WWW-Authenticate") != "Bearer" || json.Unmarshal(w.Body.Bytes(), &body) != nil || body["reason"] != tc.reason || body["error"] == "" || len(body) != 2 {
				t.Fatalf("unsafe or missing session diagnostic: status=%d body=%s", w.Code, w.Body)
			}
			wantLog := fmt.Sprintf("wechat login exchange failed stage=wechat errcode=%d\n", tc.code)
			if logs.String() != wantLog || strings.Contains(w.Body.String(), "private-") {
				t.Fatal("session response or log contained uncontrolled exchange data")
			}
		})
	}
}

func TestWeChatSessionTransportFailureIsUnavailableWithoutSecrets(t *testing.T) {
	logs := captureWeChatDiagnosticLog(t)
	c, _, root := fixture(t)
	g := wechatHandler(t, c, root, filepath.Join(root, "wechat"), time.Now).(*wechatGateway)
	g.config.Exchanger = WeChatAPIExchanger{AppID: "app-id", AppSecret: "private-secret", Client: &http.Client{Transport: wechatTransport(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New(wechatDiagnosticSecret)
	})}}
	w := wechatRequest(g, http.MethodPost, "/api/session", "", `{"code":"private-code"}`)
	var body map[string]string
	if w.Code != http.StatusUnauthorized || json.Unmarshal(w.Body.Bytes(), &body) != nil || body["reason"] != "unavailable" || len(body) != 2 {
		t.Fatalf("transport failure: status=%d body=%s", w.Code, w.Body)
	}
	if logs.String() != "wechat login exchange failed stage=transport errcode=0\n" || strings.Contains(w.Body.String(), "private-") {
		t.Fatal("transport credentials escaped into response or log")
	}
}

type wechatUnknownExchangeFailure struct{}

func (wechatUnknownExchangeFailure) Exchange(context.Context, string) (string, error) {
	return "private-openid", errors.New(wechatDiagnosticSecret)
}

func TestWeChatSessionUnknownExchangerErrorsStayPrivate(t *testing.T) {
	logs := captureWeChatDiagnosticLog(t)
	c, _, root := fixture(t)
	g := wechatHandler(t, c, root, filepath.Join(root, "wechat"), time.Now).(*wechatGateway)
	g.config.Exchanger = wechatUnknownExchangeFailure{}
	w := wechatRequest(g, http.MethodPost, "/api/session", "", `{"code":"private-code"}`)
	var body map[string]string
	if w.Code != http.StatusUnauthorized || json.Unmarshal(w.Body.Bytes(), &body) != nil || len(body) != 2 || body["error"] == "" || body["reason"] != "unavailable" || strings.Contains(w.Body.String(), "private-") {
		t.Fatalf("unknown exchanger error: status=%d body=%s", w.Code, w.Body)
	}
	if logs.Len() != 0 {
		t.Fatal("unknown exchanger error was logged")
	}
}
