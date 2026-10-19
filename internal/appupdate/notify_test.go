package appupdate

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type memoryNotifications struct {
	revision  string
	delivered bool
	claims    int
}

func (s *memoryNotifications) Claim(_ context.Context, revision string) (bool, error) {
	s.claims++
	if s.revision == revision && s.delivered {
		return false, nil
	}
	s.revision = revision
	return true, nil
}
func (s *memoryNotifications) Finish(_ context.Context, _ string, delivered bool) error {
	s.delivered = delivered
	return nil
}

func TestNotificationDeliveryDeduplicatesAndRetries(t *testing.T) {
	ctx := context.Background()
	store := &memoryNotifications{}
	sent := 0
	status := Status{State: "available", Kind: "release", LatestVersion: "2.7.4", LatestCommit: strings.Repeat("a", 40), AheadBy: 2, Summary: "Fix <test> & emoji ✨", URL: ReleasePage}
	send := func(_ context.Context, text, url string) error {
		sent++
		if !strings.Contains(text, "&lt;test&gt; &amp;") || url != ReleasePage {
			t.Fatal(text, url)
		}
		return nil
	}
	if err := NotifyOnce(ctx, status, "2.7.3", "1234567", store, send); err != nil {
		t.Fatal(err)
	}
	NotifyOnce(ctx, status, "2.7.3", "1234567", store, send)
	if sent != 1 {
		t.Fatal("duplicate notification", sent)
	}
	status.LatestCommit = strings.Repeat("b", 40)
	err := NotifyOnce(ctx, status, "2.7.3", "1234567", store, func(context.Context, string, string) error { return errors.New("telegram timeout") })
	if err == nil || store.delivered {
		t.Fatal("failed send was acknowledged")
	}
	NotifyOnce(ctx, status, "2.7.3", "1234567", store, send)
	if sent != 2 || !store.delivered {
		t.Fatal("failed send not retried")
	}
	status.State = "current"
	NotifyOnce(ctx, status, "2.7.4", "1234567", store, send)
	status.State = "unknown"
	NotifyOnce(ctx, status, "2.7.4", "1234567", store, send)
	if sent != 2 {
		t.Fatal("sent without an update")
	}
}
