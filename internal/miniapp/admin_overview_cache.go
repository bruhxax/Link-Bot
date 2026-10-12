package miniapp

import (
	"context"
	"sync"
	"time"

	"link-bot/internal/database"
)

type overviewRevenueCache struct {
	mu      sync.Mutex
	data    *database.AdminOverview
	at      time.Time
	err     error
	pending chan struct{}
}

func (h *Handler) cachedRevenue(ctx context.Context, force bool) (*database.AdminOverview, bool, error) {
	c := &h.overviewRevenue
	c.mu.Lock()
	if c.pending == nil && (force || time.Since(c.at) > time.Minute) {
		c.pending = make(chan struct{})
		go func() {
			taskCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			data, err := h.customerRepository.LoadAdminOverview(taskCtx, 90, time.Now(), false, true)
			c.mu.Lock()
			defer c.mu.Unlock()
			if err == nil {
				c.data = data
			}
			c.err = err
			c.at = time.Now()
			close(c.pending)
			c.pending = nil
		}()
	}
	pending := c.pending
	cold := c.data == nil
	c.mu.Unlock()
	if cold && pending != nil {
		select {
		case <-pending:
		case <-ctx.Done():
		case <-time.After(100 * time.Millisecond):
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.data, c.pending != nil, c.err
}

func (h *Handler) requestPanelCatalog(ctx context.Context, force bool, revision string) (panelCatalogSnapshot, bool, error) {
	s, pending, err := h.panelCatalog(force, revision)
	if pending && (s.Users == nil || force) {
		h.overviewPanel.mu.Lock()
		done := h.overviewPanel.pending
		h.overviewPanel.mu.Unlock()
		if done != nil {
			select {
			case <-done:
			case <-ctx.Done():
			case <-time.After(100 * time.Millisecond):
			}
		}
		return h.panelCatalog(false, revision)
	}
	return s, pending, err
}
