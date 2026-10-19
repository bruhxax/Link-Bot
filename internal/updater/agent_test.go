package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func testAgent(t *testing.T) *Agent {
	t.Helper()
	dir := t.TempDir()
	agent, err := NewAgent(dir, dir, testToken)
	if err != nil {
		t.Fatal(err)
	}
	return agent
}
func agentRequest(a *Agent, method, path, body, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	a.ServeHTTP(w, r)
	return w
}

func TestAgentAuthorizationAndFixedOperation(t *testing.T) {
	a := testAgent(t)
	for _, tc := range []struct {
		method, path, body, token string
		want                      int
	}{
		{"GET", "/status", "", "wrong", 401},
		{"POST", "/update", "", "", 401},
		{"POST", "/update?command=rm", "", testToken, 400},
		{"POST", "/update", `{"command":"whoami"}`, testToken, 400},
		{"GET", "/update", "", testToken, 404},
		{"POST", "/exec", "", testToken, 404},
		{"GET", "/status", "", testToken, 200},
	} {
		w := agentRequest(a, tc.method, tc.path, tc.body, tc.token)
		if w.Code != tc.want {
			t.Errorf("%+v: %d %s", tc, w.Code, w.Body.String())
		}
	}
}

func TestAgentSurvivesCallerCancellationAndPreventsParallelUpdates(t *testing.T) {
	a := testAgent(t)
	started, finish := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.run = func(ctx context.Context) error {
		close(started)
		select {
		case <-ctx.Done():
			t.Error("HTTP cancellation killed update")
			return ctx.Err()
		case <-finish:
			return errLocalChanges
		}
	}
	r := httptest.NewRequest("POST", "/update", nil).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer "+testToken)
	w := httptest.NewRecorder()
	a.ServeHTTP(w, r)
	cancel()
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("job not started")
	}
	var first State
	json.Unmarshal(w.Body.Bytes(), &first)
	second := agentRequest(a, "POST", "/update", "", testToken)
	var state State
	json.Unmarshal(second.Body.Bytes(), &state)
	if second.Code != 200 || state.ID != first.ID || state.State != "running" {
		t.Fatal(second.Code, state)
	}
	// A newly created client sees the same host-side job rather than starting one.
	persisted, err := NewAgent(a.Repo, a.Dir, testToken)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.state.State != "failed" || !strings.Contains(persisted.state.Message, "перезапущен") {
		t.Fatal(persisted.state)
	}
	close(finish)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		a.mu.Lock()
		done := a.state.State != "running"
		a.mu.Unlock()
		if done {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("job did not finish")
}

func TestAgentJobResultIsPersistedWithoutRawErrors(t *testing.T) {
	a := testAgent(t)
	a.run = func(context.Context) error { return errors.New("secret-token-should-not-leak") }
	w := agentRequest(a, "POST", "/update", "", testToken)
	if w.Code != 202 {
		t.Fatal(w.Code)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		w = agentRequest(a, "GET", "/status", "", testToken)
		var state State
		json.Unmarshal(w.Body.Bytes(), &state)
		if state.State == "failed" {
			if strings.Contains(w.Body.String(), "secret-token") || state.FinishedAt == "" {
				t.Fatal(state)
			}
			restored, err := NewAgent(a.Repo, a.Dir, testToken)
			if err != nil || restored.state.ID != state.ID || restored.state.State != "failed" {
				t.Fatal(restored, err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("job did not finish")
}

func TestStageWriterDoesNotExposeUntrustedLogs(t *testing.T) {
	a := testAgent(t)
	var log bytes.Buffer
	w := &stageWriter{agent: a, log: &log}
	w.Write([]byte("[1/5] Сборка новой версии\n[2/5] token=secret\n"))
	if a.state.Stage != "Сборка новой версии" || !strings.Contains(log.String(), "token=secret") {
		t.Fatal(a.state)
	}
	w.Write(bytes.Repeat([]byte("x"), 3<<20))
	if log.Len() > 2<<20 || len(w.pending) > 8192 {
		t.Fatal("unbounded logs")
	}
}

func TestClientUnavailableDoesNotLeakPath(t *testing.T) {
	_, err := (Client{Dir: filepath.Join(t.TempDir(), "private")}).Request(context.Background(), true)
	if err == nil || !strings.Contains(err.Error(), "bash ./update.sh") || strings.Contains(err.Error(), "private") {
		t.Fatal(err)
	}
}

func TestOfficialOrigins(t *testing.T) {
	for _, origin := range []string{"https://github.com/bruhxax/Link-Bot.git", "git@github.com:bruhxax/Link-Bot.git", "ssh://git@github.com/bruhxax/Link-Bot"} {
		if !officialOrigin(origin) {
			t.Fatal(origin)
		}
	}
	for _, origin := range []string{"https://github.com/other/Link-Bot.git", "https://github.com.evil/bruhxax/Link-Bot", "https://github.com/bruhxax/Link-Bot.git; whoami"} {
		if officialOrigin(origin) {
			t.Fatal(origin)
		}
	}
}

func TestAgentCannotUpdateWrongCheckoutOrDirtyFiles(t *testing.T) {
	a := testAgent(t)
	if err := a.execute(context.Background()); !errors.Is(err, errWrongCheckout) {
		t.Fatal(err)
	}
	// Credential state is readable by the container, never logs or job snapshots.
	if _, err := os.Stat(filepath.Join(a.Dir, "update.log")); !os.IsNotExist(err) {
		t.Fatal("unexpected log created before validation")
	}
}

var _ http.Handler = (*Agent)(nil)
