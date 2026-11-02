package miniapp

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"link-bot/internal/database"
	"link-bot/internal/runtimeconfig"
)

type themePresetRequest struct {
	Action     string          `json:"action"`
	ID         int64           `json:"id"`
	Name       string          `json:"name"`
	Appearance json.RawMessage `json:"appearance"`
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
	if r.URL.Path == "/api/mini-app/admin/appearance/presets" && req.Action == "list" {
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
