package devicequota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"link-bot/internal/plans"
	"link-bot/internal/remnawave"
	"link-bot/internal/runtimeconfig"
	"link-bot/internal/trafficpolicy"
)

const gb = int64(1024 * 1024 * 1024)

type memoryStore struct {
	b        *Binding
	mu       sync.Mutex
	plan     string
	failSave func(*Binding) bool
	discover bool
	terms    *trafficpolicy.Terms
	configs  map[string]*Configuration
}

func (m *memoryStore) FindParent(_ context.Context, key string) (*Binding, error) {
	if m.b != nil && m.b.ParentKey == key {
		return m.b, nil
	}
	return nil, nil
}
func (m *memoryStore) FindToken(_ context.Context, token uuid.UUID) (*Binding, error) {
	if m.b != nil && m.b.Token == token {
		return m.b, nil
	}
	return nil, nil
}
func (m *memoryStore) Create(_ context.Context, b *Binding) error { b.ID = 7; m.b = b; return nil }
func (m *memoryStore) Locked(_ context.Context, _ int64, fn func(*Binding, func() error) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	clone := func(b *Binding) *Binding {
		raw, _ := json.Marshal(b)
		var copy Binding
		_ = json.Unmarshal(raw, &copy)
		return &copy
	}
	b := clone(m.b)
	return fn(b, func() error {
		if m.failSave != nil && m.failSave(b) {
			return errors.New("checkpoint interrupted")
		}
		m.b = clone(b)
		return nil
	})
}
func (m *memoryStore) List(context.Context) ([]*Binding, error) {
	if m.b == nil {
		return nil, nil
	}
	return []*Binding{m.b}, nil
}
func (m *memoryStore) Plan(context.Context, *remnawave.PanelUser) (int64, string, *trafficpolicy.Terms, error) {
	return 5, m.plan, m.terms, nil
}
func (m *memoryStore) Candidates(context.Context, []string) ([]remnawave.PanelUser, error) {
	if m.discover && m.b == nil {
		return []remnawave.PanelUser{{ID: 1}}, nil
	}
	return nil, nil
}

type fakeSettings struct{ value runtimeconfig.Settings }

func (m *memoryStore) LoadConfig(_ context.Context, id int64, n int, format, hwid, source string) (*Configuration, error) {
	return m.configs[fmt.Sprintf("%d/%d/%s/%s/%s", id, n, format, hwid, source)], nil
}
func (m *memoryStore) SaveConfig(_ context.Context, id int64, n int, format, hwid, source string, cfg *Configuration) error {
	if m.configs == nil {
		m.configs = map[string]*Configuration{}
	}
	m.configs[fmt.Sprintf("%d/%d/%s/%s/%s", id, n, format, hwid, source)] = cfg
	return nil
}

func (f *fakeSettings) Snapshot() runtimeconfig.Settings { return f.value }

type fakePanel struct {
	users       map[int64]*remnawave.PanelUser
	registered  map[int64]string
	revocations int
	fetchError  int64
	source      string
	revokeError bool
	drops       int
}

func (f *fakePanel) DropQuotaConnections(context.Context, int64) error { f.drops++; return nil }

