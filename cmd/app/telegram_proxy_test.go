package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestTelegramSOCKS5hPassesHostnameToProxy(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	hosts := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			hosts <- "accept: " + err.Error()
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		hello := make([]byte, 3)
		if _, err := io.ReadFull(conn, hello); err != nil {
			hosts <- "greeting: " + err.Error()
			return
		}
		if string(hello) != string([]byte{5, 1, 0}) {
			hosts <- fmt.Sprintf("unexpected greeting %v", hello)
			return
		}
		_, _ = conn.Write([]byte{5, 0})
		header := make([]byte, 5)
		if _, err := io.ReadFull(conn, header); err != nil {
			hosts <- "request: " + err.Error()
			return
		}
		if string(header[:4]) != string([]byte{5, 1, 0, 3}) {
			hosts <- fmt.Sprintf("expected domain address, got %v", header[:4])
			return
		}
		host := make([]byte, int(header[4]))
		if _, err := io.ReadFull(conn, host); err != nil {
			hosts <- "hostname: " + err.Error()
			return
		}
		port := make([]byte, 2)
		_, _ = io.ReadFull(conn, port)
		hosts <- string(host)
		_, _ = conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 80})
		reader := bufio.NewReader(conn)
		for {
			line, err := reader.ReadString('\n')
			if err != nil || line == "\r\n" {
				break
			}
		}
		_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK")
	}()

	client, err := newTelegramProxyClient("socks5h://" + listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	client.Timeout = 3 * time.Second
	response, err := client.Get("http://unresolvable.invalid/test")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("HTTP status = %d", response.StatusCode)
	}
	if host := <-hosts; host != "unresolvable.invalid" {
		t.Fatalf("SOCKS target = %q", host)
	}
}

func TestTelegramProxyURLValidationDoesNotLeakCredentials(t *testing.T) {
	for _, raw := range []string{
		"socks5h://user:secret@localhost", "socks5h://localhost:0", "socks5h://localhost:65536",
		"socks5h://localhost:1080/path", "ftp://user:secret@localhost:1080", "socks5h://user:%zz@localhost:1080",
	} {
		client, err := newTelegramProxyClient(raw)
		if err == nil || client != nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("invalid proxy URL was accepted or leaked credentials: %q, %v", raw, err)
		}
	}
	client, err := newTelegramProxyClient("  ")
	if err != nil || client != nil {
		t.Fatalf("empty proxy should preserve default client: %v, %v", client, err)
	}
}
