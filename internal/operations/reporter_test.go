package operations

import (
	"testing"
	"time"
)

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
