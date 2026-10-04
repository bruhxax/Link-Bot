package supportai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestCompatibleProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer third-party-test-key" {
			t.Error("missing bearer key")
		}
		switch r.URL.Path {
		case "/v1/models":
			w.Write([]byte(`{"data":[{"id":"z-model"},{"id":"a-model"},{"id":"z-model"},{"id":""}]}`))
		case "/v1/chat/completions":
			var input struct {
				Model    string    `json:"model"`
				Messages []Message `json:"messages"`
			}
			if json.NewDecoder(r.Body).Decode(&input) != nil {
				t.Error("invalid request")
			}
			if input.Model != "a-model" || len(input.Messages) != 2 || input.Messages[0].Role != "system" || !strings.Contains(input.Messages[0].Content, "Зови себя Ася") || !strings.Contains(input.Messages[0].Content, "нет инструментов") || input.Messages[1].Content != "VPN не работает" {
				t.Errorf("wrong model/prompt/history: %+v", input)
			}
			w.Write([]byte(`{"choices":[{"message":{"content":"{\"reply\":\"Здравствуйте! Уточните устройство.\",\"handoff\":false}"},"finish_reason":"stop"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := NewClient()
	models, err := c.Models(context.Background(), server.URL, "third-party-test-key")
	if err != nil || !reflect.DeepEqual(models, []string{"a-model", "z-model"}) {
		t.Fatalf("models: %v %v", models, err)
	}
	reply, err := c.Respond(context.Background(), server.URL, "third-party-test-key", "a-model", "Зови себя Ася", []Message{{Role: "user", Content: "VPN не работает"}})
	if err != nil || reply.Handoff || reply.Text != "Здравствуйте! Уточните устройство." {
		t.Fatalf("reply: %+v %v", reply, err)
	}
}

func TestProviderErrorsNeverExposeCredentials(t *testing.T) {
	for _, status := range []int{401, 403, 429, 500} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			w.Write([]byte(`{"error":"secret-test-key"}`))
		}))
		_, err := NewClient().Models(context.Background(), server.URL, "secret-test-key")
		server.Close()
		if err == nil || strings.Contains(err.Error(), "secret-test-key") {
			t.Fatalf("unsafe error: %v", err)
		}
	}
}

func TestNoCredentialForwardingOnRedirect(t *testing.T) {
	forwarded := false
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded = true
		w.Write([]byte(`{"data":[{"id":"model"}]}`))
	}))
	defer other.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/v1/models", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	_, err := NewClient().Models(context.Background(), server.URL, "test-key")
	if err == nil || forwarded {
		t.Fatal("followed credential-bearing redirect")
	}
}

func TestMalformedOrTruncatedReplyEscalates(t *testing.T) {
	for _, content := range []string{`{"choices":[]}`, `{"choices":[{"message":{"content":"plain text"}}]}`, `{"choices":[{"message":{"content":"{\"reply\":\"ok\",\"handoff\":false}"},"finish_reason":"length"}]}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(content)) }))
		_, err := NewClient().Respond(context.Background(), server.URL, "test", "model", "", nil)
		server.Close()
		if err == nil {
			t.Fatal("accepted unusable reply")
		}
	}
}

func TestOperatorRequests(t *testing.T) {
	for _, text := range []string{"позови оператора", "Позови админа!", "Нужен администратор", "хочу живого человека", "Хочу поговорить с человеком", "Соедините с сотрудником поддержки", "Please connect me to a human", "I want to speak to a person", "operator please"} {
		if !WantsOperator(text) {
			t.Errorf("missed operator: %q", text)
		}
	}
	for _, text := range []string{"VPN не работает", "Как настроить айфон?", "Спасибо, помогло", "Не зови оператора", "Администратор пока не нужен", "Don't call an operator"} {
		if WantsOperator(text) {
			t.Errorf("false escalation: %q", text)
		}
	}
}

func TestURLValidation(t *testing.T) {
	for _, test := range []struct{ raw, want string }{{"https://provider.example/", "https://provider.example/v1"}, {"http://127.0.0.1:4500/v1/", "http://127.0.0.1:4500/v1"}, {"https://provider.example/api/v1", "https://provider.example/api/v1"}, {"https://provider.example/v1beta/openai/", "https://provider.example/v1beta/openai"}} {
		got, err := NormalizeURL(test.raw)
		if err != nil || got != test.want {
			t.Errorf("%q -> %q %v", test.raw, got, err)
		}
	}
	for _, raw := range []string{"file:///secret", "ftp://server", "https://user:secret@server/v1", "https://server/v1?key=secret", "https://server/#secret", "server/v1"} {
		if _, err := NormalizeURL(raw); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
}
