package miniapp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/utils"
)

const (
	maxSupportMediaSize    int64 = 50 << 20
	maxSupportMediaRequest int64 = maxSupportMediaSize + (1 << 20)
)

var (
	errSupportMediaTooLarge    = errors.New("support media is too large")
	errSupportMediaUnsupported = errors.New("unsupported support media format")
	errSupportMediaDimensions  = errors.New("invalid support media dimensions")
	supportMediaNameRegexp     = regexp.MustCompile(`^support-[0-9a-f]{32}\.(?:jpg|png|webp|gif|mp4|webm|mov)$`)
)

type supportMediaRequest struct {
	MessageID int64 `json:"messageId"`
}

func (h *Handler) handleSupportMediaUpload(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if h.supportRepository == nil {
		h.writeError(w, http.StatusServiceUnavailable, "support_unavailable", "Поддержка временно недоступна")
		return
	}
	if strings.TrimSpace(h.logoUploadDir) == "" {
		h.writeError(w, http.StatusServiceUnavailable, "support_media_storage_unavailable", "Хранилище медиа временно недоступно")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxSupportMediaRequest)
	if err := r.ParseMultipartForm(maxSupportMediaSize); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "request body too large") {
			h.writeError(w, http.StatusRequestEntityTooLarge, "support_media_too_large", "Файл должен быть не больше 50 МБ")
			return
		}
		h.writeError(w, http.StatusBadRequest, "support_media_invalid", "Не удалось прочитать файл")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	ticketID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("ticketId")), 10, 64)
	if err != nil || ticketID <= 0 {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}
	caption := strings.TrimSpace(r.FormValue("caption"))
	if len([]rune(caption)) > 2000 {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}

	ticket, err := h.loadSupportTicketForViewer(r.Context(), sess, customer, ticketID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "support_send_failed", "Не удалось отправить файл")
		return
	}
	if ticket == nil {
		h.writeError(w, http.StatusNotFound, "support_ticket_not_found", "Обращение не найдено")
		return
	}
	if ticket.Status == database.SupportTicketStatusClosed {
		h.writeError(w, http.StatusBadRequest, "support_ticket_closed", "Обращение уже закрыто")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "support_media_missing", "Выберите фото или видео")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxSupportMediaSize+1))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "support_media_invalid", "Не удалось прочитать файл")
		return
	}
	attachment, created, err := storeSupportMedia(h.logoUploadDir, data, header.Filename)
	if err != nil {
		switch {
		case errors.Is(err, errSupportMediaTooLarge):
			h.writeError(w, http.StatusRequestEntityTooLarge, "support_media_too_large", "Файл должен быть не больше 50 МБ")
		case errors.Is(err, errSupportMediaUnsupported):
			h.writeError(w, http.StatusUnsupportedMediaType, "support_media_format_invalid", "Поддерживаются JPG, PNG, WebP, GIF, MP4, WebM и MOV")
		case errors.Is(err, errSupportMediaDimensions):
			h.writeError(w, http.StatusBadRequest, "support_media_dimensions_invalid", "Размер изображения не должен превышать 8192×8192")
		default:
			slog.Error("mini app: store support media failed", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID), "ticketId", ticket.ID)
			h.writeError(w, http.StatusInternalServerError, "support_media_upload_failed", "Не удалось сохранить файл")
		}
		return
	}
	removeCreated := func() {
		if created {
			_ = os.Remove(filepath.Join(h.logoUploadDir, attachment.StorageName))
		}
	}

	var stored *database.SupportMessage
	if h.isAdmin(sess.User.ID) {
		stored, err = h.supportRepository.AddAdminMediaMessage(r.Context(), ticket.ID, sess.User.ID, caption, attachment)
	} else {
		highestPurchase, purchaseErr := h.purchaseRepository.FindHighestSuccessfulPurchaseByCustomer(r.Context(), customer.ID)
		if purchaseErr != nil {
			removeCreated()
			h.writeError(w, http.StatusInternalServerError, "support_send_failed", "Не удалось отправить файл")
			return
		}
		panelUsername := h.resolveSupportPanelUsername(r.Context(), customer)
		stored, err = h.supportRepository.AddCustomerMediaMessage(
			r.Context(),
			ticket.ID,
			sess.User.ID,
			caption,
			panelUsername,
			strings.TrimPrefix(strings.TrimSpace(sess.User.Username), "@"),
			h.buildSubscriptionLabel(customer, highestPurchase),
			attachment,
		)
	}
	if err != nil || stored == nil {
		removeCreated()
		slog.Error("mini app: add support media message failed", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID), "ticketId", ticket.ID)
		h.writeError(w, http.StatusInternalServerError, "support_send_failed", "Не удалось отправить файл")
		return
	}

	notificationText := supportMediaNotificationText(caption, attachment.Type)
	if h.isAdmin(sess.User.ID) {
		h.notifySupportAsync(func(ctx context.Context) {
			h.notifyCustomerAboutSupportReply(ctx, ticket, notificationText)
		})
	} else {
		h.notifySupportAsync(func(ctx context.Context) {
			h.notifyAdminAboutSupportReply(ctx, ticket, notificationText)
		})
	}

	updatedTicket, err := h.supportRepository.FindTicketByID(r.Context(), ticket.ID)
	if err != nil || updatedTicket == nil {
		h.writeError(w, http.StatusInternalServerError, "support_send_failed", "Не удалось обновить переписку")
		return
	}
	threadPayload, err := h.buildSupportThreadPayload(r.Context(), sess, customer, updatedTicket)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "support_send_failed", "Не удалось обновить переписку")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": threadPayload})
}

