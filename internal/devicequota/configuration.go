package devicequota

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v4"
)

type Configuration struct {
	Body    []byte
	Headers map[string]string
}

func sourceHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func configurationFormat(r *http.Request) string {
	client := strings.ToLower(strings.SplitN(limitedHeader(r, "User-Agent"), "/", 2)[0])
	accept := limitedHeader(r, "Accept")
	if accept == "*/*" {
		accept = ""
	}
	return sourceHash(client + "|" + accept + "|" + strings.ToLower(limitedHeader(r, "X-Device-Os")))
}
func (s *Service) writeConfiguration(w http.ResponseWriter, cfg *Configuration) {
	for _, key := range []string{"Content-Type", "Content-Disposition", "Subscription-Userinfo", "Profile-Title", "Profile-Update-Interval", "Support-Url", "Announce"} {
		if v := cfg.Headers[key]; v != "" {
			w.Header().Set(key, v)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Profile-Web-Page-Url", s.origin)
	_, _ = w.Write(cfg.Body)
}
func (s *PostgresStore) LoadConfig(ctx context.Context, bindingID int64, number int, format, hwid, source string) (*Configuration, error) {
	var cfg Configuration
	var headers []byte
	err := s.pool.QueryRow(ctx, `SELECT body,headers FROM device_profile_config WHERE binding_id=$1 AND slot_number=$2 AND format_key=$3 AND hwid=$4 AND source_hash=$5`, bindingID, number, format, hwid, source).Scan(&cfg.Body, &headers)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(headers, &cfg.Headers); err != nil {
		return nil, err
	}
	return &cfg, nil
}
func (s *PostgresStore) SaveConfig(ctx context.Context, bindingID int64, number int, format, hwid, source string, cfg *Configuration) error {
	headers, err := json.Marshal(cfg.Headers)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO device_profile_config(binding_id,slot_number,format_key,hwid,source_hash,body,headers) VALUES($1,$2,$3,$4,$5,$6,$7)
 ON CONFLICT(binding_id,slot_number,format_key) DO UPDATE SET hwid=EXCLUDED.hwid,source_hash=EXCLUDED.source_hash,body=EXCLUDED.body,headers=EXCLUDED.headers,updated_at=NOW()`, bindingID, number, format, hwid, source, cfg.Body, string(headers))
	if err != nil {
		return err
	}
	// User-Agent/Accept are client controlled. Keep a bounded set of recent
	// formats instead of allowing one bearer to grow the private cache forever.
	_, err = s.pool.Exec(ctx, `DELETE FROM device_profile_config WHERE binding_id=$1 AND slot_number=$2 AND format_key IN
 (SELECT format_key FROM device_profile_config WHERE binding_id=$1 AND slot_number=$2 ORDER BY updated_at DESC,format_key LIMIT ALL OFFSET 4)`, bindingID, number)
	return err
}
func (s *Service) warmConfiguration(ctx context.Context, b *Binding, hwid string) {
	slot := findSlot(b, hwid)
	if slot == nil || slot.Paused {
		return
	}
	user, err := s.panel.RawUser(ctx, slot.ChildID, slot.ChildUUID)
	if err != nil || !isActive(user) {
		return
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://local.invalid/", nil)
	req.Header.Set("X-Hwid", hwid)
	req.Header.Set("User-Agent", slot.Device.UserAgent)
	req.Header.Set("X-Device-Os", slot.Device.Platform)
	req.Header.Set("X-Ver-Os", slot.Device.OSVersion)
	req.Header.Set("X-Device-Model", slot.Device.DeviceModel)
	cfg, err := s.fetch(ctx, req, user.SubscriptionURL)
	if err == nil {
		_ = s.store.SaveConfig(ctx, b.ID, slot.Number, configurationFormat(req), hwid, sourceHash(user.SubscriptionURL), cfg)
	}
}
