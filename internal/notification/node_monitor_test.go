package notification

import (
	"testing"

	"link-bot/internal/remnawave"
)

func TestHostOnlyOnInactiveNodes(t *testing.T) {
	const down = "node-down"
	const inbound = "inbound-estonia"
	const profile = "profile-main"
	downNode := remnawave.NodeStatus{UUID: down, Inbounds: []remnawave.NodeInbound{{UUID: inbound, ProfileUUID: profile}}}
	for _, test := range []struct {
		name  string
		host  remnawave.HostStatus
		nodes []remnawave.NodeStatus
		want  bool
	}{
		{"own host", remnawave.HostStatus{Nodes: []string{down}}, []remnawave.NodeStatus{downNode}, true},
		{"profile inbound", remnawave.HostStatus{InboundUUID: inbound, ProfileUUID: profile}, []remnawave.NodeStatus{downNode}, true},
		{"different inbound", remnawave.HostStatus{InboundUUID: "other", ProfileUUID: profile}, []remnawave.NodeStatus{downNode}, false},
		{"different profile", remnawave.HostStatus{InboundUUID: inbound, ProfileUUID: "other"}, []remnawave.NodeStatus{downNode}, false},
		{"shared with healthy node", remnawave.HostStatus{Nodes: []string{down, "node-up"}}, []remnawave.NodeStatus{downNode, {UUID: "node-up", IsOnline: true}}, false},
		{"shared with unknown node", remnawave.HostStatus{Nodes: []string{down, "node-unknown"}}, []remnawave.NodeStatus{downNode}, false},
		{"unrelated host", remnawave.HostStatus{Nodes: []string{"other"}}, []remnawave.NodeStatus{downNode}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := hostOnlyOnInactiveNodes(test.host, down, test.nodes); got != test.want {
				t.Fatalf("got %v, want %v", got, test.want)
			}
		})
	}
}
