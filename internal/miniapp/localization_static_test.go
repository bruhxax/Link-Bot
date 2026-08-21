package miniapp

import (
	"strings"
	"testing"
)

func TestMiniAppContainsGlobalPersianLocalizationAndVazir(t *testing.T) {
	appRaw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read embedded app.js: %v", err)
	}
	stylesRaw, err := embeddedStatic.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read embedded styles.css: %v", err)
	}
	appJS := string(appRaw)
	styles := string(stylesRaw)
	for _, required := range []string{
		"copybook.fa =",
		`document.documentElement.dir = state.locale === "fa" ? "rtl" : "ltr"`,
		`data-setting-path="localization.language"`,
		`data-setting-path="localization.fontFamily"`,
		`localizedText("Язык и шрифт", "Language and font", "زبان و فونت")`,
	} {
		if !strings.Contains(appJS, required) {
			t.Fatalf("app.js does not contain %q", required)
		}
	}
	for _, required := range []string{
		`font-family: "Vazir"`,
		"persian-computing/vazir-font@44b82b3c3fecf487514ff73d9272bceb9bda0d74",
		`html[dir="rtl"] body`,
		`html[data-font="vazir"] body`,
	} {
		if !strings.Contains(styles, required) {
			t.Fatalf("styles.css does not contain %q", required)
		}
	}
}
