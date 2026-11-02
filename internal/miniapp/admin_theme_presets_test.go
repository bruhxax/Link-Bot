package miniapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v4"
	"link-bot/internal/database"
)

type themeDownloadTestStore struct{ item database.ThemePreset }

func (s themeDownloadTestStore) List(context.Context) ([]database.ThemePreset, error) {
	return nil, nil
}
func (s themeDownloadTestStore) Get(_ context.Context, id int64) (*database.ThemePreset, error) {
	if id != s.item.ID {
		return nil, pgx.ErrNoRows
	}
	return &s.item, nil
}
func (s themeDownloadTestStore) Create(context.Context, string, json.RawMessage) (*database.ThemePreset, error) {
	return nil, nil
}
func (s themeDownloadTestStore) Delete(context.Context, int64) (bool, error) { return false, nil }

func TestThemeDownloadIsAnAttachmentWithShortLivedSignature(t *testing.T) {
	store := themeDownloadTestStore{item: database.ThemePreset{ID: 42, Name: "Зимняя тема", Appearance: json.RawMessage(`{"backgroundMode":"twinkle"}`)}}
	expires := time.Now().Add(5 * time.Minute).Unix()
	url := "/api/mini-app/theme-download?id=42&expires=" + strconv.FormatInt(expires, 10) + "&signature=" + signThemeDownload("secret", 42, expires)
	w := httptest.NewRecorder()
	serveThemeDownload(w, httptest.NewRequest(http.MethodGet, url, nil), "secret", store)
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment;") || w.Header().Get("Access-Control-Allow-Origin") != "https://web.telegram.org" {
		t.Fatalf("download response: %d %v", w.Code, w.Header())
	}
	var file struct {
		Format     string          `json:"format"`
		Name       string          `json:"name"`
		Appearance json.RawMessage `json:"appearance"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &file); err != nil || file.Format != "bruhvpn-theme" || file.Name != store.item.Name || len(file.Appearance) == 0 {
		t.Fatalf("download body: %s, error: %v", w.Body.String(), err)
	}
	head := httptest.NewRecorder()
	serveThemeDownload(head, httptest.NewRequest(http.MethodHead, url, nil), "secret", store)
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") == "" {
		t.Fatalf("HEAD download response: %d %v", head.Code, head.Header())
	}
	for _, invalid := range []string{strings.Replace(url, "signature=", "signature=00", 1), "/api/mini-app/theme-download?id=42&expires=1&signature=" + signThemeDownload("secret", 42, 1)} {
		denied := httptest.NewRecorder()
		serveThemeDownload(denied, httptest.NewRequest(http.MethodGet, invalid, nil), "secret", store)
		if denied.Code != http.StatusNotFound {
			t.Fatalf("invalid download returned %d", denied.Code)
		}
	}
}
