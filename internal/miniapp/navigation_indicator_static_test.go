package miniapp

import (
	"strings"
	"testing"
)

func TestBottomNavigationIndicatorIgnoresDockEntranceTransform(t *testing.T) {
	appRaw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	app := string(appRaw)
	for _, fragment := range []string{
		`previousItem.offsetLeft`,
		`previousItem.offsetWidth`,
		`activeItem.offsetLeft`,
		`activeItem.offsetWidth`,
	} {
		if !strings.Contains(app, fragment) {
			t.Fatalf("stable navigation indicator geometry is missing %q", fragment)
		}
	}
	if strings.Contains(app, `previousRect.left - navRect.left`) || strings.Contains(app, `activeRect.left - navRect.left`) {
		t.Fatal("navigation indicator still measures transformed viewport rectangles")
	}
}
