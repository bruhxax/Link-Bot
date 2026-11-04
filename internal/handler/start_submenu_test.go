package handler

import (
	"strings"
	"testing"
)

func TestRenderStartSubmenuTextEscapesValuesAndKeepsFormatting(t *testing.T) {
	got := renderStartSubmenuText("<b>{subscription_name}</b>\n{traffic} · {devices}\n{plan_name}", map[string]string{
		"subscription_name": "A & B <premium>",
		"traffic":           "50/100 ГБ",
		"devices":           "5/10",
	})
	if !strings.Contains(got, "<b>A &amp; B &lt;premium&gt;</b>") || !strings.Contains(got, "50/100 ГБ · 5/10") || !strings.Contains(got, "\n—") {
		t.Fatalf("unexpected rendered submenu: %q", got)
	}
}
