package miniapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCustomBackgroundUploadAndPublicServing(t *testing.T) {
	initMiniAppTestConfig()
	h := &Handler{logoUploadDir: t.TempDir()}
	data := testLogoPNG(t, 300, 200)
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, _ := form.CreateFormFile("background", "photo.wrong-extension")
	file.Write(data)
	form.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/mini-app/admin/background/upload", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	result := httptest.NewRecorder()
	h.handleAdminBackgroundUpload(result, req, &session{User: telegramUser{ID: 1}}, nil)
	if result.Code != 200 {
		t.Fatalf("upload: %d %s", result.Code, result.Body.String())
	}
	var payload struct{ Data struct{ URL, Type string } }
	if err := json.Unmarshal(result.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Type != "image" || !strings.HasSuffix(payload.Data.URL, ".png") {
		t.Fatalf("payload: %+v", payload)
	}
	stored, err := os.ReadFile(filepath.Join(h.logoUploadDir, filepath.Base(payload.Data.URL)))
	if err != nil || !bytes.Equal(data, stored) {
		t.Fatalf("stored file: %v", err)
	}
	req = httptest.NewRequest(http.MethodGet, payload.Data.URL, nil)
	result = httptest.NewRecorder()
	h.serveUploadedLogo(result, req)
	if result.Code != 200 || result.Header().Get("Content-Type") != "image/png" || !bytes.Equal(data, result.Body.Bytes()) {
		t.Fatal("background not served correctly")
	}
	for _, suffix := range []string{"../secret.png", "background-not-a-hash.png"} {
		result = httptest.NewRecorder()
		h.serveUploadedLogo(result, httptest.NewRequest("GET", "/mini-app/uploads/"+suffix, nil))
		if result.Code != 404 {
			t.Fatal("invalid upload path accepted")
		}
	}
}

func TestBackgroundFormatsAndLimits(t *testing.T) {
	formats := []struct {
		data      []byte
		ext, kind string
	}{
		{testLogoPNG(t, 2, 2), "png", "image"}, {testBannerGIF(t), "gif", "gif"},
		{[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="2" height="2"/></svg>`), "svg", "image"},
		{append([]byte{0, 0, 0, 24}, []byte("ftypavif00000000")...), "avif", "image"},
		{append([]byte{0, 0, 0, 24}, []byte("ftypisom00000000")...), "mp4", "video"},
		{append([]byte{0, 0, 0, 24}, []byte("ftypqt  00000000")...), "mov", "video"},
	}
	for _, f := range formats {
		ext, kind, err := backgroundFormat(f.data)
		if err != nil || ext != f.ext || kind != f.kind {
			t.Errorf("%s: %s %s %v", f.ext, ext, kind, err)
		}
	}
	for _, data := range [][]byte{nil, []byte("<html>not media</html>"), []byte("MZfake executable"), testBannerGIF(t)[:20]} {
		if _, _, err := backgroundFormat(data); err == nil {
			t.Error("invalid media accepted")
		}
	}
	huge := testBannerGIF(t)
	huge[6], huge[7], huge[8], huge[9] = 0xff, 0x7f, 0xff, 0x7f
	if validBackgroundGIF(huge) {
		t.Fatal("huge GIF accepted")
	}
	for _, mutation := range []func([]byte){
		func(data []byte) { pos := bytes.Index(data, []byte{0x21, 0xf9}); data[pos+1] = 0x00 },
		func(data []byte) { pos := bytes.Index(data, []byte{0x21, 0xf9}); data[pos+2] = 5 },
	} {
		data := testBannerGIF(t)
		mutation(data)
		if validBackgroundGIF(data) {
			t.Fatal("GIF extension that can desynchronize the client decoder accepted")
		}
	}
	if _, _, err := storeUploadedBackground(t.TempDir(), make([]byte, maxBackgroundSize+1)); !errors.Is(err, errBackgroundSize) {
		t.Fatal("oversized upload accepted")
	}
}

func TestGIFBackgroundCreatesAStaticThumbnail(t *testing.T) {
	h := &Handler{logoUploadDir: t.TempDir()}
	rec := httptest.NewRecorder()
	h.saveBackgroundResponse(rec, testBannerGIF(t))
	var payload struct {
		Data struct{ URL, Type, Poster string }
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || payload.Data.Type != "gif" || !strings.HasSuffix(payload.Data.Poster, ".png") {
		t.Fatalf("thumbnail response: %d %s", rec.Code, rec.Body.String())
	}
	thumb := httptest.NewRecorder()
	h.serveUploadedLogo(thumb, httptest.NewRequest("GET", payload.Data.Poster, nil))
	if thumb.Code != 200 || thumb.Header().Get("Content-Type") != "image/png" {
		t.Fatal("GIF thumbnail not served")
	}
}

func TestBackgroundSVGIsSandboxedAndVideoSupportsRanges(t *testing.T) {
	h := &Handler{logoUploadDir: t.TempDir()}
	url, _, err := storeUploadedBackground(h.logoUploadDir, []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.serveUploadedLogo(rec, httptest.NewRequest("GET", url, nil))
	if rec.Header().Get("Content-Type") != "image/svg+xml" || !strings.Contains(rec.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatal("SVG is not isolated")
	}
	video := append([]byte{0, 0, 0, 24}, []byte("ftypisom00000000")...)
	url, _, err = storeUploadedBackground(h.logoUploadDir, video)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", url, nil)
	req.Header.Set("Range", "bytes=0-7")
	rec = httptest.NewRecorder()
	h.serveUploadedLogo(rec, req)
	if rec.Code != 206 || !bytes.Equal(rec.Body.Bytes(), video[:8]) {
		t.Fatal("video range request failed")
	}
}

func TestBackgroundImportRejectsNonPublicDestinations(t *testing.T) {
	for _, raw := range []string{"file:///etc/passwd", "http://127.0.0.1/x", "http://[::1]/x", "http://169.254.169.254/x", "http://10.1.2.3/x", "https://user:pass@example.com/x", "https://example.com:8080/x", "https://[::ffff:127.0.0.1]/x"} {
		if _, err := validateBackgroundURL(raw); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	if _, err := validateBackgroundURL("https://example.com/image.gif?download=1"); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"100.64.0.1", "192.0.2.1", "198.18.0.1", "fe80::1", "fc00::1", "2001:db8::1"} {
		if backgroundPublicIP(net.ParseIP(raw)) {
			t.Errorf("accepted private/reserved IP %s", raw)
		}
	}
	if !backgroundPublicIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("public destination rejected")
	}
}

func TestBackgroundImportFromPublicServer(t *testing.T) {
	raw := os.Getenv("LINK_BOT_TEST_MEDIA_URL")
	if raw == "" {
		t.Skip("set LINK_BOT_TEST_MEDIA_URL to test a public media server")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	data, err := downloadBackground(ctx, raw)
	if err != nil {
		t.Fatalf("download public background: %v", err)
	}
	url, _, err := storeUploadedBackground(t.TempDir(), data)
	if err != nil || !strings.HasPrefix(url, "/mini-app/uploads/background-") {
		t.Fatalf("imported background: %q %v", url, err)
	}
}

func TestBackgroundRoutesRequireAppearancePermission(t *testing.T) {
	for _, route := range []string{"background/upload", "background/import"} {
		full := "/api/mini-app/admin/" + route
		if !adminRouteAllowed(adminAccess{IsAdmin: true, Permissions: []string{"appearance"}}, full) {
			t.Fatal("appearance role denied")
		}
		if adminRouteAllowed(adminAccess{IsAdmin: true, Permissions: []string{"content"}}, full) {
			t.Fatal("content role gained appearance access")
		}
	}
}
