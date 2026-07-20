package miniapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	telegramInitDataMaxAge     = 12 * time.Hour
	telegramLoginDataMaxAge    = 7 * 24 * time.Hour
	telegramInitDataFutureSkew = 5 * time.Minute
	sessionProviderTelegram    = "telegram"
	sessionProviderGoogle      = "google"
)

var currentMiniAppTime = func() time.Time {
	return time.Now().UTC()
}

type telegramUser struct {
	ID           int64  `json:"id"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	Username     string `json:"username"`
	PhotoURL     string `json:"photo_url"`
	LanguageCode string `json:"language_code"`
}

type session struct {
	QueryID             string
	StartParam          string
	AuthDate            time.Time
	User                telegramUser
	Provider            string
	GoogleSubject       string
	GoogleEmail         string
	GoogleEmailVerified bool
}

func parseAndValidateInitData(initData, botToken string) (*session, error) {
	if strings.TrimSpace(initData) == "" {
		return nil, fmt.Errorf("missing init data")
	}

	values, err := url.ParseQuery(initData)
	if err != nil {
		return nil, fmt.Errorf("parse init data: %w", err)
	}

	hash := values.Get("hash")
	if hash == "" {
		return nil, fmt.Errorf("missing hash")
	}

	var pairs []string
	for key, items := range values {
		if key == "hash" || len(items) == 0 {
			continue
		}
		pairs = append(pairs, fmt.Sprintf("%s=%s", key, items[0]))
	}
	sort.Strings(pairs)

	secret := hmacSHA256([]byte("WebAppData"), []byte(botToken))
	checksum := hmacSHA256(secret, []byte(strings.Join(pairs, "\n")))
	if !hmac.Equal([]byte(hex.EncodeToString(checksum)), []byte(hash)) {
		return nil, fmt.Errorf("init data hash mismatch")
	}

	authDateUnix, err := strconv.ParseInt(values.Get("auth_date"), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse auth date: %w", err)
	}
	authDate := time.Unix(authDateUnix, 0).UTC()

	var user telegramUser
	if err := json.Unmarshal([]byte(values.Get("user")), &user); err != nil {
		return nil, fmt.Errorf("parse user: %w", err)
	}
	if user.ID <= 0 {
		return nil, fmt.Errorf("invalid user id")
	}

	now := currentMiniAppTime()
	if authDate.After(now.Add(telegramInitDataFutureSkew)) {
		return nil, fmt.Errorf("auth date is too far in the future")
	}
	if now.Sub(authDate) > telegramInitDataMaxAge {
		return nil, fmt.Errorf("init data expired")
	}

	return &session{
		QueryID:    values.Get("query_id"),
		StartParam: values.Get("start_param"),
		AuthDate:   authDate,
		User:       user,
		Provider:   sessionProviderTelegram,
	}, nil
}

func parseAndValidateLoginData(loginData, botToken string) (*session, error) {
	if strings.TrimSpace(loginData) == "" {
		return nil, fmt.Errorf("missing login data")
	}

	values, err := url.ParseQuery(loginData)
	if err != nil {
		return nil, fmt.Errorf("parse login data: %w", err)
	}

	hash := values.Get("hash")
	if hash == "" {
		return nil, fmt.Errorf("missing hash")
	}

	var pairs []string
	for key, items := range values {
		if key == "hash" || len(items) == 0 {
			continue
		}
		pairs = append(pairs, fmt.Sprintf("%s=%s", key, items[0]))
	}
	sort.Strings(pairs)

	secret := sha256.Sum256([]byte(botToken))
	checksum := hmacSHA256(secret[:], []byte(strings.Join(pairs, "\n")))
	if !hmac.Equal([]byte(hex.EncodeToString(checksum)), []byte(hash)) {
		return nil, fmt.Errorf("login data hash mismatch")
	}

	authDateUnix, err := strconv.ParseInt(values.Get("auth_date"), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse auth date: %w", err)
	}
	authDate := time.Unix(authDateUnix, 0).UTC()

	userID, err := strconv.ParseInt(values.Get("id"), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse user id: %w", err)
	}
	if userID <= 0 {
		return nil, fmt.Errorf("invalid user id")
	}

	now := currentMiniAppTime()
	if authDate.After(now.Add(telegramInitDataFutureSkew)) {
		return nil, fmt.Errorf("auth date is too far in the future")
	}
	if now.Sub(authDate) > telegramLoginDataMaxAge {
		return nil, fmt.Errorf("login data expired")
	}

	return &session{
		AuthDate: authDate,
		User: telegramUser{
			ID:           userID,
			FirstName:    values.Get("first_name"),
			LastName:     values.Get("last_name"),
			Username:     values.Get("username"),
			PhotoURL:     values.Get("photo_url"),
			LanguageCode: values.Get("language_code"),
		},
		Provider: sessionProviderTelegram,
	}, nil
}

func hmacSHA256(key, payload []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	return mac.Sum(nil)
}
