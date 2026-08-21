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
	if settings.Grace.Enabled || settings.Grace.Days != 1 || len(settings.Grace.InternalSquadUUIDs) != 0 {
		t.Fatalf("unexpected default grace settings: %+v", settings.Grace)
	}
	if len(settings.DevicePacks) != 3 {
		t.Fatalf("default device packs length = %d, want 3", len(settings.DevicePacks))
	}
	if settings.Panel.UsernameTemplate != "{{customer_id}}_{{telegram_id}}" {
		t.Fatalf("default username template = %q", settings.Panel.UsernameTemplate)
	}
}

func TestNormalizeAndValidateDevicePacks(t *testing.T) {
	settings := DefaultSettings()
	settings.DevicePacks = []DevicePackSettings{
		{ID: " pack_5 ", Devices: 5, PriceRub: 149},
		{ID: "pack_10", Devices: 10, PriceRub: 249},
	}

	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}
	if len(settings.DevicePacks) != 2 || settings.DevicePacks[0].ID != "pack_5" {
		t.Fatalf("unexpected device packs: %+v", settings.DevicePacks)
	}
	if settings.DevicePacks[0].PriceStars < 1 {
		t.Fatalf("device pack Stars price was not derived: %+v", settings.DevicePacks[0])
	}
}

func TestNormalizeAndValidateRejectsInvalidDevicePack(t *testing.T) {
	settings := DefaultSettings()
	settings.DevicePacks = []DevicePackSettings{{ID: "pack_0", Devices: 0, PriceRub: 100}}

	if err := NormalizeAndValidate(&settings); err == nil || !strings.Contains(err.Error(), "invalid device count") {
		t.Fatalf("NormalizeAndValidate() error = %v, want invalid device count", err)
	}
}

func TestNormalizeAndValidateMigratesDevicePackDefaultsAndNotifications(t *testing.T) {
	settings := DefaultSettings()
	settings.Version = CurrentVersion - 1
	settings.DevicePacks = nil
	settings.Content.Copy = map[string]map[string]string{"ru": {}}
	settings.Trial = TrialSettings{
		Enabled:              true,
		Days:                 9,
		TrafficGB:            77,
		DeviceLimit:          4,
		TrafficResetStrategy: "WEEK",
		Tag:                  "custom-trial",
	}

	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}
	if len(settings.DevicePacks) != 3 {
		t.Fatalf("migrated device packs length = %d, want 3", len(settings.DevicePacks))
	}
	for _, key := range []string{"deviceAddedTemplate", "deviceLimitReachedTemplate", "reviewCreatedTemplate"} {
		if strings.TrimSpace(settings.Content.Copy["ru"][key]) == "" {
			t.Fatalf("migrated notification %q is empty", key)
		}
	}
	if !settings.Trial.Enabled || settings.Trial.Days != 9 || settings.Trial.TrafficGB != 77 || settings.Trial.DeviceLimit != 4 || settings.Trial.TrafficResetStrategy != "WEEK" || settings.Trial.Tag != "custom-trial" {
		t.Fatalf("custom trial settings changed during migration: %+v", settings.Trial)
	}
}

func TestNormalizeAndValidatePanelUsernameTemplate(t *testing.T) {
	settings := DefaultSettings()
	settings.Panel.UsernameTemplate = "tg_{{telegram_id}}"
	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}
	if settings.Panel.UsernameTemplate != "tg_{{telegram_id}}" {
		t.Fatalf("username template = %q", settings.Panel.UsernameTemplate)
	}
}

func TestNormalizeAndValidateGraceAccess(t *testing.T) {
	settings := DefaultSettings()
	settings.Grace = GraceSettings{
		Enabled:            true,
		Days:               2,
		InternalSquadUUIDs: []string{"7d3258cf-2b39-4ad0-8b11-fbcd30d76348"},
	}

	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}
	if !settings.Grace.Enabled || settings.Grace.Days != 2 || len(settings.Grace.InternalSquadUUIDs) != 1 {
		t.Fatalf("unexpected normalized grace settings: %+v", settings.Grace)
	}
}

