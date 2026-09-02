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

	provider := strings.TrimSpace(values.Get("provider"))
	if provider == "" {
		provider = sessionProviderTelegram
	}
	if provider != sessionProviderTelegram && provider != sessionProviderGoogle {
		return nil, fmt.Errorf("invalid session provider")
	}
	googleSubject := ""
	googleEmail := ""
	googleEmailVerified := false
	if provider == sessionProviderGoogle {
		googleSubject = strings.TrimSpace(values.Get("google_subject"))
		googleEmail = strings.ToLower(strings.TrimSpace(values.Get("google_email")))
		googleEmailVerified = values.Get("google_email_verified") == "1"
		if googleSubject == "" || googleEmail == "" || !googleEmailVerified {
			return nil, fmt.Errorf("invalid google browser session")
		}
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
		Provider:            provider,
		GoogleSubject:       googleSubject,
		GoogleEmail:         googleEmail,
		GoogleEmailVerified: googleEmailVerified,
	}, nil
}

func createTelegramBrowserSessionData(user telegramUser, botToken string, now time.Time) (string, error) {
	return createBrowserSessionData(&session{User: user, Provider: sessionProviderTelegram}, botToken, now)
}

func createBrowserSessionData(sess *session, botToken string, now time.Time) (string, error) {
	if sess == nil || sess.User.ID <= 0 {
		return "", fmt.Errorf("invalid user id")
	}
	if strings.TrimSpace(botToken) == "" {
		return "", fmt.Errorf("missing bot token")
	}
	provider := strings.TrimSpace(sess.Provider)
	if provider == "" {
		provider = sessionProviderTelegram
	}
	if provider != sessionProviderTelegram && provider != sessionProviderGoogle {
		return "", fmt.Errorf("invalid session provider")
	}
	if provider == sessionProviderGoogle && (strings.TrimSpace(sess.GoogleSubject) == "" || strings.TrimSpace(sess.GoogleEmail) == "" || !sess.GoogleEmailVerified) {
		return "", fmt.Errorf("invalid google browser session")
	}

	values := url.Values{}
	values.Set("id", strconv.FormatInt(sess.User.ID, 10))
	values.Set("auth_date", strconv.FormatInt(now.UTC().Unix(), 10))
	optional := map[string]string{
		"first_name":    sess.User.FirstName,
		"last_name":     sess.User.LastName,
		"username":      sess.User.Username,
		"photo_url":     sess.User.PhotoURL,
		"language_code": sess.User.LanguageCode,
	}
	for key, value := range optional {
		if value = strings.TrimSpace(value); value != "" {
			values.Set(key, value)
		}
	}
	if provider == sessionProviderGoogle {
		values.Set("provider", sessionProviderGoogle)
		values.Set("google_subject", strings.TrimSpace(sess.GoogleSubject))
		values.Set("google_email", strings.ToLower(strings.TrimSpace(sess.GoogleEmail)))
		values.Set("google_email_verified", "1")
	}

	var pairs []string
	for key, items := range values {
		if len(items) > 0 {
			pairs = append(pairs, fmt.Sprintf("%s=%s", key, items[0]))
		}
	}
	sort.Strings(pairs)
	secret := sha256.Sum256([]byte(botToken))
	values.Set("hash", hex.EncodeToString(hmacSHA256(secret[:], []byte(strings.Join(pairs, "\n")))))
	return values.Encode(), nil
}

func hmacSHA256(key, payload []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	return mac.Sum(nil)
}
