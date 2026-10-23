package game

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"syscall"
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

type wechatTransportKind uint8

const (
	wechatTransportUnknown wechatTransportKind = iota
	wechatTransportUnknownAuthority
	wechatTransportHostname
	wechatTransportInvalidCertificate
	wechatTransportSystemRoots
	wechatTransportRecordHeader
	wechatTransportDNS
	wechatTransportContextTimeout
	wechatTransportContextCanceled
	wechatTransportNetworkTimeout
	wechatTransportConnectionRefused
	wechatTransportConnectionReset
	wechatTransportNetworkUnreachable
	wechatTransportEOF
)

// No upstream error, URL, response text or credentials may be retained here.
// Numeric API codes are diagnostic data; all printable stages are fixed below.
type wechatExchangeError struct {
	stage         wechatExchangeStage
	errCode       int
	transportKind wechatTransportKind
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

// Only enum values can cross the logging and client-response boundary.
func (e *wechatExchangeError) TransportKind() string {
	if e != nil && e.stage == wechatExchangeTransport {
		switch e.transportKind {
		case wechatTransportUnknownAuthority:
			return "tls_unknown_authority"
		case wechatTransportHostname:
			return "tls_hostname"
		case wechatTransportInvalidCertificate:
			return "tls_invalid_certificate"
		case wechatTransportSystemRoots:
			return "tls_system_roots"
		case wechatTransportRecordHeader:
			return "tls_record_header"
		case wechatTransportDNS:
			return "dns"
		case wechatTransportContextTimeout:
			return "context_timeout"
		case wechatTransportContextCanceled:
			return "context_canceled"
		case wechatTransportNetworkTimeout:
			return "network_timeout"
		case wechatTransportConnectionRefused:
			return "connection_refused"
		case wechatTransportConnectionReset:
			return "connection_reset"
		case wechatTransportNetworkUnreachable:
			return "network_unreachable"
		case wechatTransportEOF:
			return "eof"
		}
	}
	return "unknown"
}

func (e *wechatExchangeError) Error() string {
	message := "WeChat login exchange failed (stage=" + e.Stage() + ", errcode=" + strconv.Itoa(e.ErrCode())
	if e != nil && e.stage == wechatExchangeTransport {
		message += ", transport=" + e.TransportKind()
	}
	return message + ")"
}

// Inspect typed causes without formatting or retaining the original error.
func classifyWeChatTransport(err error) wechatTransportKind {
	var roots x509.SystemRootsError
	var rootsPointer *x509.SystemRootsError
	var authority x509.UnknownAuthorityError
	var authorityPointer *x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var hostnamePointer *x509.HostnameError
	var certificate x509.CertificateInvalidError
	var certificatePointer *x509.CertificateInvalidError
	var record tls.RecordHeaderError
	var recordPointer *tls.RecordHeaderError
	var dns *net.DNSError
	switch {
	case errors.As(err, &roots) || errors.As(err, &rootsPointer):
		return wechatTransportSystemRoots
	case errors.As(err, &authority) || errors.As(err, &authorityPointer):
		return wechatTransportUnknownAuthority
	case errors.As(err, &hostname) || errors.As(err, &hostnamePointer):
		return wechatTransportHostname
	case errors.As(err, &certificate) || errors.As(err, &certificatePointer):
		return wechatTransportInvalidCertificate
	case errors.As(err, &record) || errors.As(err, &recordPointer):
		return wechatTransportRecordHeader
	case errors.As(err, &dns):
		return wechatTransportDNS
	case errors.Is(err, context.DeadlineExceeded):
		return wechatTransportContextTimeout
	case errors.Is(err, context.Canceled):
		return wechatTransportContextCanceled
	case errors.Is(err, syscall.ECONNREFUSED):
		return wechatTransportConnectionRefused
	case errors.Is(err, syscall.ECONNRESET):
		return wechatTransportConnectionReset
	case errors.Is(err, syscall.ENETUNREACH):
		return wechatTransportNetworkUnreachable
	case errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF):
		return wechatTransportEOF
	case wechatTransportTimeout(err):
		return wechatTransportNetworkTimeout
	default:
		return wechatTransportUnknown
	}
}

// url.Error.Timeout only checks its immediate cause, so inspect wrapped causes
// as well when another wrapper sits between it and the timeout error.
func wechatTransportTimeout(err error) bool {
	if network, ok := err.(net.Error); ok && network.Timeout() {
		return true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, cause := range joined.Unwrap() {
			if wechatTransportTimeout(cause) {
				return true
			}
		}
		return false
	}
	if cause := errors.Unwrap(err); cause != nil {
		return wechatTransportTimeout(cause)
	}
	return false
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
		return "", &wechatExchangeError{stage: wechatExchangeTransport, transportKind: classifyWeChatTransport(err)}
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
