package miniapp

import (
	"os"
	"strings"
	"testing"
)

func TestMoyNalogAdminStaticSurface(t *testing.T) {
	appRaw, err := os.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	stylesRaw, err := os.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}
	app := string(appRaw)
	styles := string(stylesRaw)
	for _, fragment := range []string{
		`"Мой налог"`,
		`renderAdminMoyNalogPage`,
		`/api/mini-app/admin/moynalog/state`,
		`/api/mini-app/admin/moynalog/test`,
		`/api/mini-app/admin/moynalog/retry`,
		`data-moynalog-method`,
	} {
		if !strings.Contains(app, fragment) {
			t.Fatalf("app.js does not contain %q", fragment)
		}
	}
	for _, fragment := range []string{".admin-moynalog__metrics", ".admin-moynalog__methods", ".admin-moynalog-receipt"} {
		if !strings.Contains(styles, fragment) {
			t.Fatalf("styles.css does not contain %q", fragment)
		}
	}
}
