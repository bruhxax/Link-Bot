package devicequota

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"link-bot/internal/remnawave"
)

var hwidPattern = regexp.MustCompile(`^[a-zA-Z0-9=-]{10,64}$`)

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "method not allowed", 405)
		return
	}
	token, err := uuid.Parse(strings.TrimPrefix(r.URL.Path, Route))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	b, err := s.store.FindToken(r.Context(), token)
	if err != nil {
		http.Error(w, "subscription temporarily unavailable", 503)
		return
	}
	if b == nil {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	preParent, err := s.panel.RawUser(ctx, b.ParentID, b.ParentUUID)
	if err != nil {
		http.Error(w, "subscription unavailable", 503)
		return
	}
	subID, p, err := s.plan(ctx, preParent)
	if err != nil {
		http.Error(w, "subscription temporarily unavailable", 503)
		return
	}
	var source string
	var cacheSlot int
	var useCached bool
	err = s.store.Locked(ctx, b.ID, func(b *Binding, save func() error) error {
		parent, e := s.panel.RawUser(ctx, b.ParentID, b.ParentUUID)
		if e != nil {
			return e
		}
		if subID == 0 || b.SubscriptionID == nil || subID != *b.SubscriptionID {
			return ErrUnavailable
		}
		hwid := strings.TrimSpace(r.Header.Get("X-Hwid"))
		if !isActive(parent) {
			return ErrUnavailable
		}
		if !enforced(p) {
			if _, _, _, e = s.synchronize(ctx, b, save, parent, p, subID); e != nil {
				return e
			}
			source = parent.SubscriptionURL
			return nil
		}
		if !hwidPattern.MatchString(hwid) {
			return errHWID
		}
		if isBlocked(b, hwid) {
			return ErrDeviceBlocked
		}
		_, total, _, e := s.synchronize(ctx, b, save, parent, p, subID)
		if e != nil {
			return e
		}
		if parent.TrafficLimitBytes > 0 && total >= parent.TrafficLimitBytes {
			return ErrUnavailable
		}
		slot := findSlot(b, hwid)
		if slot == nil {
			number := availableSlot(b, parent.HwidDeviceLimit)
			if number == 0 {
				return ErrDeviceLimit
			}
			for _, candidate := range b.Slots {
				if candidate.Number == number {
					slot = candidate
					break
				}
			}
			if slot == nil {
				slot = &Slot{Number: number}
				b.Slots = append(b.Slots, slot)
			}
			// Save assignment before creation. Deterministic usernames recover a child
			// created just before a crash without allocating a fresh traffic allowance.
			slot.Device = remnawave.UserDevice{Hwid: hwid, Platform: limitedHeader(r, "X-Device-Os"), OSVersion: limitedHeader(r, "X-Ver-Os"), DeviceModel: limitedHeader(r, "X-Device-Model"), UserAgent: limitedHeader(r, "User-Agent"), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
			slot.Registered = false
			slot.Paused = false
			slot.UserLimit = nil
			if e = save(); e != nil {
				return e
			}
		}
		if slot.ChildID <= 0 && slot.ChildUUID == uuid.Nil {
			// A random binding namespace also avoids adopting accounts left in a
			// panel after a bot database was replaced or restored independently.
			if slot.Username == "" {
				slot.Username = deviceUsername(parent, b, slot.Number)
				if e = save(); e != nil {
					return e
				}
			}
			child, e := s.panel.EnsureQuotaUser(ctx, slot.Username, map[string]any{
				"status": "DISABLED", "expireAt": parent.ExpireAt.UTC().Format(time.RFC3339), "trafficLimitBytes": quota(p, slot.Number), "trafficLimitStrategy": parent.TrafficLimitStrategy, "hwidDeviceLimit": 1, "activeInternalSquads": []string{},
				"description": deviceDescription(parent, slot),
			})
			if e != nil {
				return e
			}
			slot.ChildID = child.ID
			slot.ChildUUID = child.UUID
			if e = save(); e != nil {
				return e
			}
		}
		if _, _, _, e = s.synchronize(ctx, b, save, parent, p, subID); e != nil {
			return e
		}
		child, e := s.panel.RawUser(ctx, slot.ChildID, slot.ChildUUID)
		if e != nil {
			return e
		}
		if (!isActive(child) && !slot.Paused) || !slotAllowed(slot.Number, parent.HwidDeviceLimit) {
			return ErrUnavailable
		}
		source = child.SubscriptionURL
		cacheSlot = slot.Number
		useCached = slot.Paused
		return nil
	})
	if err != nil {
		code := http.StatusServiceUnavailable
		message := "subscription temporarily unavailable; retry shortly"
		if errors.Is(err, errHWID) {
			code = 400
			message = "Для отдельных лимитов нужен клиент, передающий HWID: импортируйте ссылку в Happ или v2RayTun."
		}
		if errors.Is(err, ErrDeviceLimit) {
			code = 403
			message = "Device limit reached. Remove an old device in your cabinet."
		}
		if errors.Is(err, ErrDeviceBlocked) {
			code = 403
			message = ErrDeviceBlocked.Error()
		}
		if errors.Is(err, ErrUnavailable) {
			code = 403
			message = "Subscription expired, disabled, or traffic limit reached."
		}
		http.Error(w, message, code)
		return
	}
	if cacheSlot > 0 {
		var cfg *Configuration
		if useCached {
			cfg, err = s.store.LoadConfig(ctx, b.ID, cacheSlot, configurationFormat(r), strings.TrimSpace(r.Header.Get("X-Hwid")), sourceHash(source))
		} else {
			cfg, err = s.fetch(ctx, r, source)
			if err == nil {
				_ = s.store.SaveConfig(ctx, b.ID, cacheSlot, configurationFormat(r), strings.TrimSpace(r.Header.Get("X-Hwid")), sourceHash(source), cfg)
			}
		}
		if err != nil || cfg == nil {
			http.Error(w, "profile temporarily unavailable; it will resume after enabling the device", 503)
			return
		}
		s.writeConfiguration(w, cfg)
		return
	}
	if err = s.proxy(ctx, w, r, source); err != nil {
		http.Error(w, "configuration temporarily unavailable", 503)
	}
}

var errHWID = errors.New("a valid HWID header is required")

func limitedHeader(r *http.Request, key string) string {
	v := strings.TrimSpace(r.Header.Get(key))
	if len(v) > 256 {
		v = v[:256]
	}
	return v
}
func findSlot(b *Binding, hwid string) *Slot {
	for _, s := range b.Slots {
		if s.Device.Hwid == hwid {
			return s
		}
	}
	return nil
}
func availableSlot(b *Binding, limit *int) int {
	upper := len(b.Slots) + 1
	if limit != nil && *limit > 0 && *limit < upper {
		upper = *limit
	}
	for n := 1; n <= upper; n++ {
		occupied := false
		for _, slot := range b.Slots {
			if slot.Number == n && (slot.Device.Hwid != "" || slot.RevokeHWID != "") {
				occupied = true
				break
			}
		}
		if !occupied {
			return n
		}
	}
	return 0
}
func (s *Service) proxy(ctx context.Context, w http.ResponseWriter, r *http.Request, source string) error {
	cfg, err := s.fetch(ctx, r, source)
	if err != nil {
		return err
	}
	s.writeConfiguration(w, cfg)
	return nil
}

func (s *Service) fetch(ctx context.Context, r *http.Request, source string) (*Configuration, error) {
	u, err := url.Parse(source)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, ErrUnavailable
	}
	if strings.HasSuffix(u.Path, Route) || strings.Contains(u.Path, Route) {
		return nil, ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"User-Agent", "Accept", "X-Hwid", "X-Device-Os", "X-Ver-Os", "X-Device-Model"} {
		if value := r.Header.Get(key); value != "" {
			req.Header.Set(key, value)
		}
	}
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(next *http.Request, via []*http.Request) error {
		if len(via) >= 3 || next.URL.Scheme != u.Scheme || next.URL.Host != u.Host {
			return errors.New("untrusted subscription redirect")
		}
		return nil
	}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("subscription endpoint returned %d", resp.StatusCode)
	}
	// Buffer before writing headers so upstream failures never yield a partial
	// configuration followed by an error message. Do not forward auth/cookies.
	const maxConfig = 8 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxConfig+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxConfig {
		return nil, errors.New("subscription configuration too large")
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, errors.New("subscription configuration is empty")
	}
	cfg := &Configuration{Body: body, Headers: map[string]string{}}
	for _, key := range []string{"Content-Type", "Content-Disposition", "Subscription-Userinfo", "Profile-Title", "Profile-Update-Interval", "Support-Url", "Announce"} {
		if value := resp.Header.Get(key); value != "" {
			cfg.Headers[key] = value
		}
	}
	return cfg, nil
}
