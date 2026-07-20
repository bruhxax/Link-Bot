package runtimeconfig

import (
	"strings"
	"testing"
)

func TestNormalizeAndValidatePreservesPlanOrder(t *testing.T) {
	settings := DefaultSettings()
	settings.Plans = []PlanSettings{
		{ID: "custom_a", Enabled: true, Months: 1, PriceRub: 100},
		{ID: "custom_b", Enabled: true, Months: 2, PriceRub: 200},
	}
	settings.Plans[0], settings.Plans[1] = settings.Plans[1], settings.Plans[0]

	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}
	if settings.Plans[0].ID != "custom_b" || settings.Plans[1].ID != "custom_a" {
		t.Fatalf("plan order was not preserved: %s, %s", settings.Plans[0].ID, settings.Plans[1].ID)
	}
}

func TestDefaultSettingsStartsWithoutPlans(t *testing.T) {
	settings := DefaultSettings()
	if len(settings.Plans) != 0 {
		t.Fatalf("default plans length = %d, want 0", len(settings.Plans))
	}
	if got := settings.Appearance.Colors["unlimitedBadge"]; got != "#949494" {
		t.Fatalf("default unlimited badge color = %q, want #949494", got)
	}
}

func TestNormalizeAndValidateAllowsCustomPlanAndDeletion(t *testing.T) {
	settings := DefaultSettings()
	settings.Plans = []PlanSettings{
		{
			ID: "custom_2m", Enabled: true, Months: 2,
			PriceRub: 199, TrafficGB: 250, DeviceLimit: 4,
		},
	}

	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}
	if len(settings.Plans) != 1 {
		t.Fatalf("plans length = %d, want 1", len(settings.Plans))
	}
	plan := settings.Plans[0]
	if plan.ID != "custom_2m" || plan.Months != 2 || plan.TitleRU != "2 месяца" || plan.TitleEN != "2 months" {
		t.Fatalf("custom plan was not normalized: %+v", plan)
	}
}

func TestCheckoutPlansIncludesCustomPlan(t *testing.T) {
	settings := DefaultSettings()
	settings.Plans = []PlanSettings{
		{ID: "custom_4m", Enabled: true, Months: 4, PriceRub: 299, TrafficGB: 800, DeviceLimit: 8},
		{ID: "custom_8m", Enabled: true, Months: 8, PriceRub: 499, TrafficGB: 0, UnlimitedTraffic: true, DeviceLimit: 0},
	}
	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}

	service := &Service{}
	service.value.Store(settings)
	plans := service.CheckoutPlans()
	if len(plans) != 2 {
		t.Fatalf("checkout plans length = %d, want 2", len(plans))
	}
	if plans[0].ID != "custom_4m" || plans[0].Months != 4 || plans[0].TrafficLimitBytes != 800*gibibyte || plans[0].DeviceLimitCount != 8 {
		t.Fatalf("limited custom plan mismatch: %+v", plans[0])
	}
	if plans[1].ID != "custom_8m" || plans[1].Variant != "unlimited" || plans[1].TrafficLimitBytes != 0 || plans[1].DeviceLimitCount != 0 {
		t.Fatalf("unlimited custom plan mismatch: %+v", plans[1])
	}
}

func TestNormalizeAndValidateUsesNavigationDimensions(t *testing.T) {
	settings := DefaultSettings()
	for index := range settings.Layout.Elements {
		item := &settings.Layout.Elements[index]
		if item.Area == "navigation" && item.ID == "dashboard" {
			item.Width = 60
			item.Height = 50
		}
	}

	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}
	for _, item := range settings.Layout.Elements {
		if item.Area == "navigation" && item.ID == "dashboard" {
			if item.Width != 60 || item.Height != 50 {
				t.Fatalf("navigation dimensions changed: width=%g height=%d", item.Width, item.Height)
			}
			return
		}
	}
	t.Fatal("dashboard navigation element not found")
}

func TestNormalizeAndValidateAcceptsCamelCaseContentKeys(t *testing.T) {
	settings := DefaultSettings()
	settings.Content.Copy["ru"][" promoExpiresAt "] = " Valid until RU "
	settings.Content.Copy["ru"]["resumePaymentTitle"] = "Payment incomplete RU"
	settings.Content.Copy["en"] = map[string]string{}
	settings.Content.Copy["en"]["promoExpiresAt"] = "Valid until"
	settings.Content.Copy["en"]["resumePaymentTitle"] = "Payment not completed"

	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}
	if _, exists := settings.Content.Copy["ru"][" promoExpiresAt "]; exists {
		t.Fatal("content key whitespace was not normalized")
	}
	if got := settings.Content.Copy["ru"]["promoExpiresAt"]; got != "Valid until RU" {
		t.Fatalf("normalized promoExpiresAt = %q", got)
	}
	if _, exists := settings.Content.Copy["en"]; exists {
		t.Fatal("english content settings were not removed")
	}
}

