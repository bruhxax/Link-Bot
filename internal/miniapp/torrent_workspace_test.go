package miniapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/google/uuid"
	"link-bot/internal/database"
)

type torrentWorkspaceMemory struct {
	*torrentMemory
	reports       []database.TorrentReport
	userDone      map[int64]bool
	query, status string
}

func newTorrentWorkspaceMemory() *torrentWorkspaceMemory {
	return &torrentWorkspaceMemory{torrentMemory: newTorrentMemory(), userDone: map[int64]bool{}}
}
func (m *torrentWorkspaceMemory) SaveTorrentReport(_ context.Context, r database.TorrentReport) error {
	m.reports = []database.TorrentReport{r}
	return nil
}
func (m *torrentWorkspaceMemory) ListTorrentReports(_ context.Context, q, s string, _ int64) ([]database.TorrentReport, bool, database.TorrentSummary, error) {
	m.query = q
	m.status = s
	return append([]database.TorrentReport{}, m.reports...), false, database.TorrentSummary{Total: 1}, nil
}
func (m *torrentWorkspaceMemory) TorrentRecipients(context.Context, uuid.UUID, int64) ([]int64, error) {
	return []int64{42}, nil
}
func (m *torrentWorkspaceMemory) ClaimTorrentUserAlert(_ context.Context, _ string, id int64, _ uuid.UUID) (bool, bool, error) {
	return !m.userDone[id], m.userDone[id], nil
}
func (m *torrentWorkspaceMemory) FinishTorrentUserAlert(_ context.Context, _ string, id int64, _ uuid.UUID, done bool) error {
	m.userDone[id] = done
	return nil
}

func TestTorrentSeparateRecipientsRetryWithoutRepeatingAdmin(t *testing.T) {
	initMiniAppTestConfig()
	secret := strings.Repeat("s", 32)
	t.Setenv("REMNAWAVE_WEBHOOK_SECRET", secret)
	messages := map[string][]string{}
	failUser := true
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseMultipartForm(1 << 20)
		id := r.FormValue("chat_id")
		if id == "42" && failUser {
			w.WriteHeader(502)
			return
		}
		messages[id] = append(messages[id], r.FormValue("text"))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"result":{"message_id":1,"chat":{"id":1,"type":"private"}}}`))
	}))
	defer api.Close()
	tg, err := bot.New("test-token", bot.WithSkipGetMe(), bot.WithServerURL(api.URL))
	if err != nil {
		t.Fatal(err)
	}
	store := newTorrentWorkspaceMemory()
	h := &Handler{telegramBot: tg, torrentAlertRepository: store}
	var data map[string]any
	json.Unmarshal(torrentFixture(t, true), &data)
	action := data["data"].(map[string]any)["report"].(map[string]any)["actionReport"].(map[string]any)
	action["willUnblockAt"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	body, _ := json.Marshal(data)
	call := func() int {
		w := httptest.NewRecorder()
		h.handleRemnawaveSecurityWebhook(w, torrentRequest(body, secret))
		return w.Code
	}
	if code := call(); code != 503 || len(messages["1"]) != 1 || len(messages["42"]) != 0 {
		t.Fatalf("partial delivery: %d %+v", code, messages)
	}
	failUser = false
	for i := 0; i < 2; i++ {
		if code := call(); code != 204 {
			t.Fatalf("retry: %d", code)
		}
	}
	if len(messages["1"]) != 1 || len(messages["42"]) != 1 {
		t.Fatal(messages)
	}
	user, admin := messages["42"][0], messages["1"][0]
	if !strings.Contains(user, "Node &lt;one&gt;") || !strings.Contains(user, "1 ч.") || strings.Contains(user, "2001:db8::1") || !strings.Contains(admin, "2001:db8::1") {
		t.Fatalf("wrong audience fields user=%s admin=%s", user, admin)
	}
	if len(store.entries) != 1 || len(store.reports) != 1 {
		t.Fatal("report duplicated")
	}
}

func TestTorrentWorkspacePermissionAndQueries(t *testing.T) {
	store := newTorrentWorkspaceMemory()
	store.reports = []database.TorrentReport{{ID: 1, Username: "alice", CustomerID: 99}}
	h := &Handler{torrentAlertRepository: store}
	for _, permissions := range [][]string{{"users"}, {"torrents"}, {"torrents", "users"}} {
		sess := &session{User: telegramUser{ID: 123}, AdminAccess: adminAccess{IsAdmin: true, Permissions: permissions}}
		w := httptest.NewRecorder()
		h.handleAdminTorrentReports(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"query":"alice","status":"active"}`)), sess, nil)
		if len(permissions) == 1 && permissions[0] == "users" {
			if w.Code != 403 {
				t.Fatal(w.Code)
			}
			continue
		}
		if w.Code != 200 || store.query != "alice" || store.status != "active" {
			t.Fatalf("request: %d %s", w.Code, w.Body.String())
		}
		if len(permissions) == 1 && strings.Contains(w.Body.String(), `"customerId":99`) {
			t.Fatal("exposed customer action without users permission")
		}
	}
	if adminRouteAllowed(adminAccess{IsAdmin: true, Permissions: []string{"users"}}, "/api/mini-app/admin/torrents/reports") {
		t.Fatal("route bypasses permission")
	}
}
