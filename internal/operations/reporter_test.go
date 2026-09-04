package operations

import (
	"context"
	"testing"
	"time"

	"link-bot/internal/adminnotify"
	"link-bot/internal/database"
)

type recordingAdminNotifier struct {
	event adminnotify.Event
}

func (n *recordingAdminNotifier) Notify(_ context.Context, event adminnotify.Event) error {
	n.event = event
	return nil
}

func TestShouldAlertRespectsCooldownForCriticalEvents(t *testing.T) {
	reporter := &Reporter{lastAlert: map[string]time.Time{}}
	const fingerprint = "healthcheck"

	if !reporter.shouldAlert(fingerprint, 1, "critical") {
		t.Fatal("first event must trigger an alert")
	}
	if reporter.shouldAlert(fingerprint, 2, "critical") {
		t.Fatal("repeated critical event must respect the cooldown")
	}

	reporter.lastAlert[fingerprint] = time.Now().Add(-alertCooldown)
	if !reporter.shouldAlert(fingerprint, 3, "critical") {
		t.Fatal("event must trigger after the cooldown")
	}
}

func TestSendAdminAlertUsesWebPushWithoutTelegramBot(t *testing.T) {
	notifier := &recordingAdminNotifier{}
	reporter := &Reporter{push: notifier}
	reporter.sendAdminAlert(context.Background(), &database.OperationalEvent{
		Fingerprint: "healthcheck",
		Severity:    "critical",
		Category:    "database",
		Operation:   "ping",
		Message:     "connection failed",
	})

	if notifier.event.Title != "Критическая ошибка" {
		t.Fatalf("push title = %q", notifier.event.Title)
	}
	if notifier.event.URL != "/mini-app/?page=admin&section=diagnostics" {
		t.Fatalf("push URL = %q", notifier.event.URL)
	}
}

func TestReportDispatchesAutomaticDiagnosticPush(t *testing.T) {
	notifier := &recordingAdminNotifier{}
	reporter := &Reporter{push: notifier, lastAlert: map[string]time.Time{}}
	reporter.Report(context.Background(), ReportInput{
		Category:  "database",
		Severity:  "critical",
		Operation: "ping",
		Message:   "connection failed",
	})

	if notifier.event.Title != "Критическая ошибка" || notifier.event.URL != "/mini-app/?page=admin&section=diagnostics" {
		t.Fatalf("automatic diagnostic push = %+v", notifier.event)
	}
}

func TestDiagnosticPushDeliveryWindowCoversNetworkRequest(t *testing.T) {
	if adminDiagnosticPushTimeout < 15*time.Second {
		t.Fatalf("diagnostic push timeout = %s, want at least 15s", adminDiagnosticPushTimeout)
	}
}
