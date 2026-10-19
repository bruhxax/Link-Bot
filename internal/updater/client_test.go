package updater

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestClientUsesAuthenticatedUnixSocket(t *testing.T) {
	// Keep the socket under the Unix 108-byte path limit, including on Windows.
	dir, err := os.MkdirTemp("", "lbu-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	if err = os.WriteFile(filepath.Join(dir, "token"), []byte(testToken+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := NewAgent(dir, dir, testToken)
	if err != nil {
		t.Fatal(err)
	}
	a.run = func(context.Context) error { return errLocalChanges }
	listener, err := net.Listen("unix", filepath.Join(dir, "agent.sock"))
	if err != nil {
		if runtime.GOOS == "windows" && strings.Contains(err.Error(), "not supported") {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	server := &http.Server{Handler: a}
	t.Cleanup(func() { server.Close() })
	go server.Serve(listener)
	client := Client{Dir: dir}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	status, err := client.Request(ctx, false)
	if err != nil || !status.Available || status.State != "idle" {
		t.Fatal(status, err)
	}
	started, err := client.Request(ctx, true)
	if err != nil || started.State != "running" || started.ID == "" {
		t.Fatal(started, err)
	}
	for {
		status, err = client.Request(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		if status.State == "failed" {
			break
		}
		if ctx.Err() != nil {
			t.Fatal("job stuck")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if status.ID != started.ID || !strings.Contains(status.Message, "исходниках") {
		t.Fatal(status)
	}
	if err = os.WriteFile(filepath.Join(dir, "token"), []byte(strings.Repeat("x", 64)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Request(ctx, false); err == nil {
		t.Fatal("invalid socket token accepted")
	}
}
