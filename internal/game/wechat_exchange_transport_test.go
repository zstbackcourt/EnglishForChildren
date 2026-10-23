package game

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

type wechatTestTimeout struct{}

func (wechatTestTimeout) Error() string   { return wechatDiagnosticSecret }
func (wechatTestTimeout) Timeout() bool   { return true }
func (wechatTestTimeout) Temporary() bool { return false }

func wechatTransportFailureCases() []struct {
	name     string
	upstream error
	kind     string
} {
	cert := &x509.Certificate{Subject: pkix.Name{CommonName: "private-certificate"}, DNSNames: []string{"private.example"}}
	return []struct {
		name     string
		upstream error
		kind     string
	}{
		{"unknown-authority", x509.UnknownAuthorityError{Cert: cert}, "tls_unknown_authority"},
		{"unknown-authority-pointer", &x509.UnknownAuthorityError{Cert: cert}, "tls_unknown_authority"},
		{"hostname", x509.HostnameError{Certificate: cert, Host: "private-host"}, "tls_hostname"},
		{"hostname-pointer", &x509.HostnameError{Certificate: cert, Host: "private-host"}, "tls_hostname"},
		{"invalid-cert", x509.CertificateInvalidError{Cert: cert, Reason: x509.Expired, Detail: wechatDiagnosticSecret}, "tls_invalid_certificate"},
		{"invalid-cert-pointer", &x509.CertificateInvalidError{Cert: cert, Reason: x509.Expired, Detail: wechatDiagnosticSecret}, "tls_invalid_certificate"},
		{"system-roots", x509.SystemRootsError{Err: errors.New(wechatDiagnosticSecret)}, "tls_system_roots"},
		{"system-roots-pointer", &x509.SystemRootsError{Err: errors.New(wechatDiagnosticSecret)}, "tls_system_roots"},
		{"tls-record", tls.RecordHeaderError{Msg: wechatDiagnosticSecret}, "tls_record_header"},
		{"tls-record-pointer", &tls.RecordHeaderError{Msg: wechatDiagnosticSecret}, "tls_record_header"},
		{"tls-verification-wrapper", &tls.CertificateVerificationError{UnverifiedCertificates: []*x509.Certificate{cert}, Err: x509.UnknownAuthorityError{Cert: cert}}, "tls_unknown_authority"},
		{"dns", &net.DNSError{Err: wechatDiagnosticSecret, Name: "private-host", Server: "192.0.2.44"}, "dns"},
		{"context-timeout", context.DeadlineExceeded, "context_timeout"},
		{"context-canceled", context.Canceled, "context_canceled"},
		{"network-timeout", &net.OpError{Op: "dial", Net: "tcp", Addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.44"), Port: 443}, Err: wechatTestTimeout{}}, "network_timeout"},
		{"connection-refused", syscall.ECONNREFUSED, "connection_refused"},
		{"connection-reset", syscall.ECONNRESET, "connection_reset"},
		{"network-unreachable", syscall.ENETUNREACH, "network_unreachable"},
		{"eof", io.EOF, "eof"},
		{"unexpected-eof", io.ErrUnexpectedEOF, "eof"},
		{"unknown-text-is-not-classified", errors.New("x509: unknown authority timeout " + wechatDiagnosticSecret), "unknown"},
	}
}

func wechatSecretWrappedTransport(upstream error) *http.Client {
	return &http.Client{Transport: wechatTransport(func(r *http.Request) (*http.Response, error) {
		return nil, &url.Error{Op: "Get", URL: wechatDiagnosticSecret, Err: fmt.Errorf("private-wrapper: %w", upstream)}
	})}
}

func TestWeChatCode2SessionClassifiesWrappedTransportWithoutRetainingSecrets(t *testing.T) {
	for _, tc := range wechatTransportFailureCases() {
		t.Run(tc.name, func(t *testing.T) {
			ex := WeChatAPIExchanger{AppID: "app-id", AppSecret: "private-secret", Client: wechatSecretWrappedTransport(tc.upstream)}
			openid, err := ex.Exchange(context.Background(), "private-code")
			if err == nil || openid != "" {
				t.Fatal("transport failure authorized an identity")
			}
			var diagnostic interface {
				error
				Stage() string
				ErrCode() int
				TransportKind() string
			}
			if !errors.As(err, &diagnostic) {
				t.Fatal("transport error lacks a safe TransportKind")
			}
			if diagnostic.Stage() != "transport" || diagnostic.ErrCode() != 0 || diagnostic.TransportKind() != tc.kind {
				t.Fatalf("unexpected safe classification: stage=%q code=%d kind=%q", diagnostic.Stage(), diagnostic.ErrCode(), diagnostic.TransportKind())
			}
			if err.Error() != "WeChat login exchange failed (stage=transport, errcode=0, transport="+tc.kind+")" || errors.Unwrap(err) != nil {
				t.Fatal("transport error retained unapproved information")
			}
		})
	}
}

func TestWeChatSessionTransportCategoriesAreSafeAndNeverIssueToken(t *testing.T) {
	for _, tc := range wechatTransportFailureCases() {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureWeChatDiagnosticLog(t)
			c, _, root := fixture(t)
			g := wechatHandler(t, c, root, filepath.Join(root, "wechat"), time.Now).(*wechatGateway)
			g.config.Exchanger = WeChatAPIExchanger{AppID: "app-id", AppSecret: "private-secret", Client: wechatSecretWrappedTransport(tc.upstream)}
			w := wechatRequest(g, http.MethodPost, "/api/session", "", `{"code":"private-code"}`)
			var body map[string]string
			if w.Code != 401 || w.Header().Get("WWW-Authenticate") != "Bearer" || json.Unmarshal(w.Body.Bytes(), &body) != nil || len(body) != 3 || body["reason"] != "unavailable" || body["diagnostic"] != "transport_"+tc.kind || body["error"] == "" || body["token"] != "" {
				t.Fatal("transport failure response is not a bounded unauthorized diagnostic")
			}
			if logs.String() != "wechat login exchange failed stage=transport errcode=0 transport="+tc.kind+"\n" || strings.Contains(w.Body.String(), "private-") || strings.Contains(w.Body.String(), "192.0.2.44") {
				t.Fatal("transport failure leaked upstream details")
			}
		})
	}
}

