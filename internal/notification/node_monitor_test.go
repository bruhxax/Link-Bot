package notification

import (
	"testing"

	"link-bot/internal/remnawave"
)

func TestHostOnlyOnInactiveNodes(t *testing.T) {
	const down = "node-down"
	for _, test := range []struct {
		name   string
		host   remnawave.HostStatus
		active map[string]bool
		want   bool
	}{
		{"own host", remnawave.HostStatus{Nodes: []string{down}}, map[string]bool{down: false}, true},
		{"shared with healthy node", remnawave.HostStatus{Nodes: []string{down, "node-up"}}, map[string]bool{down: false, "node-up": true}, false},
		{"shared with unknown node", remnawave.HostStatus{Nodes: []string{down, "node-unknown"}}, map[string]bool{down: false}, false},
		{"unrelated host", remnawave.HostStatus{Nodes: []string{"other"}}, map[string]bool{down: false}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := hostOnlyOnInactiveNodes(test.host, down, test.active); got != test.want {
				t.Fatalf("got %v, want %v", got, test.want)
			}
		})
	}
}
