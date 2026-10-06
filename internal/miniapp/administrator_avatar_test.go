package miniapp

import (
	"encoding/json"
	"testing"

	"link-bot/internal/database"
)

func TestAdministratorListIncludesAvatarAndPreservesRole(t *testing.T) {
	items := administratorListItems([]database.Administrator{
		{CustomerID: 1, TelegramID: 10, Username: " @Example_User ", Role: "Главный администратор", Color: "#69a8d4", IsOwner: true, Permissions: []string{}},
		{CustomerID: 2, TelegramID: 20, Permissions: []string{"status"}},
	})
	raw, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	var decoded []map[string]any
	if err = json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded[0]["avatarUrl"] != "https://t.me/i/userpic/320/example_user.jpg" || decoded[0]["isOwner"] != true || decoded[0]["role"] != "Главный администратор" || decoded[0]["customerId"] != float64(1) {
		t.Fatalf("incomplete administrator payload: %s", raw)
	}
	if _, exists := decoded[1]["avatarUrl"]; exists {
		t.Fatal("username-less account has a fake avatar URL")
	}
	if empty, _ := json.Marshal(administratorListItems(nil)); string(empty) != "[]" {
		t.Fatalf("empty list = %s", empty)
	}
}