func (f *fakePanel) RawUser(_ context.Context, id int64, _ uuid.UUID) (*remnawave.PanelUser, error) {
	if id == f.fetchError {
		return nil, errors.New("telemetry unavailable")
	}
	u := f.users[id]
	if u == nil {
		return nil, remnawave.ErrAdminSubscriptionNotFound
	}
	copy := *u
	return &copy, nil
}
func (f *fakePanel) RawQuotaUserByTelegramID(ctx context.Context, id int64) (*remnawave.PanelUser, error) {
	for _, u := range f.users {
		if u.TelegramID != nil && *u.TelegramID == id {
			return f.RawUser(ctx, u.ID, u.UUID)
		}
	}
	return nil, remnawave.ErrAdminSubscriptionNotFound
}
func (f *fakePanel) UpdateQuotaUser(_ context.Context, u *remnawave.PanelUser, fields map[string]any) (*remnawave.PanelUser, error) {
	target := f.users[u.ID]
	if target == nil {
		return nil, remnawave.ErrAdminSubscriptionNotFound
	}
	for key, v := range fields {
		switch key {
		case "username":
			target.Username = v.(string)
		case "description":
			description := v.(string)
			target.Description = &description
		case "status":
			target.Status = v.(string)
		case "activeInternalSquads":
			target.ActiveInternalSquads = nil
			for _, id := range v.([]string) {
				target.ActiveInternalSquads = append(target.ActiveInternalSquads, remnawave.PanelSquad{UUID: uuid.MustParse(id)})
			}
		case "expireAt":
			date, e := time.Parse(time.RFC3339, v.(string))
			if e != nil || !date.After(time.Now()) {
				return nil, errors.New("panel rejects past expiration")
			}
			target.ExpireAt = date
		case "trafficLimitBytes":
			target.TrafficLimitBytes = v.(int64)
		case "trafficLimitStrategy":
			target.TrafficLimitStrategy = v.(string)
		case "hwidDeviceLimit":
			limit := v.(int)
			target.HwidDeviceLimit = &limit
		case "externalSquadUuid":
			target.ExternalSquadUUID = v.(*uuid.UUID)
		default:
			panic(key)
		}
	}
	copy := *target
	return &copy, nil
}
func (f *fakePanel) EnsureQuotaUser(_ context.Context, name string, fields map[string]any) (*remnawave.PanelUser, error) {
	for _, u := range f.users {
		if u.Username == name {
			copy := *u
			return &copy, nil
		}
	}
	id := int64(100 + len(f.users))
	date, _ := time.Parse(time.RFC3339, fields["expireAt"].(string))
	limit := 1
	tag := remnawave.DeviceQuotaTag
	u := &remnawave.PanelUser{ID: id, UUID: uuid.New(), Username: name, Status: "DISABLED", ExpireAt: date, TrafficLimitBytes: fields["trafficLimitBytes"].(int64), TrafficLimitStrategy: fields["trafficLimitStrategy"].(string), HwidDeviceLimit: &limit, Tag: &tag, SubscriptionURL: f.source}
	f.users[id] = u
	copy := *u
	return &copy, nil
}
func (f *fakePanel) RegisterQuotaHWID(_ context.Context, u *remnawave.PanelUser, d remnawave.UserDevice) error {
	if old := f.registered[u.ID]; old != "" && old != d.Hwid {
		return ErrDeviceLimit
	}
	f.registered[u.ID] = d.Hwid
	return nil
}
func (f *fakePanel) RevokeQuotaUser(_ context.Context, u *remnawave.PanelUser) error {
	if f.revokeError {
		f.revokeError = false
		return errors.New("revoke temporarily unavailable")
	}
	f.revocations++
	return nil
}