func (h *Handler) handleSupportMediaLink(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if h.supportRepository == nil || strings.TrimSpace(h.logoUploadDir) == "" {
		h.writeError(w, http.StatusServiceUnavailable, "support_media_unavailable", "Медиа временно недоступно")
		return
	}

	var req supportMediaRequest
	if err := h.decodeJSONRequest(w, r, 4096, &req); err != nil || req.MessageID <= 0 {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}
	message, err := h.supportRepository.FindMessageByID(r.Context(), req.MessageID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "support_media_failed", "Не удалось загрузить медиа")
		return
	}
	if message == nil || message.MediaType == "" || !supportMediaNameRegexp.MatchString(message.MediaStorageName) {
		h.writeError(w, http.StatusNotFound, "support_media_not_found", "Медиа не найдено")
		return
	}
	ticket, err := h.loadSupportTicketForViewer(r.Context(), sess, customer, message.TicketID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "support_media_failed", "Не удалось загрузить медиа")
		return
	}
	if ticket == nil {
		h.writeError(w, http.StatusNotFound, "support_media_not_found", "Медиа не найдено")
		return
	}
	expires := time.Now().UTC().Add(15 * time.Minute).Unix()
	signature, err := signSupportMediaURL(message.ID, expires)
	if err != nil {
		h.writeError(w, http.StatusServiceUnavailable, "support_media_unavailable", "Медиа временно недоступно")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"data": map[string]any{
			"url":     fmt.Sprintf("/mini-app/support-media/%d?expires=%d&signature=%s", message.ID, expires, signature),
			"expires": expires,
		},
	})
}

func (h *Handler) serveSupportMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	setCommonSecurityHeaders(w)
	w.Header().Set("Cache-Control", "private, no-store")
	if h.supportRepository == nil || strings.TrimSpace(h.logoUploadDir) == "" {
		http.NotFound(w, r)
		return
	}
	messageID, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/mini-app/support-media/"), 10, 64)
	if err != nil || messageID <= 0 {
		http.NotFound(w, r)
		return
	}
	expires, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("expires")), 10, 64)
	if err != nil || expires < time.Now().UTC().Unix() || expires > time.Now().UTC().Add(time.Hour).Unix() {
		http.NotFound(w, r)
		return
	}
	expected, err := signSupportMediaURL(messageID, expires)
	if err != nil || !hmac.Equal([]byte(expected), []byte(strings.TrimSpace(r.URL.Query().Get("signature")))) {
		http.NotFound(w, r)
		return
	}
	message, err := h.supportRepository.FindMessageByID(r.Context(), messageID)
	if err != nil || message == nil || message.MediaType == "" || !supportMediaNameRegexp.MatchString(message.MediaStorageName) {
		http.NotFound(w, r)
		return
	}

	file, err := os.Open(filepath.Join(h.logoUploadDir, message.MediaStorageName))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", message.MediaMIME)
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	disposition := "inline"
	if r.URL.Query().Get("download") == "1" {
		disposition = "attachment"
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": message.MediaOriginalName}))
	http.ServeContent(w, r, message.MediaOriginalName, info.ModTime(), file)
}

