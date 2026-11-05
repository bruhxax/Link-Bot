package runtimeconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-telegram/bot"
)

// Telegram ignores a custom emoji entity when its fallback character does not
// match the emoji attached to the sticker. Keep the editable HTML in sync with
// Telegram's own sticker metadata, including templates saved before this fix.
var premiumEmojiHTMLTag = regexp.MustCompile(`(?is)(<tg-emoji\s+emoji-id=["']([0-9]{5,32})["'][^>]*>)([^<]*)(</tg-emoji>)`)

func (s *Service) SetTelegramBot(ctx context.Context, telegramBot *bot.Bot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emojiBot = telegramBot
	settings := s.Snapshot()
	if err := s.normalizePremiumEmoji(ctx, &settings); err != nil {
		return err
	}
	if reflect.DeepEqual(settings, s.Snapshot()) {
		return nil
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("encode corrected premium emoji: %w", err)
	}
	if err := s.repository.Save(ctx, raw, 0); err != nil {
		return fmt.Errorf("save corrected premium emoji: %w", err)
	}
	s.value.Store(settings)
	return nil
}

// The caller holds s.mu. One API call resolves all IDs in the editable
// settings, and resolved IDs stay cached for subsequent settings updates.
func (s *Service) normalizePremiumEmoji(ctx context.Context, settings *Settings) error {
	if s.emojiBot == nil {
		return nil
	}
	if s.emojiAlts == nil {
		s.emojiAlts = make(map[string]string)
	}
	ids := make(map[string]bool)
	visitPremiumEmojiStrings(reflect.ValueOf(settings), func(value string) string {
		for _, match := range premiumEmojiHTMLTag.FindAllStringSubmatch(value, -1) {
			if _, known := s.emojiAlts[match[2]]; !known {
				ids[match[2]] = true
			}
		}
		return value
	})
	if len(ids) > 0 {
		pending := make([]string, 0, len(ids))
		for id := range ids {
			pending = append(pending, id)
		}
		sort.Strings(pending)
		for start := 0; start < len(pending); start += 200 {
			end := start + 200
			if end > len(pending) {
				end = len(pending)
			}
			lookupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			stickers, err := s.emojiBot.GetCustomEmojiStickers(lookupCtx, &bot.GetCustomEmojiStickersParams{CustomEmojiIDs: pending[start:end]})
			cancel()
			if err != nil {
				return fmt.Errorf("check premium emoji with Telegram: %w", err)
			}
			for _, sticker := range stickers {
				if sticker != nil && sticker.CustomEmojiID != "" && sticker.Emoji != "" {
					s.emojiAlts[sticker.CustomEmojiID] = sticker.Emoji
				}
			}
			for _, id := range pending[start:end] {
				if s.emojiAlts[id] == "" {
					return fmt.Errorf("Telegram does not recognize premium emoji ID %s", id)
				}
			}
		}
	}
	visitPremiumEmojiStrings(reflect.ValueOf(settings), func(value string) string {
		return premiumEmojiHTMLTag.ReplaceAllStringFunc(value, func(tag string) string {
			parts := premiumEmojiHTMLTag.FindStringSubmatch(tag)
			if len(parts) != 5 || s.emojiAlts[parts[2]] == "" || strings.TrimSpace(parts[3]) == s.emojiAlts[parts[2]] {
				return tag
			}
			return parts[1] + s.emojiAlts[parts[2]] + parts[4]
		})
	})
	return nil
}

func visitPremiumEmojiStrings(value reflect.Value, edit func(string) string) {
	if !value.IsValid() {
		return
	}
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !value.IsNil() {
			visitPremiumEmojiStrings(value.Elem(), edit)
		}
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			if value.Field(i).CanInterface() {
				visitPremiumEmojiStrings(value.Field(i), edit)
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			visitPremiumEmojiStrings(value.Index(i), edit)
		}
	case reflect.Map:
		for _, key := range value.MapKeys() {
			item := reflect.New(value.Type().Elem()).Elem()
			item.Set(value.MapIndex(key))
			visitPremiumEmojiStrings(item, edit)
			value.SetMapIndex(key, item)
		}
	case reflect.String:
		if value.CanSet() && strings.Contains(value.String(), "<tg-emoji") {
			value.SetString(edit(value.String()))
		} else if strings.Contains(value.String(), "<tg-emoji") {
			edit(value.String())
		}
	}
}