func TestInterruptedCreationAndDeletionRecoverWithoutFreshTraffic(t *testing.T) {
	s, store, panel, _ := fixture(t)
	failed := false
	store.failSave = func(b *Binding) bool {
		if !failed && len(b.Slots) == 1 && b.Slots[0].ChildID > 0 {
			failed = true
			return true
		}
		return false
	}
	if w := request(s, store.b, "device-first-123"); w.Code != 503 {
		t.Fatalf("interruption not reported: %d", w.Code)
	}
	if len(panel.users) != 2 || store.b.Slots[0].ChildID != 0 {
		t.Fatal("failed checkpoint did not retain recoverable assignment")
	}
	var child *remnawave.PanelUser
	for id, u := range panel.users {
		if id != 1 {
			child = u
		}
	}
	child.UserTraffic.UsedTrafficBytes = 8 * gb
	if w := request(s, store.b, "device-first-123"); w.Code != 200 {
		t.Fatalf("creation recovery failed: %s", w.Body.String())
	}
	if len(panel.users) != 2 || store.b.Slots[0].ChildID != child.ID || child.UserTraffic.UsedTrafficBytes != 8*gb {
		t.Fatal("recovery replaced an existing allowance")
	}
	panel.revokeError = true
	if _, _, err := s.DeleteDevice(context.Background(), 1, panel.users[1].UUID, "device-first-123"); err == nil {
		t.Fatal("failed revocation reported success")
	}
	if store.b.Slots[0].RevokeHWID == "" || child.Status != "DISABLED" {
		t.Fatal("failed revocation was not checkpointed safely")
	}
	if w := request(s, store.b, "replacement-device-2"); w.Code != 200 {
		t.Fatalf("revocation recovery failed: %s", w.Body.String())
	}
	if child.UserTraffic.UsedTrafficBytes != 8*gb || panel.registered[child.ID] != "replacement-device-2" || len(panel.users) != 2 {
		t.Fatal("replacement bypassed recovery or reset quota")
	}
}
func (f *fakePanel) DeleteRawHWIDDevice(_ context.Context, id int64, _ uuid.UUID, hwid string) ([]remnawave.UserDevice, error) {
	if f.registered[id] == hwid {
		delete(f.registered, id)
	}
	return nil, nil
}
func fixture(t *testing.T) (*Service, *memoryStore, *fakePanel, *fakeSettings) {
	t.Helper()
	limit := 2
	parent := &remnawave.PanelUser{ID: 1, UUID: uuid.New(), Status: "ACTIVE", ExpireAt: time.Now().Add(48 * time.Hour).Truncate(time.Second), HwidDeviceLimit: &limit, TrafficLimitBytes: 100 * gb, TrafficLimitStrategy: "MONTH", ActiveInternalSquads: []remnawave.PanelSquad{{UUID: uuid.New()}}, SubscriptionURL: "https://panel.invalid/parent"}
	store := &memoryStore{plan: "p"}
	panel := &fakePanel{users: map[int64]*remnawave.PanelUser{1: parent}, registered: map[int64]string{}}
	settings := &fakeSettings{value: runtimeconfig.Settings{Plans: []runtimeconfig.PlanSettings{{ID: "p", DeviceLimit: 2, DeviceTrafficEnabled: true, DeviceTrafficEnforced: true, DeviceTrafficRules: []plans.DeviceTrafficRule{{Devices: 1, TrafficGB: 20}, {Devices: 2, TrafficGB: 40}}}}}}
	s := New(store, panel, settings, "https://my.example/plans?foo=bar")
	if err := s.Observe(context.Background(), parent, &remnawave.UserState{}); err != nil {
		t.Fatal(err)
	}
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Subscription-Userinfo", "upload=0; download=100; total=200")
		w.Write([]byte("device-configuration"))
	}))
	t.Cleanup(source.Close)
	panel.source = source.URL + "/child"
	return s, store, panel, settings
}
func request(s *Service, b *Binding, hwid string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", Route+b.Token.String(), nil)
	r.Header.Set("X-Hwid", hwid)
	r.Header.Set("User-Agent", "Happ/1.0")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func TestDeviceProfilesHaveIndependentNativeLimitsAndOnePublicLink(t *testing.T) {
	s, store, panel, _ := fixture(t)
	if len(panel.users[1].ActiveInternalSquads) != 0 {
		t.Fatal("shared parent still has data-plane access")
	}
	for _, hwid := range []string{"device-first-123", "device-second-456"} {
		if w := request(s, store.b, hwid); w.Code != 200 || w.Body.String() != "device-configuration" {
			t.Fatalf("import %s: %d %s", hwid, w.Code, w.Body.String())
		}
	}
	if len(panel.users) != 3 {
		t.Fatal("one native profile per device required")
	}
	for i, slot := range store.b.Slots {
		u := panel.users[slot.ChildID]
		if u.TrafficLimitBytes != int64((i+1)*20)*gb || u.TelegramID != nil || u.HwidDeviceLimit == nil || *u.HwidDeviceLimit != 1 || u.Status != "ACTIVE" {
			t.Fatalf("wrong native quota: %+v", u)
		}
	}
	if w := request(s, store.b, "device-third-789"); w.Code != 403 {
		t.Fatalf("device cap bypassed: %d", w.Code)
	}
	first := store.b.Slots[0]
	panel.users[first.ChildID].UserTraffic.UsedTrafficBytes = 7 * gb
	state := &remnawave.UserState{}
	if err := s.Observe(context.Background(), panel.users[1], state); err != nil {
		t.Fatal(err)
	}
	if state.SubscriptionLink == nil || *state.SubscriptionLink != "https://my.example"+Route+store.b.Token.String() || state.UsedTrafficBytes != 7*gb || len(state.Devices) != 2 || *state.Devices[0].TrafficUsedBytes != 7*gb {
		t.Fatalf("bad logical state: %+v", state)
	}
}

func TestPurchasedQuotaSurvivesCatalogEditsAndPlanDeletion(t *testing.T) {
	s, store, panel, settings := fixture(t)
	store.terms = &trafficpolicy.Terms{Enabled: true, Enforced: true, Rules: []trafficpolicy.Rule{{Devices: 1, TrafficGB: 20}, {Devices: 2, TrafficGB: 40}}}
	if w := request(s, store.b, "device-first-123"); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	child := panel.users[store.b.Slots[0].ChildID]
	child.UserTraffic.UsedTrafficBytes = 7 * gb
	settings.value.Plans = nil
	s.reconcile(context.Background())
	if child.Status != "ACTIVE" || child.TrafficLimitBytes != 20*gb || child.UserTraffic.UsedTrafficBytes != 7*gb || len(panel.users[1].ActiveInternalSquads) != 0 {
		t.Fatal("catalog deletion disabled or changed a purchased quota")
	}
	if w := request(s, store.b, "device-second-456"); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if panel.users[store.b.Slots[1].ChildID].TrafficLimitBytes != 40*gb {
		t.Fatal("new device did not inherit purchased terms")
	}
	state := &remnawave.UserState{}
	if err := s.Observe(context.Background(), panel.users[1], state); err != nil || !state.DeviceTrafficManaged || len(state.Devices) != 2 {
		t.Fatalf("purchased quota disappeared: %+v %v", state, err)
	}
}
func TestRemovalRevokesCredentialsWithoutResettingTrafficOrAllocatingAnotherUser(t *testing.T) {
	s, store, panel, _ := fixture(t)
	if w := request(s, store.b, "device-first-123"); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	slot := store.b.Slots[0]
	childID := slot.ChildID
	panel.users[childID].UserTraffic.UsedTrafficBytes = 13 * gb
	devices, managed, err := s.DeleteDevice(context.Background(), 1, panel.users[1].UUID, "device-first-123")
	if err != nil || !managed || len(devices) != 0 || panel.revocations != 1 || panel.users[childID].Status != "DISABLED" {
		t.Fatalf("delete failed: %v", err)
	}
	if w := request(s, store.b, "replacement-98765"); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if store.b.Slots[0].ChildID != childID || len(panel.users) != 2 || panel.users[childID].UserTraffic.UsedTrafficBytes != 13*gb {
		t.Fatal("deletion granted a fresh allowance")
	}
	if panel.registered[childID] != "replacement-98765" {
		t.Fatal("replacement was not HWID bound")
	}
}
func TestPerDeviceAndAggregateCapsAndNativeReset(t *testing.T) {
	s, store, panel, _ := fixture(t)
	request(s, store.b, "device-first-123")
	request(s, store.b, "device-second-456")
	first, second := panel.users[store.b.Slots[0].ChildID], panel.users[store.b.Slots[1].ChildID]
	first.UserTraffic.UsedTrafficBytes = 20 * gb
	if w := request(s, store.b, "device-first-123"); w.Code != 403 {
		t.Fatal("exhausted child still imports")
	}
	if second.Status != "ACTIVE" {
		t.Fatal("unrelated device was blocked")
	}
	first.UserTraffic.UsedTrafficBytes = 19 * gb
	second.UserTraffic.UsedTrafficBytes = 31 * gb
	panel.users[1].TrafficLimitBytes = 50 * gb
	s.reconcile(context.Background())
	if first.Status != "DISABLED" || second.Status != "DISABLED" {
		t.Fatal("aggregate tariff cap bypassed")
	}
	first.UserTraffic.UsedTrafficBytes = 0
	second.UserTraffic.UsedTrafficBytes = 0
	s.reconcile(context.Background())
	if first.Status != "ACTIVE" || second.Status != "ACTIVE" {
		t.Fatal("native monthly reset did not restore access")
	}
}
func TestParentExpiryAndQuotaOptOut(t *testing.T) {
	s, store, panel, settings := fixture(t)
	request(s, store.b, "device-first-123")
	child := panel.users[store.b.Slots[0].ChildID]
	panel.users[1].Status = "DISABLED"
	s.reconcile(context.Background())
	if child.Status != "DISABLED" {
		t.Fatal("blocked parent retained device access")
	}
	panel.users[1].Status = "ACTIVE"
	panel.users[1].ExpireAt = time.Now().Add(-time.Hour)
	s.reconcile(context.Background())
	if child.Status != "DISABLED" || len(child.ActiveInternalSquads) != 0 {
		t.Fatal("expired parent retained device access")
	}
	panel.users[1].ExpireAt = time.Now().Add(time.Hour).Truncate(time.Second)
	s.reconcile(context.Background())
	if child.Status != "ACTIVE" {
		t.Fatal("renewal not propagated")
	}
	settings.value.Plans[0].DeviceTrafficEnforced = false
	s.reconcile(context.Background())
	if child.Status != "DISABLED" || len(panel.users[1].ActiveInternalSquads) != 1 || store.b.Isolated {
		t.Fatal("opt out failed to disable children before restoring parent")
	}
}
func TestUnknownTelemetryNeverReportsZeroAndInvalidImportsDoNotAllocate(t *testing.T) {
	s, store, panel, _ := fixture(t)
	for _, hwid := range []string{"", "short", "../../invalid-device"} {
		if w := request(s, store.b, hwid); w.Code != 400 {
			t.Fatalf("invalid HWID %q: %d", hwid, w.Code)
		}
	}
	if len(store.b.Slots) != 0 {
		t.Fatal("invalid import consumed a slot")
	}
	if w := request(s, store.b, "device-first-123"); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	panel.fetchError = store.b.Slots[0].ChildID
	if err := s.Observe(context.Background(), panel.users[1], &remnawave.UserState{}); err == nil {
		t.Fatal("failed telemetry was presented as a valid zero")
	}
	if panel.users[store.b.Slots[0].ChildID].Status != "DISABLED" {
		t.Fatal("unknown allowance left device access running")
	}
	bad := httptest.NewRecorder()
	s.ServeHTTP(bad, httptest.NewRequest("GET", Route+uuid.NewString(), nil))
	if bad.Code != 404 {
		t.Fatal("unknown bearer token accepted")
	}
}

func TestBackgroundActivationAndOrphanCleanup(t *testing.T) {
	s, store, panel, _ := fixture(t)
	for _, id := range store.b.Squads {
		panel.users[1].ActiveInternalSquads = append(panel.users[1].ActiveInternalSquads, remnawave.PanelSquad{UUID: uuid.MustParse(id)})
	}
	store.b = nil
	store.discover = true
	s.reconcile(context.Background())
	if store.b == nil || !store.b.Isolated || len(panel.users[1].ActiveInternalSquads) != 0 {
		t.Fatal("offline subscription was not activated")
	}
	if w := request(s, store.b, "device-first-123"); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	child := panel.users[store.b.Slots[0].ChildID]
	store.b.SubscriptionID = nil
	s.reconcile(context.Background())
	if child.Status != "DISABLED" || panel.users[1].Status != "DISABLED" || len(panel.users[1].ActiveInternalSquads) != 0 {
		t.Fatal("orphan cleanup restored shared access")
	}
}
func TestProxyDoesNotForwardCabinetCredentialsOrFollowCrossOriginRedirects(t *testing.T) {
	s, _, _, _ := fixture(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("X-Hwid") != "device-first-123" {
			t.Error("unsafe forwarded headers")
		}
		w.Header().Set("Set-Cookie", "secret=x")
		w.Header().Set("Subscription-Userinfo", "total=200")
		w.Write([]byte("config"))
	}))
	defer upstream.Close()
	req := httptest.NewRequest("GET", Route+uuid.NewString(), nil)
	req.Header.Set("Cookie", "session=secret")
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("X-Hwid", "device-first-123")
	w := httptest.NewRecorder()
	if err := s.proxy(context.Background(), w, req, upstream.URL); err != nil {
		t.Fatal(err)
	}
	if w.Header().Get("Set-Cookie") != "" || w.Header().Get("Cache-Control") == "public" || w.Header().Get("Subscription-Userinfo") != "total=200" {
		t.Fatal("unsafe response headers")
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, upstream.URL, 302) }))
	defer redirect.Close()
	w = httptest.NewRecorder()
	if err := s.proxy(context.Background(), w, req, redirect.URL); err == nil || strings.Contains(w.Body.String(), "config") {
		t.Fatal("cross-origin redirect allowed")
	}
}
