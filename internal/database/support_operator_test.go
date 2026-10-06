package database

import "testing"

func TestOperatorAssignmentRules(t *testing.T) {
	for _, tc := range []struct {
		name, status         string
		assigned, actor      int64
		release, owner, want bool
	}{
		{"claim free", "open", 0, 99, false, false, true},
		{"claim idempotent", "open", 99, 99, false, false, true},
		{"cannot steal", "open", 99, 100, false, false, false},
		{"release own", "open", 99, 99, true, false, true},
		{"cannot release another", "open", 99, 100, true, false, false},
		{"owner can return to queue", "open", 99, 100, true, true, true},
		{"closed", "closed", 0, 99, false, true, false},
		{"invalid actor", "open", 0, 0, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := supportOperatorAllowed(tc.status, tc.assigned, tc.actor, tc.release, tc.owner); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
