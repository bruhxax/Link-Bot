package runtimeconfig

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"link-bot/internal/broadcast"
)

type TorrentSettings struct {
	UserEnabled  bool   `json:"userEnabled"`
	AdminEnabled bool   `json:"adminEnabled"`
	UserText     string `json:"userText"`
	AdminText    string `json:"adminText"`
}

func DefaultTorrentSettings() TorrentSettings {
	return TorrentSettings{
		UserEnabled: true, AdminEnabled: true,
		UserText:  "🧲 <b>Обнаружен торрент-трафик</b>\n\nНода: <b>{node}</b>\nВаш IP заблокирован на {duration}\nБлокировка закончится: {unblock_at}",
		AdminText: "🧲 <b>Обнаружен торрент-трафик</b>\n\nПользователь: <b>{username}</b>\nUUID: <code>{user_uuid}</code>\nНода: <b>{node}</b>\nIP: <code>{ip}</code>\nРезультат: {status}\nСрок: {duration}\nСнятие блокировки: {unblock_at}\nВремя: {time}",
	}
}

var torrentVariablePattern = regexp.MustCompile(`\{([a-zA-Z_]+)\}`)

func ReplaceTorrentVariables(template string, values map[string]string, escape func(string) string) string {
	return torrentVariablePattern.ReplaceAllStringFunc(template, func(token string) string {
		if value, ok := values[token[1:len(token)-1]]; ok {
			return escape(value)
		}
		return token
	})
}

func ValidateTorrentSettings(value *TorrentSettings) error {
	defaults := DefaultTorrentSettings()
	for _, item := range []struct {
		text     *string
		fallback string
		user     bool
	}{
		{&value.UserText, defaults.UserText, true}, {&value.AdminText, defaults.AdminText, false},
	} {
		if strings.TrimSpace(*item.text) == "" {
			*item.text = item.fallback
		}
		if utf8.RuneCountInString(*item.text) > 2000 {
			return fmt.Errorf("текст уведомления о торрентах: максимум 2000 символов")
		}
		allowed := map[string]bool{"node": true, "duration": true, "unblock_at": true, "time": true, "username": true, "status": true}
		if !item.user {
			allowed["ip"] = true
			allowed["user_uuid"] = true
		}
		sample := *item.text
		for _, match := range torrentVariablePattern.FindAllStringSubmatch(sample, -1) {
			if !allowed[match[1]] {
				return fmt.Errorf("неизвестная переменная уведомления: %s", match[0])
			}
			sample = strings.ReplaceAll(sample, match[0], strings.Repeat("x", 160))
		}
		if err := broadcast.ValidateHTML(sample, true); err != nil {
			return err
		}
	}
	return nil
}
