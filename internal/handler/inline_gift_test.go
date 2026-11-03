package handler

import "testing"

func TestInlineGiftPendingTextEscapesNames(t *testing.T) {
	got := inlineGiftPendingText("<b>{plan}</b> от {sender}", "Месяц <VIP>", "@alice&bob")
	if want := "<b>Месяц &lt;VIP&gt;</b> от @alice&amp;bob"; got != want {
		t.Fatalf("pending gift text = %q, want %q", got, want)
	}
}
