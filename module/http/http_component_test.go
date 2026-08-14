package http

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	nethttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
)

func TestApplyCrossDomainHeadersAllowsConfiguredOrigin(t *testing.T) {
	component := NewBareHttpComponent("127.0.0.1:0")
	component.SetCORSAllowedOrigins([]string{
		"https://game.example",
		"*",
		"",
	})

	request := httptest.NewRequest(nethttp.MethodOptions, "/login", nil)
	request.Header.Set("Origin", "https://game.example")
	recorder := httptest.NewRecorder()
	applyCrossDomainHeaders(recorder, request, component.allowedOriginsSnapshot())

	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "https://game.example" {
		t.Fatalf("allow-origin = %q, want configured origin", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST" {
		t.Fatalf("allow-methods = %q, want GET, POST", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Fatal("preflight response missing allow-headers")
	}
	if got := recorder.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("vary = %q, want Origin", got)
	}
}

func TestApplyCrossDomainHeadersRejectsUnconfiguredOrigin(t *testing.T) {
	allowed := map[string]struct{}{"https://game.example": {}}
	request := httptest.NewRequest(nethttp.MethodGet, "/login", nil)
	request.Header.Set("Origin", "https://evil.example")
	recorder := httptest.NewRecorder()
	applyCrossDomainHeaders(recorder, request, allowed)

	for _, header := range []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Methods",
		"Access-Control-Allow-Headers",
		"Access-Control-Max-Age",
	} {
		if got := recorder.Header().Get(header); got != "" {
			t.Fatalf("%s = %q for unconfigured origin, want empty", header, got)
		}
	}
}

func TestHTTPComponentUsesTLSWhenConfigured(t *testing.T) {
	certFile, keyFile := writeTestTLSFiles(t)
	component := NewBareHttpComponent("127.0.0.1:0")
	if err := component.ConfigureTLS(certFile, keyFile, true); err != nil {
		t.Fatalf("ConfigureTLS error = %v", err)
	}
	scene := ecs.NewScene(ecs.SceneTypeHTTP, 1, "http")
	scene.AddComponent(component)
	defer scene.Dispose()
	defer component.OnDestroy()

	if err := component.Start(); err != nil {
		t.Fatalf("Start error = %v", err)
	}
	conn, err := tls.Dial("tcp", component.Addr(), &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, // test certificate is self-signed
	})
	if err != nil {
		t.Fatalf("TLS dial error = %v", err)
	}
	defer conn.Close()
	if !conn.ConnectionState().HandshakeComplete {
		t.Fatal("TLS handshake did not complete")
	}
}

func TestHTTPComponentReloadsTLSCertificateForNewConnections(t *testing.T) {
	certFile, keyFile := writeTestTLSFilesWithSerial(t, 1)
	component := NewBareHttpComponent("127.0.0.1:0")
	if err := component.ConfigureTLS(certFile, keyFile, true); err != nil {
		t.Fatalf("ConfigureTLS error = %v", err)
	}
	scene := ecs.NewScene(ecs.SceneTypeHTTP, 1, "http")
	scene.AddComponent(component)
	defer scene.Dispose()
	defer component.OnDestroy()

	if err := component.Start(); err != nil {
		t.Fatalf("Start error = %v", err)
	}
	first := dialTestTLS(t, component.Addr())
	if got := first.ConnectionState().PeerCertificates[0].SerialNumber.Int64(); got != 1 {
		first.Close()
		t.Fatalf("initial certificate serial = %d, want 1", got)
	}
	first.Close()

	nextCertFile, nextKeyFile := writeTestTLSFilesWithSerial(t, 2)
	certData, err := os.ReadFile(nextCertFile)
	if err != nil {
		t.Fatalf("read rotated certificate error = %v", err)
	}
	keyData, err := os.ReadFile(nextKeyFile)
	if err != nil {
		t.Fatalf("read rotated key error = %v", err)
	}
	if err := os.WriteFile(certFile, certData, 0o600); err != nil {
		t.Fatalf("replace certificate error = %v", err)
	}
	if err := os.WriteFile(keyFile, keyData, 0o600); err != nil {
		t.Fatalf("replace key error = %v", err)
	}

	second := dialTestTLS(t, component.Addr())
	defer second.Close()
	if got := second.ConnectionState().PeerCertificates[0].SerialNumber.Int64(); got != 2 {
		t.Fatalf("rotated certificate serial = %d, want 2", got)
	}
}

func TestHTTPComponentRejectsTLSFallbackConfiguration(t *testing.T) {
	component := NewBareHttpComponent("127.0.0.1:0")
	if err := component.ConfigureTLS("", "", true); err != ErrTLSConfigurationInvalid {
		t.Fatalf("ConfigureTLS empty required config error = %v, want %v", err, ErrTLSConfigurationInvalid)
	}
	if err := component.ConfigureTLS("server.crt", "", false); err != ErrTLSConfigurationInvalid {
		t.Fatalf("ConfigureTLS partial config error = %v, want %v", err, ErrTLSConfigurationInvalid)
	}
}

func writeTestTLSFiles(t *testing.T) (string, string) {
	return writeTestTLSFilesWithSerial(t, 1)
}

func writeTestTLSFilesWithSerial(t *testing.T, serial int64) (string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey error = %v", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  nil,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate error = %v", err)
	}
	dir := t.TempDir()
	certFile := filepath.Join(dir, "server.crt")
	keyFile := filepath.Join(dir, "server.key")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write certificate error = %v", err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0o600); err != nil {
		t.Fatalf("write key error = %v", err)
	}
	return certFile, keyFile
}

func dialTestTLS(t *testing.T, address string) *tls.Conn {
	t.Helper()
	conn, err := tls.Dial("tcp", address, &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, // test certificate is self-signed
	})
	if err != nil {
		t.Fatalf("TLS dial error = %v", err)
	}
	if !conn.ConnectionState().HandshakeComplete {
		conn.Close()
		t.Fatal("TLS handshake did not complete")
	}
	return conn
}
