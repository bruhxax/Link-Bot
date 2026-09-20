package miniapp

import (
	"strings"
	"testing"
)

func TestSelectionFeedbackStaticContract(t *testing.T) {
	appRaw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	stylesRaw, err := embeddedStatic.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}

	appJS := string(appRaw)
	for _, fragment := range []string{
		`function queueSelectionFeedback(action, value)`,
		`target.hasAttribute("data-selection-feedback")`,
		`data-action="select-plan"`,
		`data-action="select-device-pack"`,
		`data-action="select-gift-plan"`,
		`data-action="select-pay-method"`,
		`data-selection-feedback aria-pressed="${selected}"`,
	} {
		if !strings.Contains(appJS, fragment) {
			t.Fatalf("app.js does not contain %q", fragment)
		}
	}

	styles := string(stylesRaw)
	for _, fragment := range []string{
		`[data-selection-feedback].is-selection-feedback`,
		`@keyframes selectionFeedback`,
		`animation: selectionFeedback 320ms`,
		`@media (prefers-reduced-motion: reduce)`,
		`animation: none !important`,
	} {
		if !strings.Contains(styles, fragment) {
			t.Fatalf("styles.css does not contain %q", fragment)
		}
	}
}
