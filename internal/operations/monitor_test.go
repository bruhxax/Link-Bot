package operations

import (
	"errors"
	"testing"
)

func TestRecordHealthResultReportsOncePerContinuousOutage(t *testing.T) {
	failures := map[string]int{}
	probeErr := errors.New("panel unavailable")

	for attempt := 1; attempt <= 6; attempt++ {
		got := recordHealthResult(failures, "remnawave", probeErr)
		want := attempt == healthFailureThreshold
		if got != want {
			t.Fatalf("attempt %d: report = %v, want %v", attempt, got, want)
		}
	}

	if recordHealthResult(failures, "remnawave", nil) {
		t.Fatal("successful probe must not report")
	}
	for attempt := 1; attempt <= healthFailureThreshold; attempt++ {
		got := recordHealthResult(failures, "remnawave", probeErr)
		want := attempt == healthFailureThreshold
		if got != want {
			t.Fatalf("new outage attempt %d: report = %v, want %v", attempt, got, want)
		}
	}
}
