package miniapp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"link-bot/internal/database"
	"link-bot/utils"
)

const (
	maxLogoUploadSize  int64 = 2 << 20
	maxLogoRequestSize int64 = maxLogoUploadSize + (256 << 10)
)

var (
	errLogoTooLarge        = errors.New("logo is too large")
	errLogoUnsupported     = errors.New("unsupported logo format")
	errLogoDimensions      = errors.New("invalid logo dimensions")
	uploadedLogoNameRegexp = regexp.MustCompile(`^logo-[0-9a-f]{16}\.(?:png|jpg|webp)$`)
)

func (h *Handler) handleAdminLogoUpload(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if !h.isAdmin(sess.User.ID) {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}
	if strings.TrimSpace(h.logoUploadDir) == "" {
		h.writeError(w, http.StatusServiceUnavailable, "logo_storage_unavailable", "Logo storage is unavailable")
		return
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))), "multipart/form-data") {
		h.writeError(w, http.StatusUnsupportedMediaType, "logo_upload_invalid", "Multipart form data required")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxLogoRequestSize)
	if err := r.ParseMultipartForm(maxLogoUploadSize); err != nil {
		h.writeError(w, http.StatusBadRequest, "logo_upload_invalid", "Could not read the logo file")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	file, _, err := r.FormFile("logo")
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "logo_upload_missing", "Select a logo file")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxLogoUploadSize+1))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "logo_upload_invalid", "Could not read the logo file")
		return
	}
	logoURL, err := storeUploadedLogo(h.logoUploadDir, data)
	if err != nil {
		switch {
		case errors.Is(err, errLogoTooLarge):
			h.writeError(w, http.StatusRequestEntityTooLarge, "logo_too_large", "Logo must be no larger than 2 MB")
		case errors.Is(err, errLogoUnsupported):
			h.writeError(w, http.StatusUnsupportedMediaType, "logo_format_invalid", "Use PNG, JPG or WebP")
		case errors.Is(err, errLogoDimensions):
			h.writeError(w, http.StatusBadRequest, "logo_dimensions_invalid", "Logo dimensions must not exceed 4096×4096")
		default:
			slog.Error("mini app: save uploaded logo failed", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID))
			h.writeError(w, http.StatusInternalServerError, "logo_upload_failed", "Could not save the logo")
		}
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"data": map[string]string{
			"url": logoURL,
		},
	})
}

func storeUploadedLogo(uploadDir string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", errLogoUnsupported
	}
	if int64(len(data)) > maxLogoUploadSize {
		return "", errLogoTooLarge
	}

	contentType := http.DetectContentType(data)
	extension := ""
	switch contentType {
	case "image/png":
		extension = ".png"
	case "image/jpeg":
		extension = ".jpg"
	case "image/webp":
		extension = ".webp"
	default:
		return "", errLogoUnsupported
	}

	if contentType != "image/webp" {
		configuration, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return "", errLogoUnsupported
		}
		if configuration.Width <= 0 || configuration.Height <= 0 || configuration.Width > 4096 || configuration.Height > 4096 {
			return "", errLogoDimensions
		}
	}

	uploadDir = strings.TrimSpace(uploadDir)
	if uploadDir == "" {
		return "", errors.New("logo upload directory is empty")
	}
	if err := os.MkdirAll(uploadDir, 0o750); err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	fileName := "logo-" + hex.EncodeToString(hash[:8]) + extension
	finalPath := filepath.Join(uploadDir, fileName)
	if info, err := os.Stat(finalPath); err == nil && info.Mode().IsRegular() {
		return "/mini-app/uploads/" + fileName, nil
	}

	temporary, err := os.CreateTemp(uploadDir, ".logo-upload-*")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()

	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		if info, statErr := os.Stat(finalPath); statErr != nil || !info.Mode().IsRegular() {
			return "", err
		}
	}
	removeTemporary = false

	return "/mini-app/uploads/" + fileName, nil
}

func (h *Handler) serveUploadedLogo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fileName := strings.TrimPrefix(r.URL.Path, "/mini-app/uploads/")
	if !uploadedLogoNameRegexp.MatchString(fileName) || strings.TrimSpace(h.logoUploadDir) == "" {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(filepath.Join(h.logoUploadDir, fileName))
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

	setCommonSecurityHeaders(w)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	switch filepath.Ext(fileName) {
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".webp":
		w.Header().Set("Content-Type", "image/webp")
	}
	http.ServeContent(w, r, fileName, info.ModTime(), file)
}