func TestNormalizeAndValidateRejectsUnsafeContentKey(t *testing.T) {
	settings := DefaultSettings()
	settings.Content.Copy["ru"]["<script>"] = "bad"

	err := NormalizeAndValidate(&settings)
	if err == nil || !strings.Contains(err.Error(), "invalid content key") {
		t.Fatalf("NormalizeAndValidate() error = %v, want invalid content key", err)
	}
}

func TestNormalizeAndValidateAddsVisualEditorAreas(t *testing.T) {
	settings := DefaultSettings()
	positionX, positionY := 31.5, 72.25
	settings.Layout.Elements = []LayoutElement{
		{ID: "payments", Area: "profile", Order: 2, Visible: true, Width: 72, Height: 66, Framed: true, Align: "right", OffsetX: 12, OffsetY: -8, PositionX: &positionX, PositionY: &positionY},
	}

	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}

	required := map[string]bool{
		"dashboard:brand":         false,
		"dashboard:logo":          false,
		"dashboard:traffic":       false,
		"buy:plans":               false,
		"buy:plan_6m":             false,
		"buy:pay_button":          false,
		"support:tickets":         false,
		"support:new_ticket":      false,
		"profile:payments":        false,
		"profile:group_purchases": false,
		"navigation:buy":          false,
	}
	for _, item := range settings.Layout.Elements {
		key := item.Area + ":" + item.ID
		if _, ok := required[key]; ok {
			required[key] = true
		}
		if key == "profile:payments" {
			if item.Width != 72 || item.Height != 66 || item.Align != "right" || item.OffsetX != 12 || item.OffsetY != -8 || item.PositionX == nil || item.PositionY == nil || *item.PositionX != positionX || *item.PositionY != positionY {
				t.Fatalf("existing layout was changed: %+v", item)
			}
		}
	}
	for key, found := range required {
		if !found {
			t.Fatalf("missing default visual editor element %s", key)
		}
	}
}

func TestNormalizeAndValidateKeepsFreeEditorGeometry(t *testing.T) {
	settings := DefaultSettings()
	positionX, positionY := 1870.5, -2910.25
	for index := range settings.Layout.Elements {
		item := &settings.Layout.Elements[index]
		if item.Area == "dashboard" && item.ID == "traffic" {
			item.Width = 142
			item.Height = 680
			item.OffsetX = 870
			item.OffsetY = -910
			item.PositionX = &positionX
			item.PositionY = &positionY
		}
	}

	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}
	if settings.Version != CurrentVersion {
		t.Fatalf("version = %d, want %d", settings.Version, CurrentVersion)
	}
	for _, item := range settings.Layout.Elements {
		if item.Area == "dashboard" && item.ID == "traffic" {
			if item.Width != 142 || item.Height != 680 || item.OffsetX != 870 || item.OffsetY != -910 || item.PositionX == nil || item.PositionY == nil || *item.PositionX != positionX || *item.PositionY != -2000 {
				t.Fatalf("free editor geometry changed: %+v", item)
			}
			return
		}
	}
	t.Fatal("dashboard traffic element not found")
}

func TestNormalizeAndValidateRemovesLegacyExternalLinks(t *testing.T) {
	settings := DefaultSettings()
	settings.Content.Links["status"] = "https://example.com/status"
	settings.Content.Links["feedback"] = "https://example.com/reviews"
	settings.Content.Links["tos"] = "https://example.com/terms"

	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}

	for _, key := range []string{"status", "feedback", "tos"} {
		if _, ok := settings.Content.Links[key]; ok {
			t.Fatalf("legacy link %q was not removed", key)
		}
	}
	for _, key := range []string{"support", "channel"} {
		if _, ok := settings.Content.Links[key]; !ok {
			t.Fatalf("active link %q is missing", key)
		}
	}
}

