package devicequota

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"link-bot/internal/remnawave"
)

func TestDevicePanelLabelsIdentifyOwnerAndDevice(t *testing.T) {
	b := &Binding{Token: uuid.New()}
	parent := &remnawave.PanelUser{Username: "bruh"}
	name := deviceUsername(parent, b, 2)
	if !strings.HasPrefix(name, "bruh_device2_") || len(name) > 36 || strings.Contains(name, strings.ReplaceAll(b.Token.String(), "-", "")[:12]) {
		t.Fatalf("unreadable or secret-derived name: %s", name)
	}
	if deviceUsername(parent, b, 2) != name || deviceUsername(parent, b, 1) == name {
		t.Fatal("device identity not deterministic and distinct")
	}
	parent.Username = strings.Repeat("longusername", 8)
	if len(deviceUsername(parent, b, 1000)) > 36 {
		t.Fatal("panel username length exceeded")
	}
	parent.Username = "bruh"
	description := deviceDescription(parent, &Slot{Number: 2, Device: remnawave.UserDevice{DeviceModel: "iPhone 16", Platform: "iOS"}})
	if description != "Устройство №2 · iPhone 16 · Подписка: bruh" {
		t.Fatalf("bad description: %s", description)
	}
}
