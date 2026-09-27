package miniapp

import (
	"testing"

	"link-bot/internal/remnawave"
)

func TestFilterServerNodesHidesNodesOnlyFromPublicPayload(t *testing.T) {
	nodes := []remnawave.NodeStatus{
		{UUID: "ABC-123", Name: "Germany", Address: "de.example", IsOnline: true},
		{UUID: "DEF-456", Name: "Sweden", Address: "se.example", IsOnline: false},
	}
	hidden := []string{"uuid:abc-123"}

	public := filterServerNodes(nodes, hidden, false)
	if len(public.Items) != 1 || public.Items[0].Name != "Sweden" || public.Items[0].ID != "" || public.Items[0].Hidden {
		t.Fatalf("public nodes = %+v, want only visible node without admin fields", public.Items)
	}
	admin := filterServerNodes(nodes, hidden, true)
	if len(admin.Items) != 2 || admin.Items[0].ID != "uuid:abc-123" || !admin.Items[0].Hidden || admin.Items[1].Hidden {
		t.Fatalf("admin nodes = %+v, want both nodes and hidden state", admin.Items)
	}
}

func TestServerNodeIDFallsBackWithoutUUID(t *testing.T) {
	first := remnawave.NodeStatus{Name: "Germany", Address: "DE.EXAMPLE"}
	second := remnawave.NodeStatus{Name: "Sweden", Address: "DE.EXAMPLE"}
	if serverNodeID(first) == serverNodeID(second) {
		t.Fatal("nodes sharing an address must have distinct visibility IDs")
	}
}
