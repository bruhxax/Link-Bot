package miniapp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v4"
	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/internal/runtimeconfig"
)

type themePresetRequest struct {
	Action     string          `json:"action"`
	ID         int64           `json:"id"`
	Name       string          `json:"name"`
	Appearance json.RawMessage `json:"appearance"`
}

type themePresetStore interface {
	List(context.Context) ([]database.ThemePreset, error)
	Get(context.Context, int64) (*database.ThemePreset, error)
	Create(context.Context, string, json.RawMessage) (*database.ThemePreset, error)
	Delete(context.Context, int64) (bool, error)
}

func (h *Handler) SetThemePresetRepository(repository *database.ThemePresetRepository) {
	h.themePresetRepository = repository
}

func validThemePresetName(name string) bool {
	if utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 48 {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func (h *Handler) handleAdminThemePresets(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !sess.canAdmin("appearance") {
		h.writeError(w, http.StatusForbidden, "forbidden", "Нет доступа к оформлению")
		return
	}
	if h.themePresetRepository == nil {
		h.writeError(w, http.StatusServiceUnavailable, "unavailable", "Пресеты недоступны")
		return
	}
	var req themePresetRequest
	if err := h.decodeJSONRequest(w, r, 512<<10, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}
	if r.URL.Path == "/api/mini-app/admin/appearance/presets/list" && req.Action != "list" {
		h.writeError(w, http.StatusBadRequest, "invalid_action", "Неизвестное действие")
		return
	}
	if r.URL.Path == "/api/mini-app/admin/appearance/presets/export" && req.Action != "export" {
		h.writeError(w, http.StatusBadRequest, "invalid_action", "Неизвестное действие")
		return
	}
	if r.URL.Path == "/api/mini-app/admin/appearance/presets" && (req.Action == "list" || req.Action == "export") {
		h.writeError(w, http.StatusBadRequest, "invalid_action", "Неизвестное действие")
		return
	}
	switch req.Action {
	case "list":
		items, err := h.themePresetRepository.List(r.Context())
		if err != nil {
			slog.Error("theme presets: list failed", "error", err)
			h.writeError(w, http.StatusInternalServerError, "list_failed", "Не удалось загрузить темы")
			return
		}
		h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": items})
	case "export":
		if req.ID <= 0 {
			h.writeError(w, http.StatusBadRequest, "invalid_request", "Выберите тему")
			return
		}
		if _, err := h.themePresetRepository.Get(r.Context(), req.ID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				h.writeError(w, http.StatusNotFound, "not_found", "Тема уже удалена")
			} else {
				slog.Error("theme presets: export lookup failed", "error", err)
				h.writeError(w, http.StatusInternalServerError, "export_failed", "Не удалось подготовить файл темы")
			}
			return
		}
		secret := strings.TrimSpace(config.TelegramToken())
		if secret == "" {
			h.writeError(w, http.StatusServiceUnavailable, "unavailable", "Скачивание темы недоступно")
			return
		}
		expires := time.Now().Add(5 * time.Minute).Unix()
		signature := signThemeDownload(secret, req.ID, expires)
		filename := fmt.Sprintf("theme-%d.bruhvpn-theme.json", req.ID)
		h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": map[string]string{
			"url":      fmt.Sprintf("/api/mini-app/theme-download?id=%d&expires=%d&signature=%s", req.ID, expires, signature),
			"fileName": filename,
		}})
	case "create":
		name := strings.TrimSpace(req.Name)
		if !validThemePresetName(name) {
			h.writeError(w, http.StatusBadRequest, "invalid_name", "Название должно содержать от 1 до 48 символов")
			return
		}
		var appearance runtimeconfig.AppearanceSettings
		if err := json.Unmarshal(req.Appearance, &appearance); err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid_theme", "Некорректный файл темы")
			return
		}
		appearance, err := runtimeconfig.ValidateThemeAppearance(appearance)
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid_theme", err.Error())
			return
		}
		raw, err := json.Marshal(appearance)
		if err != nil {
			h.writeError(w, http.StatusInternalServerError, "save_failed", "Не удалось сохранить тему")
			return
		}
		item, err := h.themePresetRepository.Create(r.Context(), name, raw)
		if errors.Is(err, database.ErrThemePresetExists) {
			h.writeError(w, http.StatusConflict, "theme_exists", "Тема с таким названием уже есть")
			return
		}
		if errors.Is(err, database.ErrThemePresetLimit) {
			h.writeError(w, http.StatusConflict, "theme_limit", "Можно сохранить не больше 20 тем")
			return
		}
		if err != nil {
			slog.Error("theme presets: create failed", "error", err)
			h.writeError(w, http.StatusInternalServerError, "save_failed", "Не удалось сохранить тему")
			return
		}
		h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": item})
	case "delete":
		if req.ID <= 0 {
			h.writeError(w, http.StatusBadRequest, "invalid_request", "Выберите тему")
			return
		}
		deleted, err := h.themePresetRepository.Delete(r.Context(), req.ID)
		if err != nil {
			slog.Error("theme presets: delete failed", "error", err)
			h.writeError(w, http.StatusInternalServerError, "delete_failed", "Не удалось удалить тему")
			return
		}
		if !deleted {
			h.writeError(w, http.StatusNotFound, "not_found", "Тема уже удалена")
			return
		}
		h.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		h.writeError(w, http.StatusBadRequest, "invalid_action", "Неизвестное действие")
	}
}

func signThemeDownload(secret string, id, expires int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = fmt.Fprintf(mac, "%d:%d", id, expires)
	return hex.EncodeToString(mac.Sum(nil))
}

func (h *Handler) handleThemeDownload(w http.ResponseWriter, r *http.Request) {
	serveThemeDownload(w, r, strings.TrimSpace(config.TelegramToken()), h.themePresetRepository)
}

func serveThemeDownload(w http.ResponseWriter, r *http.Request, secret string, repository themePresetStore) {
	w.Header().Set("Access-Control-Allow-Origin", "https://web.telegram.org")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, idErr := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	expires, expiryErr := strconv.ParseInt(r.URL.Query().Get("expires"), 10, 64)
	provided, signatureErr := hex.DecodeString(r.URL.Query().Get("signature"))
	if idErr != nil || expiryErr != nil || signatureErr != nil || id <= 0 || expires < time.Now().Unix() || expires > time.Now().Add(10*time.Minute).Unix() || secret == "" {
		http.NotFound(w, r)
		return
	}
	expected, _ := hex.DecodeString(signThemeDownload(secret, id, expires))
	if !hmac.Equal(provided, expected) || repository == nil {
		http.NotFound(w, r)
		return
	}
	item, err := repository.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	data, err := json.Marshal(struct {
		Format     string          `json:"format"`
		Version    int             `json:"version"`
		Name       string          `json:"name"`
		Appearance json.RawMessage `json:"appearance"`
	}{Format: "bruhvpn-theme", Version: 1, Name: item.Name, Appearance: item.Appearance})
	if err != nil {
		http.Error(w, "Could not export theme", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="theme-%d.bruhvpn-theme.json"`, id))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(data)
	}
}