func TestNormalizeAndValidateRejectsGraceWithoutSquad(t *testing.T) {
	settings := DefaultSettings()
	settings.Grace = GraceSettings{Enabled: true, Days: 2}

	if err := NormalizeAndValidate(&settings); err == nil || !strings.Contains(err.Error(), "requires at least one internal squad") {
		t.Fatalf("NormalizeAndValidate() error = %v, want missing squad error", err)
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

func TestCheckoutPlansIncludesFreePlanAndNormalizesOneTimeRule(t *testing.T) {
	settings := DefaultSettings()
	settings.Plans = []PlanSettings{
		{ID: "free_1m", Enabled: true, Months: 1, PriceRub: 0, FreeOneTime: true, TrafficGB: 100, DeviceLimit: 2},
		{ID: "paid_1m", Enabled: true, Months: 1, PriceRub: 100, FreeOneTime: true, TrafficGB: 100, DeviceLimit: 2},
	}
	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}
	if settings.Plans[1].FreeOneTime {
		t.Fatal("paid plan must not keep the free one-time rule")
	}

	service := &Service{}
	service.value.Store(settings)
	plans := service.CheckoutPlans()
	if len(plans) != 2 {
		t.Fatalf("checkout plans length = %d, want 2", len(plans))
	}
	if plans[0].PriceRub != 0 || plans[0].PriceStars != 0 || !plans[0].FreeOneTime {
		t.Fatalf("free checkout plan mismatch: %+v", plans[0])
	}
}

func TestCheckoutPlansPreservesExplicitEmptySquadSelection(t *testing.T) {
	settings := DefaultSettings()
	settings.Plans = []PlanSettings{
		{ID: "no_squads", Enabled: true, Months: 1, InternalSquadsConfigured: true},
	}
	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}

	service := &Service{}
	service.value.Store(settings)
	plans := service.CheckoutPlans()
	if len(plans) != 1 || !plans[0].InternalSquadsConfigured || len(plans[0].InternalSquadUUIDs) != 0 {
		t.Fatalf("explicit empty squad selection was not preserved: %+v", plans)
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

func TestParseTelegramChannelChatID(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want any
		ok   bool
	}{
		{name: "public url", raw: "https://t.me/link_bot_news", want: "@link_bot_news", ok: true},
		{name: "public username", raw: "@link_bot_news", want: "@link_bot_news", ok: true},
		{name: "private numeric", raw: "-1001234567890", want: int64(-1001234567890), ok: true},
		{name: "empty", raw: "", ok: false},
		{name: "invite url requires numeric id", raw: "https://t.me/+invite", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseTelegramChannelChatID(tt.raw)
			if ok != tt.ok {
				t.Fatalf("ParseTelegramChannelChatID(%q) ok = %v, want %v", tt.raw, ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Fatalf("ParseTelegramChannelChatID(%q) = %#v, want %#v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestNormalizeAndValidateSupportAndVerificationContent(t *testing.T) {
	settings := DefaultSettings()
	settings.Content.Links["channel"] = "https://t.me/link_bot_news"
	settings.Content.Verification.ChannelChatID = "-1001234567890"
	settings.Content.Support.NewTicketText = "Ticket {ticket_id}: {message}"
	settings.Content.Support.OpenButton.Text = "Open support"

	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}
	if got := settings.Content.Verification.ChannelChatID; got != "-1001234567890" {
		t.Fatalf("channel chat id = %q", got)
	}
	if got := settings.Content.Support.NewTicketText; got != "Ticket {ticket_id}: {message}" {
		t.Fatalf("support template = %q", got)
	}
	if got := settings.Content.Support.OpenButton.Text; got != "Open support" {
		t.Fatalf("support button = %q", got)
	}
}

func TestNormalizeAndValidateAllowsDisablingChannelVerification(t *testing.T) {
	settings := DefaultSettings()
	settings.Version = CurrentVersion
	settings.Content.Links["channel"] = ""
	settings.Content.Verification.ChannelChatID = ""

	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}
	if got := settings.Content.Links["channel"]; got != "" {
		t.Fatalf("disabled channel link = %q, want empty", got)
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

func TestNormalizeAndValidateRemovesDashboardWrappers(t *testing.T) {
	settings := DefaultSettings()
	settings.Layout.Elements = append(settings.Layout.Elements,
		LayoutElement{ID: "brand", Area: "dashboard", Visible: true},
		LayoutElement{ID: "subscription", Area: "dashboard", Visible: true},
		LayoutElement{ID: "actions", Area: "dashboard", Visible: true},
	)

	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}
	for _, item := range settings.Layout.Elements {
		if item.Area == "dashboard" && contains([]string{"brand", "subscription", "actions"}, item.ID) {
			t.Fatalf("legacy dashboard wrapper remains: %+v", item)
		}
	}
}

func TestNormalizeAndValidateGridAppearanceAndAdminContact(t *testing.T) {
	settings := DefaultSettings()
	settings.Appearance.BackgroundMode = "grid"
	settings.Content.AdminContact = "https://t.me/link_bot_admin"
	settings.Features["yookassa"] = true
	settings.Features["crypto"] = true

	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}
	if settings.Content.AdminContact != "@link_bot_admin" {
		t.Fatalf("admin contact = %q", settings.Content.AdminContact)
	}
	for _, key := range []string{"yookassa", "crypto"} {
		if _, exists := settings.Features[key]; exists {
			t.Fatalf("legacy integration feature %q was not removed", key)
		}
	}
	for _, key := range []string{"gridBackground", "gridLine", "gridGlowLeft", "gridGlowRight", "grid2Background", "grid2Line", "grid2Glow", "waveBackground", "waveDot"} {
		if settings.Appearance.Colors[key] == "" {
			t.Fatalf("grid color %q is empty", key)
		}
	}
}

func TestNormalizeAndValidateGrid2Appearance(t *testing.T) {
	settings := DefaultSettings()
	settings.Appearance.BackgroundMode = "grid2"

	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}
	if settings.Appearance.BackgroundMode != "grid2" {
		t.Fatalf("background mode = %q, want grid2", settings.Appearance.BackgroundMode)
	}
	for _, key := range []string{"grid2Background", "grid2Line", "grid2Glow"} {
		if settings.Appearance.Colors[key] == "" {
			t.Fatalf("grid2 color %q is empty", key)
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

func TestNormalizeAndValidateAddsProfileFeatureFlags(t *testing.T) {
	settings := DefaultSettings()
	settings.Version = CurrentVersion - 1
	settings.Features["reviews"] = false
	for _, name := range []string{"payments_history", "news", "login_methods", "terms", "privacy"} {
		delete(settings.Features, name)
	}

	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}

	for _, name := range []string{"payments_history", "news", "login_methods", "terms", "privacy"} {
		if !settings.Features[name] {
			t.Fatalf("migrated feature %q is disabled", name)
		}
	}
	if settings.Features["reviews"] {
		t.Fatal("existing disabled feature was enabled during migration")
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

func TestNormalizeAndValidateMigratesProfileEditorSettings(t *testing.T) {
	settings := DefaultSettings()
	settings.Version = CurrentVersion - 1
	settings.Layout.Elements = filterLayoutElement(settings.Layout.Elements, "profile", "privacy")
	settings.Content.ProfileButtons = nil
	settings.Content.LegalDocuments = nil
	settings.Content.CustomLinks = []CustomLink{
		{
			ID:      "legacy_link",
			LabelRU: "Legacy",
			HintRU:  "Existing profile link",
			URL:     "https://example.com/legacy",
			Icon:    "external",
		},
	}

	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}
	if settings.Version != CurrentVersion {
		t.Fatalf("version = %d, want %d", settings.Version, CurrentVersion)
	}
	if settings.Content.ProfileButtons == nil || settings.Content.LegalDocuments == nil {
		t.Fatal("profile editor maps were not initialized")
	}
	if got := settings.Content.CustomLinks[0].Type; got != "url" {
		t.Fatalf("legacy custom link type = %q, want url", got)
	}
	if !hasLayoutElement(settings.Layout.Elements, "profile", "privacy") {
		t.Fatal("privacy profile item was not added during migration")
	}
	if !hasLayoutElement(settings.Layout.Elements, "profile", "custom.legacy_link") {
		t.Fatal("legacy custom profile button was not added to the layout")
	}
}

func TestDefaultSettingsUseCompactProfileRows(t *testing.T) {
	settings := DefaultSettings()
	for _, element := range settings.Layout.Elements {
		if element.Area == "profile" && element.ID == "server_status" {
			if element.Height != 48 {
				t.Fatalf("profile row height = %d, want 48", element.Height)
			}
			return
		}
	}
	t.Fatal("default server status profile row was not found")
}

func TestNormalizeAndValidateProfileButtonAndLegalPage(t *testing.T) {
	settings := DefaultSettings()
	settings.Content.ProfileButtons["web_version"] = ProfileButtonSettings{
		LabelRU: " Web ",
		HintRU:  " Browser ",
		URL:     "https://example.com/account",
	}
	settings.Content.LegalDocuments["privacy"] = LegalDocumentSettings{
		Title:          " Privacy ",
		EffectiveLabel: " Effective ",
		Jurisdiction:   " Test jurisdiction ",
		Intro:          " Intro ",
		Sections: []LegalDocumentSection{
			{Title: " Data ", Body: " Details "},
			{},
		},
		ContactTitle: " Contact ",
		Contacts:     " @admin ",
		Footer:       " Footer ",
	}
	settings.Content.CustomLinks = []CustomLink{
		{
			ID:      "custom_policy",
			LabelRU: " Custom policy ",
			HintRU:  " Details ",
			Type:    "page",
			Document: LegalDocumentSettings{
				Title:    " Custom page ",
				Sections: []LegalDocumentSection{{Title: " Section ", Body: " Body "}},
			},
		},
	}

	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}
	button := settings.Content.ProfileButtons["web_version"]
	if button.LabelRU != "Web" || button.HintRU != "Browser" || button.URL != "https://example.com/account" {
		t.Fatalf("profile button was not normalized: %+v", button)
	}
	privacy := settings.Content.LegalDocuments["privacy"]
	if privacy.Title != "Privacy" || privacy.EffectiveLabel != "Effective" || len(privacy.Sections) != 1 || privacy.Sections[0].Body != "Details" {
		t.Fatalf("privacy document was not normalized: %+v", privacy)
	}
	custom := settings.Content.CustomLinks[0]
	if custom.Type != "page" || custom.URL != "" || custom.Document.Title != "Custom page" {
		t.Fatalf("custom page was not normalized: %+v", custom)
	}
}

func TestNormalizeAndValidateRejectsUnsafeProfileURL(t *testing.T) {
	settings := DefaultSettings()
	settings.Content.ProfileButtons["web_version"] = ProfileButtonSettings{
		LabelRU: "Web",
		URL:     "javascript:alert(1)",
	}

	err := NormalizeAndValidate(&settings)
	if err == nil || !strings.Contains(err.Error(), "profile button URL") {
		t.Fatalf("NormalizeAndValidate() error = %v, want profile button URL error", err)
	}
}

func TestNormalizeAndValidateRejectsUnknownProfileButton(t *testing.T) {
	settings := DefaultSettings()
	settings.Content.ProfileButtons["unknown"] = ProfileButtonSettings{LabelRU: "Unknown"}

	err := NormalizeAndValidate(&settings)
	if err == nil || !strings.Contains(err.Error(), "invalid profile button") {
		t.Fatalf("NormalizeAndValidate() error = %v, want invalid profile button error", err)
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

func filterLayoutElement(items []LayoutElement, area, id string) []LayoutElement {
	result := make([]LayoutElement, 0, len(items))
	for _, item := range items {
		if item.Area == area && item.ID == id {
			continue
		}
		result = append(result, item)
	}
	return result
}

func hasLayoutElement(items []LayoutElement, area, id string) bool {
	for _, item := range items {
		if item.Area == area && item.ID == id {
			return true
		}
	}
	return false
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
