package miniapp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

func TestSMTPSecretsRetainedOnlyForSameHost(t *testing.T) {
	stored := emailSMTPSettings{host: "smtp.example.com", password: "secret", proxyURL: "https://user:secret@proxy.example.com"}
	same := resolveSMTPSecrets(emailSMTPSettings{host: "SMTP.EXAMPLE.COM"}, stored)
	if same.password != stored.password || same.proxyURL != stored.proxyURL {
		t.Fatal("same-host settings lost secrets")
	}
	other := resolveSMTPSecrets(emailSMTPSettings{host: "elsewhere.example.com"}, stored)
	if other.password != "" || other.proxyURL != "" {
		t.Fatal("saved secrets forwarded to another host")
	}
}
func TestSMTPViewMasksEnvironmentCredentials(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_USER", "sender@example.com")
	t.Setenv("SMTP_PASSWORD", "private-password")
	t.Setenv("SMTP_FROM", "sender@example.com")
	t.Setenv("SMTP_PROXY_URL", "https://user:proxy-secret@proxy.example.com")
	raw, err := json.Marshal((&Handler{}).adminSMTPView())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "private-password") || strings.Contains(string(raw), "proxy-secret") {
		t.Fatal("SMTP view exposed secrets")
	}
	if !strings.Contains(string(raw), `"passwordConfigured":true`) {
		t.Fatal("saved-password indicator missing")
	}
}
func TestSMTPPermissionIsIndependent(t *testing.T) {
	for _, path := range []string{"settings", "update", "check", "test"} {
		route := "/api/mini-app/admin/smtp/" + path
		if !adminRouteAllowed(adminAccess{IsAdmin: true, Permissions: []string{"smtp"}}, route) {
			t.Fatalf("SMTP role denied %s", path)
		}
		if adminRouteAllowed(adminAccess{IsAdmin: true, Permissions: []string{"integrations", "ai", "broadcast"}}, route) {
			t.Fatalf("other role gained %s", path)
		}
	}
}

// Exercise STARTTLS, authentication, health checks and delivery locally.
func TestSMTPConnectionAndTestDelivery(t *testing.T) {
	certServer := httptest.NewTLSServer(nil)
	defer certServer.Close()
	cert, err := x509.ParseCertificate(certServer.TLS.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	s := emailSMTPSettings{host: "127.0.0.1", port: port, user: "sender", password: "local-test", from: "Link-Bot <sender@example.com>", rootCAs: roots}
	delivered := make(chan string, 1)
	serverDone := make(chan error, 1)
	go func() {
		for i := 0; i < 2; i++ {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				serverDone <- acceptErr
				return
			}
			_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
			p := textproto.NewConn(conn)
			_ = p.PrintfLine("220 local SMTP ready")
			for {
				line, readErr := p.ReadLine()
				if readErr == io.EOF {
					break
				}
				if readErr != nil {
					serverDone <- readErr
					conn.Close()
					return
				}
				switch {
				case strings.HasPrefix(line, "EHLO"):
					_ = p.PrintfLine("250-local SMTP\r\n250-STARTTLS\r\n250 AUTH PLAIN")
				case line == "STARTTLS":
					_ = p.PrintfLine("220 ready for TLS")
					secure := tls.Server(conn, certServer.TLS)
					if handshakeErr := secure.Handshake(); handshakeErr != nil {
						serverDone <- handshakeErr
						conn.Close()
						return
					}
					p = textproto.NewConn(secure)
				case strings.HasPrefix(line, "AUTH PLAIN"):
					_ = p.PrintfLine("235 authenticated")
				case line == "NOOP":
					_ = p.PrintfLine("250 ok")
				case strings.HasPrefix(line, "MAIL FROM:") || strings.HasPrefix(line, "RCPT TO:"):
					_ = p.PrintfLine("250 ok")
				case line == "DATA":
					_ = p.PrintfLine("354 send message")
					body, dataErr := p.ReadDotBytes()
					if dataErr != nil {
						serverDone <- dataErr
						conn.Close()
						return
					}
					delivered <- string(body)
					_ = p.PrintfLine("250 accepted")
				case line == "QUIT":
					_ = p.PrintfLine("221 bye")
					p.Close()
					goto next
				default:
					serverDone <- io.ErrUnexpectedEOF
					conn.Close()
					return
				}
			}
			p.Close()
		next:
		}
		serverDone <- nil
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := connectSMTP(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Noop(); err != nil {
		t.Fatal(err)
	}
	if err = client.Quit(); err != nil {
		t.Fatal(err)
	}
	if err = sendSMTPMessage(ctx, s, "recipient@example.com", "Проверка", "Тестовое письмо"); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-delivered:
		if !strings.Contains(body, "Тестовое письмо") || !strings.Contains(body, "From: Link-Bot <sender@example.com>") || !strings.Contains(body, "To: recipient@example.com") {
			t.Fatalf("unexpected mail: %s", body)
		}
	case <-ctx.Done():
		t.Fatal("test message missing")
	}
	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("SMTP server stalled")
	}
}
