package miniapp

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreSupportMediaPersistsSupportedPhotoAndVideo(t *testing.T) {
	t.Parallel()
	uploadDir := t.TempDir()
	fixtures := []struct {
		name         string
		data         []byte
		originalName string
		kind         string
		mime         string
		extension    string
	}{
		{name: "photo", data: testLogoPNG(t, 64, 32), originalName: "ошибка.png", kind: "image", mime: "image/png", extension: ".png"},
		{name: "mp4", data: []byte("\x00\x00\x00\x18ftypisom\x00\x00\x02\x00isomiso2"), originalName: "recording.mp4", kind: "video", mime: "video/mp4", extension: ".mp4"},
		{name: "webm", data: []byte{0x1a, 0x45, 0xdf, 0xa3, 0x9f, 0x42, 0x86, 0x81}, originalName: "recording.webm", kind: "video", mime: "video/webm", extension: ".webm"},
		{name: "mov", data: []byte("\x00\x00\x00\x18ftypqt  \x00\x00\x00\x00qt  "), originalName: "iphone.mov", kind: "video", mime: "video/quicktime", extension: ".mov"},
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			attachment, created, err := storeSupportMedia(uploadDir, fixture.data, fixture.originalName)
			if err != nil {
				t.Fatalf("store media: %v", err)
			}
			if !created {
				t.Fatal("first upload should create a file")
			}
			if attachment.Type != fixture.kind || attachment.MIME != fixture.mime || attachment.OriginalName != fixture.originalName || !strings.HasSuffix(attachment.StorageName, fixture.extension) {
				t.Fatalf("unexpected attachment: %+v", attachment)
			}
			stored, err := os.ReadFile(filepath.Join(uploadDir, attachment.StorageName))
			if err != nil {
				t.Fatalf("read stored media: %v", err)
			}
			if !bytes.Equal(stored, fixture.data) {
				t.Fatal("stored media differs from upload")
			}
			duplicate, duplicateCreated, err := storeSupportMedia(uploadDir, fixture.data, fixture.originalName)
			if err != nil || duplicateCreated || duplicate.StorageName != attachment.StorageName {
				t.Fatalf("duplicate upload = %+v, created=%v, err=%v", duplicate, duplicateCreated, err)
			}
		})
	}
}

func TestStoreSupportMediaRejectsUnsafeFormatsAndLimits(t *testing.T) {
	t.Parallel()
	uploadDir := t.TempDir()
	if _, _, err := storeSupportMedia(uploadDir, []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), "unsafe.svg"); !errors.Is(err, errSupportMediaUnsupported) {
		t.Fatalf("SVG error = %v, want %v", err, errSupportMediaUnsupported)
	}
	if _, _, err := storeSupportMedia(uploadDir, make([]byte, maxSupportMediaSize+1), "large.mp4"); !errors.Is(err, errSupportMediaTooLarge) {
		t.Fatalf("oversized error = %v, want %v", err, errSupportMediaTooLarge)
	}
	if _, _, err := storeSupportMedia(uploadDir, testLogoPNG(t, 8193, 1), "wide.png"); !errors.Is(err, errSupportMediaDimensions) {
		t.Fatalf("dimensions error = %v, want %v", err, errSupportMediaDimensions)
	}
}

func TestSanitizeSupportMediaName(t *testing.T) {
	t.Parallel()
	if got := sanitizeSupportMediaName("../screen\nshot.png", "image", ".png"); strings.ContainsAny(got, "/\\\n\r") || got == "" {
		t.Fatalf("unsafe filename was not sanitized: %q", got)
	}
	if got := sanitizeSupportMediaName("", "video", ".mp4"); got != "video.mp4" {
		t.Fatalf("fallback filename = %q", got)
	}
}

func TestSignSupportMediaURLBindsMessageAndExpiry(t *testing.T) {
	initMiniAppTestConfig()
	first, err := signSupportMediaURL(42, 123456789)
	if err != nil {
		t.Fatalf("sign media URL: %v", err)
	}
	repeated, err := signSupportMediaURL(42, 123456789)
	if err != nil || repeated != first {
		t.Fatalf("signature is not deterministic: %q %q err=%v", first, repeated, err)
	}
	otherMessage, _ := signSupportMediaURL(43, 123456789)
	otherExpiry, _ := signSupportMediaURL(42, 123456790)
	if first == otherMessage || first == otherExpiry || len(first) != 64 {
		t.Fatalf("signature does not bind media request: %q %q %q", first, otherMessage, otherExpiry)
	}
}
