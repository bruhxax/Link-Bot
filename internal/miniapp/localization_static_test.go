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

func TestMiniAppIncludesLanguageSwitchAndBundledFonts(t *testing.T) {
	appRaw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	stylesRaw, err := embeddedStatic.ReadFile("static/styles.css")
	if err != nil {
		t.Fatal(err)
	}
	appJS, styles := string(appRaw), string(stylesRaw)
	for _, marker := range []string{`data-action="profile-language"`, `STORAGE_KEYS.languageOverride`, `localization.fontFamilyRu`, `localization.fontFamilyEn`} {
		if !strings.Contains(appJS, marker) {
			t.Errorf("missing language/font control %q", marker)
		}
	}
	for _, font := range []struct {
		name, scripts string
	}{
		{"golostext", "cyrillic,latin"}, {"rubik", "cyrillic,latin"},
		{"manrope", "cyrillic,latin"}, {"onest", "cyrillic,latin"},
		{"inter", "latin"}, {"dmsans", "latin"}, {"plusjakartasans", "latin"},
		{"outfit", "latin"}, {"spacegrotesk", "latin"},
	} {
		for _, script := range strings.Split(font.scripts, ",") {
			path := "static/assets/fonts/" + font.name + "/" + script + ".woff2"
			content, err := embeddedStatic.ReadFile(path)
			if err != nil {
				t.Errorf("missing bundled font %s: %v", path, err)
				continue
			}
			if len(content) < 1000 || string(content[:4]) != "wOF2" || !strings.Contains(styles, "/mini-app/assets/fonts/"+font.name+"/"+script+".woff2") {
				t.Errorf("invalid or unused bundled font %s", path)
			}
		}
		if _, err := embeddedStatic.ReadFile("static/assets/fonts/" + font.name + "/OFL.txt"); err != nil {
			t.Errorf("missing font license for %s: %v", font.name, err)
		}
	}
}
