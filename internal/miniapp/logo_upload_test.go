package miniapp

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"link-bot/internal/database"
)

func testLogoPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	canvas := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			canvas.Set(x, y, color.NRGBA{R: 255, G: 120, B: 0, A: 255})
		}
	}
	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		t.Fatalf("encode logo: %v", err)
	}
	return output.Bytes()
}

func TestStoreUploadedLogoPersistsContentAddressedFile(t *testing.T) {
	uploadDir := t.TempDir()
	data := testLogoPNG(t, 256, 256)

	logoURL, err := storeUploadedLogo(uploadDir, data)
	if err != nil {
		t.Fatalf("store logo: %v", err)
	}
	if !strings.HasPrefix(logoURL, "/mini-app/uploads/logo-") || !strings.HasSuffix(logoURL, ".png") {
		t.Fatalf("unexpected logo URL: %q", logoURL)
	}
	fileName := strings.TrimPrefix(logoURL, "/mini-app/uploads/")
	stored, err := os.ReadFile(filepath.Join(uploadDir, fileName))
	if err != nil {
		t.Fatalf("read stored logo: %v", err)
	}
	if !bytes.Equal(stored, data) {
		t.Fatal("stored logo bytes differ from upload")
	}

	duplicateURL, err := storeUploadedLogo(uploadDir, data)
	if err != nil {
		t.Fatalf("store duplicate logo: %v", err)
	}
	if duplicateURL != logoURL {
		t.Fatalf("duplicate URL = %q, want %q", duplicateURL, logoURL)
	}
}

func TestStoreUploadedLogoRejectsUnsafeOrOversizedFiles(t *testing.T) {
	uploadDir := t.TempDir()
	if _, err := storeUploadedLogo(uploadDir, []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)); err != errLogoUnsupported {
		t.Fatalf("SVG error = %v, want %v", err, errLogoUnsupported)
	}
	if _, err := storeUploadedLogo(uploadDir, make([]byte, maxLogoUploadSize+1)); err != errLogoTooLarge {
		t.Fatalf("oversized error = %v, want %v", err, errLogoTooLarge)
	}
	if _, err := storeUploadedLogo(uploadDir, testLogoPNG(t, 4097, 1)); err != errLogoDimensions {
		t.Fatalf("dimensions error = %v, want %v", err, errLogoDimensions)
	}
}

func TestAdminLogoUploadAndPublicServing(t *testing.T) {
	initMiniAppTestConfig()
	uploadDir := t.TempDir()
	handler := &Handler{logoUploadDir: uploadDir}
	data := testLogoPNG(t, 128, 128)

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	part, err := writer.CreateFormFile("logo", "brand.png")
	if err != nil {
		t.Fatalf("create logo form field: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write logo form field: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/mini-app/admin/logo/upload", &requestBody)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	handler.handleAdminLogoUpload(recorder, request, &session{User: telegramUser{ID: 1}}, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if !payload.OK || payload.Data.URL == "" {
		t.Fatalf("invalid upload response: %s", recorder.Body.String())
	}

	serveRequest := httptest.NewRequest(http.MethodGet, payload.Data.URL, nil)
	serveRecorder := httptest.NewRecorder()
	handler.serveUploadedLogo(serveRecorder, serveRequest)
	if serveRecorder.Code != http.StatusOK {
		t.Fatalf("serve status = %d", serveRecorder.Code)
	}
	if contentType := serveRecorder.Header().Get("Content-Type"); contentType != "image/png" {
		t.Fatalf("content type = %q", contentType)
	}
	if cacheControl := serveRecorder.Header().Get("Cache-Control"); !strings.Contains(cacheControl, "immutable") {
		t.Fatalf("cache control = %q", cacheControl)
	}
	if !bytes.Equal(serveRecorder.Body.Bytes(), data) {
		t.Fatal("served logo bytes differ from upload")
	}
}

func TestAdminLogoUploadSessionAcceptsMultipartRequest(t *testing.T) {
	initMiniAppTestConfig()
	staticFS, err := fs.Sub(embeddedStatic, "static")
	if err != nil {
		t.Fatalf("open embedded mini app files: %v", err)
	}
	handler := &Handler{logoUploadDir: t.TempDir(), staticFS: staticFS}
	mux := http.NewServeMux()
	handler.Register(mux)

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	part, err := writer.CreateFormFile("logo", "brand.png")
	if err != nil {
		t.Fatalf("create logo form field: %v", err)
	}
	if _, err := part.Write(testLogoPNG(t, 64, 64)); err != nil {
		t.Fatalf("write logo form field: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/mini-app/admin/logo/upload", &requestBody)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("X-Telegram-Init-Data", "invalid-test-init-data")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("multipart request status = %d, body = %s; want auth validation to run", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "JSON required") {
		t.Fatalf("multipart request was rejected as JSON: %s", recorder.Body.String())
	}
}

func TestSessionJSONRouteStillRejectsMultipartRequest(t *testing.T) {
	handler := &Handler{}
	request := httptest.NewRequest(http.MethodPost, "/api/mini-app/bootstrap", strings.NewReader("body"))
	request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	recorder := httptest.NewRecorder()

	handler.withSession(func(http.ResponseWriter, *http.Request, *session, *database.Customer) {
		t.Fatal("JSON route handler should not be called")
	}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("multipart request status = %d, want %d", recorder.Code, http.StatusUnsupportedMediaType)
	}
}

func TestServeUploadedLogoRejectsUnknownPaths(t *testing.T) {
	handler := &Handler{logoUploadDir: t.TempDir()}
	for _, target := range []string{
		"/mini-app/uploads/../settings.json",
		"/mini-app/uploads/not-a-logo.png",
		"/mini-app/uploads/",
	} {
		recorder := httptest.NewRecorder()
		handler.serveUploadedLogo(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", target, recorder.Code)
		}
	}
}