func TestWeChatCode2SessionClassifiesRealLocalTLSFailure(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(200) }))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: roots},
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
	}
	defer transport.CloseIdleConnections()
	ex := WeChatAPIExchanger{AppID: "app-id", AppSecret: "private-secret", Client: &http.Client{Transport: transport}}
	openid, err := ex.Exchange(context.Background(), "private-code")
	var diagnostic interface {
		error
		TransportKind() string
	}
	if err == nil || openid != "" || requests.Load() != 0 {
		t.Fatal("TLS hostname validation must reject before sending the login request")
	}
	if !errors.As(err, &diagnostic) || diagnostic.TransportKind() != "tls_hostname" {
		t.Fatal("real TLS hostname rejection was not safely classified")
	}
	if strings.Contains(err.Error(), "private-") || strings.Contains(err.Error(), server.Listener.Addr().String()) || errors.Unwrap(err) != nil {
		t.Fatal("real TLS error retained credentials or network details")
	}
}

func TestWeChatSessionNonTransportFailuresKeepTwoFieldResponse(t *testing.T) {
	for _, tc := range []struct {
		name, appID, body, stage string
		status                   int
	}{
		{"configuration", "", "", "configuration", 200},
		{"http", "app-id", wechatDiagnosticSecret, "http", 503},
		{"response", "app-id", wechatDiagnosticSecret, "response", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureWeChatDiagnosticLog(t)
			c, _, root := fixture(t)
			g := wechatHandler(t, c, root, filepath.Join(root, "wechat"), time.Now).(*wechatGateway)
			g.config.Exchanger = WeChatAPIExchanger{AppID: tc.appID, AppSecret: "private-secret", Client: &http.Client{Transport: wechatTransport(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header), Request: r}, nil
			})}}
			w := wechatRequest(g, http.MethodPost, "/api/session", "", `{"code":"private-code"}`)
			var body map[string]string
			if w.Code != 401 || json.Unmarshal(w.Body.Bytes(), &body) != nil || len(body) != 2 || body["reason"] != "unavailable" || body["error"] == "" {
				t.Fatal("non-transport response contract changed")
			}
			if logs.String() != "wechat login exchange failed stage="+tc.stage+" errcode=0\n" {
				t.Fatal("non-transport log gained uncontrolled details")
			}
		})
	}
}

type wechatSpoofedTransportError struct{}

func (wechatSpoofedTransportError) Error() string         { return wechatDiagnosticSecret }
func (wechatSpoofedTransportError) Stage() string         { return "transport" }
func (wechatSpoofedTransportError) ErrCode() int          { return 0 }
func (wechatSpoofedTransportError) TransportKind() string { return wechatDiagnosticSecret }

type wechatSpoofedTransportExchanger struct{}

func (wechatSpoofedTransportExchanger) Exchange(context.Context, string) (string, error) {
	return "private-openid", wechatSpoofedTransportError{}
}
func TestWeChatSessionDoesNotTrustUnknownTransportDiagnosticInterfaces(t *testing.T) {
	logs := captureWeChatDiagnosticLog(t)
	c, _, root := fixture(t)
	g := wechatHandler(t, c, root, filepath.Join(root, "wechat"), time.Now).(*wechatGateway)
	g.config.Exchanger = wechatSpoofedTransportExchanger{}
	w := wechatRequest(g, http.MethodPost, "/api/session", "", `{"code":"private-code"}`)
	var body map[string]string
	if w.Code != 401 || json.Unmarshal(w.Body.Bytes(), &body) != nil || len(body) != 2 || body["reason"] != "unavailable" || strings.Contains(w.Body.String(), "private-") || logs.Len() != 0 {
		t.Fatal("unknown exchanger injected diagnostics or authorized an identity")
	}
}
