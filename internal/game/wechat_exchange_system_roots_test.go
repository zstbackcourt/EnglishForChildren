package game

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"go/version"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Each child starts with an untouched x509 system-roots cache. No certificate
// pool or TLS configuration is injected into the Exchange client's transport.
func TestWeChatCode2SessionUsesSystemCertificateDirectories(t *testing.T) {
	if runtime.GOOS == "darwin" && version.Compare(runtime.Version(), "go1.27") < 0 {
		t.Skip("macOS needs Go 1.27 to exercise the production Linux certificate-directory loader")
	}
	root := t.TempDir()
	systemDir, platformDir, emptyDir := filepath.Join(root, "system"), filepath.Join(root, "platform"), filepath.Join(root, "empty")
	for _, dir := range []string{systemDir, platformDir, emptyDir} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal("could not create isolated certificate directory")
		}
	}
	emptyFile := filepath.Join(root, "empty-bundle.pem")
	if err := os.WriteFile(emptyFile, nil, 0600); err != nil {
		t.Fatal("could not create isolated certificate bundle")
	}
	systemCA, systemKey := wechatTLSRootCA(t)
	platformCA, platformKey := wechatTLSRootCA(t)
	for _, entry := range []struct {
		dir  string
		cert *x509.Certificate
	}{{systemDir, systemCA}, {platformDir, platformCA}} {
		data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: entry.cert.Raw})
		if err := os.WriteFile(filepath.Join(entry.dir, "root.crt"), data, 0600); err != nil {
			t.Fatal("could not write temporary test root")
		}
	}
	for _, tc := range []struct {
		name, hostname, certDirs, wantKind string
		ca                                 *x509.Certificate
		key                                *ecdsa.PrivateKey
		wantRequests                       int32
	}{
		{"missing_platform_ca", "api.weixin.qq.com", systemDir + string(os.PathListSeparator) + emptyDir, "tls_unknown_authority", platformCA, platformKey, 0},
		{"platform_ca_in_second_directory", "api.weixin.qq.com", systemDir + string(os.PathListSeparator) + platformDir, "", platformCA, platformKey, 1},
		{"first_directory_still_trusted", "api.weixin.qq.com", systemDir + string(os.PathListSeparator) + platformDir, "", systemCA, systemKey, 1},
		{"trusted_ca_cannot_bypass_hostname", "unrelated.invalid", systemDir + string(os.PathListSeparator) + platformDir, "tls_hostname", platformCA, platformKey, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			var invalidRequest atomic.Bool
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				query := r.URL.Query()
				if r.Method != http.MethodGet || r.Host != "api.weixin.qq.com" || r.URL.Path != "/sns/jscode2session" ||
					r.TLS == nil || r.TLS.ServerName != "api.weixin.qq.com" || query.Get("appid") != "test-app-id" ||
					query.Get("secret") != "test-app-secret" || query.Get("js_code") != "test-login-code" || query.Get("grant_type") != "authorization_code" {
					invalidRequest.Store(true)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"openid":"test-openid"}`)
			}))
			server.Config.ErrorLog = log.New(io.Discard, "", 0)
			server.TLS = &tls.Config{Certificates: []tls.Certificate{wechatTLSServerCertificate(t, tc.ca, tc.key, tc.hostname)}}
			server.StartTLS()
			defer server.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWeChatSystemRootsSubprocess$", "-test.count=1")
			for _, value := range os.Environ() {
				key, _, _ := strings.Cut(value, "=")
				if key != "SSL_CERT_FILE" && key != "SSL_CERT_DIR" && key != "GODEBUG" && !strings.HasPrefix(key, "WECHAT_TEST_TLS_") {
					cmd.Env = append(cmd.Env, value)
				}
			}
			cmd.Env = append(cmd.Env, "SSL_CERT_FILE="+emptyFile, "SSL_CERT_DIR="+tc.certDirs,
				"WECHAT_TEST_TLS_CHILD=1", "WECHAT_TEST_TLS_ADDRESS="+server.Listener.Addr().String(), "WECHAT_TEST_TLS_KIND="+tc.wantKind)
			if runtime.GOOS == "darwin" {
				// Go 1.27 can use the same on-disk roots loader on macOS as Linux.
				// This module's older go directive otherwise selects macOS APIs.
				// Linux children keep production defaults and need no GODEBUG.
				cmd.Env = append(cmd.Env, "GODEBUG=x509sslcertoverrideplatform=1")
			}
			output, err := cmd.CombinedOutput()
			if err != nil {
				// The child only emits fixed test messages and safe error categories.
				t.Fatalf("isolated TLS exchange failed: %s", output)
			}
			if requests.Load() != tc.wantRequests || invalidRequest.Load() {
				t.Fatalf("TLS exchange reached HTTP %d times; want %d valid requests", requests.Load(), tc.wantRequests)
			}
		})
	}
}

func TestWeChatSystemRootsSubprocess(t *testing.T) {
	if os.Getenv("WECHAT_TEST_TLS_CHILD") != "1" {
		t.Skip("helper executes only in an isolated certificate-directory subprocess")
	}
	address := os.Getenv("WECHAT_TEST_TLS_ADDRESS")
	host, _, err := net.SplitHostPort(address)
	if err != nil || (host != "127.0.0.1" && host != "::1") {
		t.Fatal("test TLS destination must be a loopback listener")
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, requestedAddress string) (net.Conn, error) {
			if requestedAddress != "api.weixin.qq.com:443" {
				return nil, errors.New("unexpected test TLS destination")
			}
			return (&net.Dialer{}).DialContext(ctx, network, address)
		},
	}
	defer transport.CloseIdleConnections()
	ex := WeChatAPIExchanger{AppID: "test-app-id", AppSecret: "test-app-secret", Client: &http.Client{Transport: transport}}
	openid, exchangeErr := ex.Exchange(context.Background(), "test-login-code")
	wantKind := os.Getenv("WECHAT_TEST_TLS_KIND")
	if wantKind == "" {
		if exchangeErr != nil {
			var diagnostic *wechatExchangeError
			if errors.As(exchangeErr, &diagnostic) {
				t.Fatalf("trusted-root exchange failed: stage=%s, transport=%s", diagnostic.Stage(), diagnostic.TransportKind())
			}
			t.Fatal("trusted-root exchange failed without a safe diagnostic")
		}
		if openid != "test-openid" {
			t.Fatal("trusted-root exchange did not return the fixture identity")
		}
		return
	}
	if wantKind != "tls_unknown_authority" && wantKind != "tls_hostname" {
		t.Fatal("unsupported test TLS category")
	}
	var diagnostic *wechatExchangeError
	if openid != "" || !errors.As(exchangeErr, &diagnostic) || diagnostic.Stage() != "transport" || diagnostic.TransportKind() != wantKind {
		t.Fatal("TLS validation did not reject with the expected safe category")
	}
	if errors.Unwrap(exchangeErr) != nil || strings.Contains(exchangeErr.Error(), "test-") || strings.Contains(exchangeErr.Error(), address) {
		t.Fatal("TLS diagnostic retained private request information")
	}
}

func wechatTLSRootCA(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal("could not generate temporary CA key")
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "WeChat integration test CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal("could not generate temporary CA certificate")
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal("could not parse temporary CA certificate")
	}
	return cert, key
}

func wechatTLSServerCertificate(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, hostname string) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal("could not generate temporary server key")
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: []string{hostname},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal("could not generate temporary server certificate")
	}
	return tls.Certificate{Certificate: [][]byte{der, ca.Raw}, PrivateKey: key}
}
