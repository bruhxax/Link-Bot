package miniapp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v4"
	"link-bot/internal/database"
)

// realtimeHub carries opaque invalidations. The client always fetches its own
// authorized data after a notification; no customer data is broadcast here.
type realtimeHub struct {
	mu      sync.Mutex
	clients map[chan struct{}]struct{}
}

func newRealtimeHub() *realtimeHub {
	return &realtimeHub{clients: make(map[chan struct{}]struct{})}
}

func (hub *realtimeHub) subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	hub.mu.Lock()
	hub.clients[ch] = struct{}{}
	hub.mu.Unlock()
	return ch
}

func (hub *realtimeHub) unsubscribe(ch chan struct{}) {
	hub.mu.Lock()
	delete(hub.clients, ch)
	hub.mu.Unlock()
}

func (hub *realtimeHub) publish() {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	for ch := range hub.clients {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// StartRealtime uses a dedicated connection so LISTEN never consumes a pool
// connection needed by payment, bot, and Mini App requests.
func (h *Handler) StartRealtime(ctx context.Context, databaseURL string) {
	if h == nil || h.realtime == nil {
		return
	}
	go func() {
		for ctx.Err() == nil {
			if err := h.listenRealtime(ctx, databaseURL); err != nil && ctx.Err() == nil {
				slog.Warn("mini app realtime listener disconnected", "error", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
	}()
}

func (h *Handler) listenRealtime(ctx context.Context, databaseURL string) error {
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())
	if _, err := conn.Exec(ctx, "LISTEN miniapp_changes"); err != nil {
		return err
	}
	// Reconcile clients after a reconnect; notifications sent during the gap
	// cannot be replayed by PostgreSQL.
	h.realtime.publish()
	for ctx.Err() == nil {
		if _, err := conn.WaitForNotification(ctx); err != nil {
			return err
		}
		h.realtime.publish()
	}
	return ctx.Err()
}

func (h *Handler) handleRealtime(w http.ResponseWriter, r *http.Request, _ *session, _ *database.Customer) {
	flusher, ok := w.(http.Flusher)
	if !ok || h.realtime == nil {
		h.writeError(w, http.StatusServiceUnavailable, "realtime_unavailable", "Realtime unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Connection", "keep-alive")
	ch := h.realtime.subscribe()
	defer h.realtime.unsubscribe(ch)
	if _, err := fmt.Fprint(w, "event: ready\ndata: {}\n\n"); err != nil {
		return
	}
	flusher.Flush()
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	maxAge := time.NewTimer(20 * time.Minute)
	defer maxAge.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-maxAge.C:
			return
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
		case <-ch:
			if _, err := fmt.Fprint(w, "event: change\ndata: {}\n\n"); err != nil {
				return
			}
		}
		flusher.Flush()
	}
}
