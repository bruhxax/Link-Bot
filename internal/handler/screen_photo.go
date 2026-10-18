package handler

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type screenPhotoKey struct {
	botID    int64
	source   string
	modified int64
	size     int64
}

type screenPhotoEntry struct {
	fileID  string
	expires time.Time
}

type screenPhotoCache struct {
	mu      sync.Mutex
	entries map[screenPhotoKey]screenPhotoEntry
}

var telegramScreenPhotos = screenPhotoCache{entries: make(map[screenPhotoKey]screenPhotoEntry)}

// Telegram file IDs are scoped to a bot. Local edits invalidate immediately;
// remote URLs are checked again after five minutes, even if their URL is unchanged.
func (c *screenPhotoCache) send(ctx context.Context, b *bot.Bot, source string, params *bot.SendPhotoParams) (*models.Message, error) {
	started := time.Now()
	source = strings.TrimSpace(source)
	key := screenPhotoKey{botID: b.ID(), source: source}
	ttl := 5 * time.Minute
	if info, err := os.Stat(source); err == nil {
		key.modified, key.size = info.ModTime().UnixNano(), info.Size()
		ttl = 24 * time.Hour
	}
	c.mu.Lock()
	entry, found := c.entries[key]
	c.mu.Unlock()
	found = found && time.Now().Before(entry.expires)
	var uploaded int
	defer func() {
		if elapsed := time.Since(started); elapsed >= time.Second {
			slog.Info("bot: slow screen photo", "duration", elapsed, "cached", found, "upload_bytes", uploaded)
		}
	}()
	request := *params
	if found {
		request.Photo = &models.InputFileString{Data: entry.fileID}
		msg, err := b.SendPhoto(ctx, &request)
		if err == nil {
			return msg, nil
		}
		// Do not retry rate limits, network errors, or unrelated bad requests.
		if !errors.Is(err, bot.ErrorBadRequest) || (!strings.Contains(err.Error(), "file identifier") && !strings.Contains(err.Error(), "wrong remote file")) {
			return nil, err
		}
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
	}
	data, filename, err := readImageSourceContext(ctx, source)
	if err != nil {
		return nil, err
	}
	data, filename = optimizeScreenImage(data, filename)
	uploaded = len(data)
	request.Photo = &models.InputFileUpload{Filename: filename, Data: bytes.NewReader(data)}
	msg, err := b.SendPhoto(ctx, &request)
	if err != nil {
		return nil, err
	}
	if msg != nil && len(msg.Photo) > 0 {
		fileID := msg.Photo[len(msg.Photo)-1].FileID
		if fileID != "" {
			c.mu.Lock()
			if c.entries == nil || len(c.entries) >= 128 {
				c.entries = make(map[screenPhotoKey]screenPhotoEntry)
			}
			c.entries[key] = screenPhotoEntry{fileID: fileID, expires: time.Now().Add(ttl)}
			c.mu.Unlock()
		}
	}
	return msg, nil
}
