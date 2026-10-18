package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-telegram/bot"
)

func TestScreenPhotoReuseInvalidationAndBotIsolation(t *testing.T) {
	var uploads, reused, requests int
	invalid, forbidden := false, false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if err := r.ParseMultipartForm(4 << 20); err != nil {
			t.Error(err)
			return
		}
		defer r.MultipartForm.RemoveAll()
		if forbidden {
			fmt.Fprint(w, `{"ok":false,"error_code":403,"description":"Forbidden"}`)
			return
		}
		if r.FormValue("photo") == "cached-file" {
			reused++
			if invalid {
				invalid = false
				fmt.Fprint(w, `{"ok":false,"error_code":400,"description":"Bad Request: wrong file identifier/HTTP URL specified"}`)
				return
			}
		} else if len(r.MultipartForm.File) > 0 {
			uploads++
		} else {
			t.Error("neither upload nor saved file ID")
		}
		fmt.Fprint(w, `{"ok":true,"result":{"message_id":1,"photo":[{"file_id":"cached-file","width":100,"height":100}]}}`)
	}))
	defer server.Close()
	newBot := func(token string) *bot.Bot {
		b, err := bot.New(token, bot.WithSkipGetMe(), bot.WithServerURL(server.URL))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	b := newBot("123:test")
	c := &screenPhotoCache{}
	path := filepath.Join(t.TempDir(), "banner.png")
	if err := os.WriteFile(path, []byte("first image"), 0600); err != nil {
		t.Fatal(err)
	}
	send := func(b *bot.Bot) error {
		_, err := c.send(context.Background(), b, path, &bot.SendPhotoParams{ChatID: 1, Caption: "menu"})
		return err
	}
	for i := 0; i < 2; i++ {
		if err := send(b); err != nil {
			t.Fatal(err)
		}
	}
	if uploads != 1 || reused != 1 {
		t.Fatalf("uploads=%d reused=%d", uploads, reused)
	}
	if err := os.WriteFile(path, []byte("replacement image, new size"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := send(b); err != nil {
		t.Fatal(err)
	}
	if uploads != 2 {
		t.Fatal("changed banner reused an outdated file")
	}
	if err := send(newBot("456:test")); err != nil {
		t.Fatal(err)
	}
	if uploads != 3 {
		t.Fatal("another bot reused a foreign file ID")
	}
	invalid = true
	if err := send(b); err != nil {
		t.Fatal(err)
	}
	if uploads != 4 {
		t.Fatal("invalid file ID did not fall back to upload")
	}
	forbidden = true
	beforeForbidden := requests
	if err := send(b); err == nil {
		t.Fatal("forbidden send succeeded")
	}
	if uploads != 4 {
		t.Fatal("forbidden send retried with upload")
	}
	if requests != beforeForbidden+1 {
		t.Fatal("forbidden send retried")
	}
}

func TestScreenPhotoRemoteCacheExpiryAndCancellation(t *testing.T) {
	downloads := 0
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads++
		fmt.Fprint(w, "image")
	}))
	defer source.Close()
	telegram := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1024); err != nil {
			t.Error(err)
			return
		}
		defer r.MultipartForm.RemoveAll()
		fmt.Fprint(w, `{"ok":true,"result":{"message_id":1,"photo":[{"file_id":"remote-file"}]}}`)
	}))
	defer telegram.Close()
	b, err := bot.New("123:test", bot.WithSkipGetMe(), bot.WithServerURL(telegram.URL))
	if err != nil {
		t.Fatal(err)
	}
	c := &screenPhotoCache{}
	for i := 0; i < 2; i++ {
		if _, err := c.send(context.Background(), b, source.URL, &bot.SendPhotoParams{ChatID: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if downloads != 1 {
		t.Fatalf("downloads=%d", downloads)
	}
	for key, entry := range c.entries {
		entry.expires = time.Now().Add(-time.Second)
		c.entries[key] = entry
	}
	if _, err := c.send(context.Background(), b, source.URL, &bot.SendPhotoParams{ChatID: 1}); err != nil {
		t.Fatal(err)
	}
	if downloads != 2 {
		t.Fatal("remote banner was not refreshed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := readImageSourceContext(ctx, source.URL); err == nil {
		t.Fatal("cancelled download succeeded")
	}
}
