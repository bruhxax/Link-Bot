package miniapp

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPaymentLogoUploadRequiresPlansPermission(t *testing.T) {
	route := "/api/mini-app/admin/payments/logo/upload"
	for _, tc := range []struct {
		name   string
		access adminAccess
		want   int
	}{
		{"user", adminAccess{}, 403},
		{"content editor", adminAccess{IsAdmin: true, Permissions: []string{"content"}}, 403},
		{"plans editor", adminAccess{IsAdmin: true, Permissions: []string{"plans"}}, 200},
		{"owner", ownerAccess(), 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			allowed := adminRouteAllowed(tc.access, route)
			if allowed != (tc.want == 200) {
				t.Fatal("incorrect route policy")
			}
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			part, err := writer.CreateFormFile("logo", "my-card.png")
			if err != nil {
				t.Fatal(err)
			}
			data := testLogoPNG(t, 48, 48)
			part.Write(data)
			writer.Close()
			r := httptest.NewRequest(http.MethodPost, route, &body)
			r.Header.Set("Content-Type", writer.FormDataContentType())
			w := httptest.NewRecorder()
			h := &Handler{logoUploadDir: t.TempDir()}
			h.handleAdminPaymentLogoUpload(w, r, &session{AdminAccess: tc.access}, nil)
			if w.Code != tc.want {
				t.Fatalf("%d: %s", w.Code, w.Body.String())
			}
			if tc.want != 200 {
				return
			}
			var response struct {
				Data struct {
					URL string `json:"url"`
				} `json:"data"`
			}
			if err = json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(response.Data.URL, "/mini-app/uploads/logo-") {
				t.Fatal(response)
			}
			out := httptest.NewRecorder()
			h.serveUploadedLogo(out, httptest.NewRequest("GET", response.Data.URL, nil))
			if out.Code != 200 || out.Header().Get("Content-Type") != "image/png" || !bytes.Equal(data, out.Body.Bytes()) {
				t.Fatal("uploaded logo not served intact")
			}
		})
	}
}

func TestPaymentLogoUploadRejectsNonImageFile(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("logo", "card.png")
	part.Write([]byte("<script>alert(1)</script>"))
	writer.Close()
	r := httptest.NewRequest("POST", "/api/mini-app/admin/payments/logo/upload", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	h := &Handler{logoUploadDir: t.TempDir()}
	h.handleAdminPaymentLogoUpload(w, r, &session{AdminAccess: ownerAccess()}, nil)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatal(w.Code, w.Body.String())
	}
}
