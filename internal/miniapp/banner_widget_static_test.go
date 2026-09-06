package miniapp

import (
	"strings"
	"testing"
)

func TestDashboardBannerWiresUploadCropActionsAndLayoutEditing(t *testing.T) {
	appRaw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	stylesRaw, err := embeddedStatic.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}
	app := string(appRaw)
	for _, fragment := range []string{
		`/api/mini-app/admin/banner/upload`,
		`accept=".png,.gif,.mp4,image/png,image/gif,video/mp4"`,
		`data-action="admin-add-banner"`,
		`data-action="admin-edit-banner"`,
		`data-action="admin-remove-banner"`,
		`data-action="open-banner"`,
		`bannerCropX`,
		`bannerCropY`,
		`bannerZoom`,
		`BANNER_PAGE_TARGETS`,
		`autoplay loop muted playsinline`,
	} {
		if !strings.Contains(app, fragment) {
			t.Fatalf("banner UI is missing %q", fragment)
		}
	}

	styles := string(stylesRaw)
	for _, fragment := range []string{
		`.dashboard-banner`,
		`.admin-banner-preview`,
		`.admin-banner-upload:focus-within`,
		`object-position: var(--banner-crop-x, 50%) var(--banner-crop-y, 50%)`,
		`@media (prefers-reduced-motion: reduce)`,
	} {
		if !strings.Contains(styles, fragment) {
			t.Fatalf("banner styles are missing %q", fragment)
		}
	}
}