func TestNormalizeAndValidateAddsUnlimitedBadgeColor(t *testing.T) {
	settings := DefaultSettings()
	delete(settings.Appearance.Colors, "unlimitedBadge")

	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}

	if got := settings.Appearance.Colors["unlimitedBadge"]; got != "#949494" {
		t.Fatalf("unlimitedBadge = %q, want #949494", got)
	}
}

func TestNormalizeAndValidateReminderButtonAndRussianOnlyFAQ(t *testing.T) {
	settings := DefaultSettings()
	settings.Content.SubscriptionReminderButton = TelegramButtonSettings{
		IconCustomEmojiID: `<tg-emoji emoji-id="5206222720416643915">emoji</tg-emoji>`,
		Style:             " SUCCESS ",
	}
	settings.Content.FAQ["ru"] = []FAQItem{{Question: " Question ", Answer: " Answer "}}
	settings.Content.FAQ["en"] = []FAQItem{{Question: "English", Answer: "Removed"}}

	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}
	if got := settings.Content.SubscriptionReminderButton.IconCustomEmojiID; got != "5206222720416643915" {
		t.Fatalf("iconCustomEmojiId = %q", got)
	}
	if got := settings.Content.SubscriptionReminderButton.Style; got != "success" {
		t.Fatalf("style = %q", got)
	}
	if _, exists := settings.Content.FAQ["en"]; exists {
		t.Fatal("english FAQ settings were not removed")
	}
	if got := settings.Content.FAQ["ru"]; len(got) != 1 || got[0].Question != "Question" || got[0].Answer != "Answer" {
		t.Fatalf("russian FAQ was not normalized: %#v", got)
	}
}

func TestNormalizeAndValidateRejectsReminderButtonColor(t *testing.T) {
	settings := DefaultSettings()
	settings.Content.SubscriptionReminderButton.Style = "purple"

	if err := NormalizeAndValidate(&settings); err == nil || !strings.Contains(err.Error(), "button color") {
		t.Fatalf("NormalizeAndValidate() error = %v, want button color error", err)
	}
}

func TestNormalizeAndValidateMigratesTelegramContentOnce(t *testing.T) {
	settings := DefaultSettings()
	settings.Version = CurrentVersion - 1
	settings.Content.Verification = TelegramVerificationSettings{}
	settings.Content.StartMenu = TelegramStartMenuSettings{}
	settings.Content.Commerce = TelegramCommerceSettings{}

	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}
	if settings.Content.StartImage != "" || settings.Content.Verification.Banner != "" || settings.Content.Commerce.Banner != "" || settings.Content.Commerce.SuccessBanner != "" {
		t.Fatalf("legacy content restored built-in banners: menu=%q verification=%q commerce=%q success=%q", settings.Content.StartImage, settings.Content.Verification.Banner, settings.Content.Commerce.Banner, settings.Content.Commerce.SuccessBanner)
	}
	if settings.Content.StartMenu.PlansButton.Text == "" || settings.Content.Commerce.PayButton.Text == "" {
		t.Fatal("legacy Telegram buttons were not migrated")
	}
}

func TestNormalizeAndValidateKeepsOptionalTelegramBannersEmpty(t *testing.T) {
	settings := DefaultSettings()
	settings.Content.StartImage = ""
	settings.Content.Verification.Banner = ""
	settings.Content.Commerce.Banner = ""
	settings.Content.Commerce.SuccessBanner = ""

	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}
	if settings.Content.StartImage != "" || settings.Content.Verification.Banner != "" || settings.Content.Commerce.Banner != "" || settings.Content.Commerce.SuccessBanner != "" {
		t.Fatalf("optional banners were restored unexpectedly: menu=%q verification=%q commerce=%q success=%q", settings.Content.StartImage, settings.Content.Verification.Banner, settings.Content.Commerce.Banner, settings.Content.Commerce.SuccessBanner)
	}
}

func TestNormalizeAndValidateTelegramButtonCodeAndColor(t *testing.T) {
	settings := DefaultSettings()
	settings.Content.Commerce.PayButton = TelegramButtonSettings{
		Text:              "  Оплатить  ",
		IconCustomEmojiID: `<tg-emoji emoji-id="5206401524200145033">emoji</tg-emoji>`,
		Style:             " SUCCESS ",
	}

	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}
	button := settings.Content.Commerce.PayButton
	if button.Text != "Оплатить" || button.IconCustomEmojiID != "5206401524200145033" || button.Style != "success" {
		t.Fatalf("Telegram button was not normalized: %+v", button)
	}
}