func signSupportMediaURL(messageID, expires int64) (string, error) {
	key := strings.TrimSpace(config.TelegramToken())
	if key == "" {
		return "", errors.New("telegram token is not configured")
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = fmt.Fprintf(mac, "%d:%d", messageID, expires)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func storeSupportMedia(uploadDir string, data []byte, originalName string) (database.SupportAttachment, bool, error) {
	if len(data) == 0 {
		return database.SupportAttachment{}, false, errSupportMediaUnsupported
	}
	if int64(len(data)) > maxSupportMediaSize {
		return database.SupportAttachment{}, false, errSupportMediaTooLarge
	}

	kind, mediaMIME, extension, err := detectSupportMedia(data)
	if err != nil {
		return database.SupportAttachment{}, false, err
	}
	uploadDir = strings.TrimSpace(uploadDir)
	if uploadDir == "" {
		return database.SupportAttachment{}, false, errors.New("support media upload directory is empty")
	}
	if err := os.MkdirAll(uploadDir, 0o750); err != nil {
		return database.SupportAttachment{}, false, err
	}

	hash := sha256.Sum256(data)
	storageName := "support-" + hex.EncodeToString(hash[:16]) + extension
	attachment := database.SupportAttachment{
		Type:         kind,
		MIME:         mediaMIME,
		StorageName:  storageName,
		OriginalName: sanitizeSupportMediaName(originalName, kind, extension),
		SizeBytes:    int64(len(data)),
	}
	finalPath := filepath.Join(uploadDir, storageName)
	if info, statErr := os.Stat(finalPath); statErr == nil && info.Mode().IsRegular() {
		return attachment, false, nil
	}

	temporary, err := os.CreateTemp(uploadDir, ".support-upload-*")
	if err != nil {
		return database.SupportAttachment{}, false, err
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o640); err != nil {
		_ = temporary.Close()
		return database.SupportAttachment{}, false, err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return database.SupportAttachment{}, false, err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return database.SupportAttachment{}, false, err
	}
	if err := temporary.Close(); err != nil {
		return database.SupportAttachment{}, false, err
	}
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		if info, statErr := os.Stat(finalPath); statErr != nil || !info.Mode().IsRegular() {
			return database.SupportAttachment{}, false, err
		}
		return attachment, false, nil
	}
	removeTemporary = false
	return attachment, true, nil
}

func detectSupportMedia(data []byte) (string, string, string, error) {
	detected := http.DetectContentType(data)
	switch detected {
	case "image/jpeg":
		if err := validateSupportImageDimensions(data); err != nil {
			return "", "", "", err
		}
		return "image", "image/jpeg", ".jpg", nil
	case "image/png":
		if err := validateSupportImageDimensions(data); err != nil {
			return "", "", "", err
		}
		return "image", "image/png", ".png", nil
	case "image/gif":
		if err := validateSupportImageDimensions(data); err != nil {
			return "", "", "", err
		}
		return "image", "image/gif", ".gif", nil
	case "image/webp":
		return "image", "image/webp", ".webp", nil
	case "video/mp4":
		return detectISOBaseMedia(data)
	case "video/webm":
		return "video", "video/webm", ".webm", nil
	}
	if len(data) >= 12 && string(data[4:8]) == "ftyp" {
		return detectISOBaseMedia(data)
	}
	if len(data) >= 4 && bytes.Equal(data[:4], []byte{0x1a, 0x45, 0xdf, 0xa3}) {
		return "video", "video/webm", ".webm", nil
	}
	return "", "", "", errSupportMediaUnsupported
}

func detectISOBaseMedia(data []byte) (string, string, string, error) {
	if len(data) < 12 || string(data[4:8]) != "ftyp" {
		return "", "", "", errSupportMediaUnsupported
	}
	if string(data[8:12]) == "qt  " {
		return "video", "video/quicktime", ".mov", nil
	}
	return "video", "video/mp4", ".mp4", nil
}

func validateSupportImageDimensions(data []byte) error {
	configuration, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return errSupportMediaUnsupported
	}
	if configuration.Width <= 0 || configuration.Height <= 0 || configuration.Width > 8192 || configuration.Height > 8192 {
		return errSupportMediaDimensions
	}
	return nil
}

func sanitizeSupportMediaName(name, kind, extension string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '/' || r == '\\' {
			return -1
		}
		return r
	}, name)
	runes := []rune(name)
	if len(runes) > 120 {
		name = string(runes[:120])
	}
	if strings.TrimSpace(name) == "" || name == "." || name == ".." {
		if kind == "video" {
			return "video" + extension
		}
		return "photo" + extension
	}
	return name
}

func supportMediaNotificationText(caption, kind string) string {
	label := "Фото"
	if kind == "video" {
		label = "Видео"
	}
	caption = strings.TrimSpace(caption)
	if caption == "" {
		return label
	}
	return fmt.Sprintf("%s: %s", label, caption)
}
