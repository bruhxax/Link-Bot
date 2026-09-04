package miniapp

import (
	"context"
	"testing"
	"time"

	"link-bot/internal/adminnotify"
	"link-bot/internal/database"
)

type recordingSupportPushNotifier struct {
	events []adminnotify.Event
}

func (n *recordingSupportPushNotifier) Notify(_ context.Context, event adminnotify.Event) error {
	n.events = append(n.events, event)
	return nil
}

func TestSupportTicketAndReplySendAdminPush(t *testing.T) {
	notifier := &recordingSupportPushNotifier{}
	handler := &Handler{webPushNotifier: notifier}
	ticket := &database.SupportTicket{
		ID:               73,
		Subject:          "Не подключается VPN",
		CustomerName:     "customer_73",
		CustomerUsername: "bruh_user",
	}

	handler.notifyAdminAboutSupportTicket(context.Background(), ticket, "Первое сообщение")
	handler.notifyAdminAboutSupportReply(context.Background(), ticket, "Ответ пользователя")

	if len(notifier.events) != 2 {
		t.Fatalf("push event count = %d, want 2", len(notifier.events))
	}
	if got := notifier.events[0]; got.Title != "Новое обращение" || got.Body != "@bruh_user · Не подключается VPN" || got.Tag != "support-ticket-73" || got.URL != "/mini-app/?page=support" {
		t.Fatalf("new ticket push = %+v", got)
	}
	if got := notifier.events[1]; got.Title != "Ответ в обращении" || got.Tag != "support-reply-73" {
		t.Fatalf("reply push = %+v", got)
	}
}

func TestSupportNotificationWindowCoversPushDelivery(t *testing.T) {
	if supportNotificationTimeout < 10*time.Second {
		t.Fatalf("support notification timeout = %s, want at least 10s", supportNotificationTimeout)
	}
}

func TestSupportAsyncDispatchesAutomaticAdminPush(t *testing.T) {
	delivered := make(chan adminnotify.Event, 1)
	handler := &Handler{webPushNotifier: adminnotifyFunc(func(_ context.Context, event adminnotify.Event) error {
		delivered <- event
		return nil
	})}
	ticket := &database.SupportTicket{ID: 74, Subject: "Не работает VPN", CustomerUsername: "client"}

	handler.notifySupportAsync(func(ctx context.Context) {
		handler.notifyAdminAboutSupportTicket(ctx, ticket, "Помогите")
	})

	select {
	case event := <-delivered:
		if event.Title != "Новое обращение" || event.Tag != "support-ticket-74" {
			t.Fatalf("automatic support push = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("automatic support push was not dispatched")
	}
}

type adminnotifyFunc func(context.Context, adminnotify.Event) error

func (fn adminnotifyFunc) Notify(ctx context.Context, event adminnotify.Event) error {
	return fn(ctx, event)
}
