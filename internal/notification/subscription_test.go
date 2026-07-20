package notification

import (
	"context"
	"errors"
	"testing"
	"time"

	"link-bot/internal/database"
)

type customerRepoMock struct {
	customers  *[]database.Customer
	err        error
	claims     map[notificationClaim]bool
	claimErr   error
	releaseErr error
}

type notificationClaim struct {
	customerID int64
	expireAt   int64
	kind       string
}

func (m *customerRepoMock) FindByExpirationRange(ctx context.Context, startDate, endDate time.Time) (*[]database.Customer, error) {
	if m.err != nil || m.customers == nil {
		return m.customers, m.err
	}
	filtered := make([]database.Customer, 0, len(*m.customers))
	for _, customer := range *m.customers {
		if customer.ExpireAt != nil && !customer.ExpireAt.Before(startDate) && !customer.ExpireAt.After(endDate) {
			filtered = append(filtered, customer)
		}
	}
	return &filtered, nil
}

func (m *customerRepoMock) ClaimSubscriptionNotification(ctx context.Context, customerID int64, expireAt time.Time, kind string) (bool, error) {
	if m.claimErr != nil {
		return false, m.claimErr
	}
	if m.claims == nil {
		m.claims = make(map[notificationClaim]bool)
	}
	claim := notificationClaim{customerID: customerID, expireAt: expireAt.UnixNano(), kind: kind}
	if m.claims[claim] {
		return false, nil
	}
	m.claims[claim] = true
	return true, nil
}

func (m *customerRepoMock) ReleaseSubscriptionNotification(ctx context.Context, customerID int64, expireAt time.Time, kind string) error {
	if m.releaseErr != nil {
		return m.releaseErr
	}
	delete(m.claims, notificationClaim{customerID: customerID, expireAt: expireAt.UnixNano(), kind: kind})
	return nil
}

func TestSubscriptionService_ProcessSubscriptionExpiration_NotifiesRecentlyExpiredCustomer(t *testing.T) {
	expireAt := time.Now().Add(-2 * time.Hour)
	customers := []database.Customer{{ID: 10, ExpireAt: &expireAt}}
	tributes := []database.Purchase{}

	svc := NewSubscriptionService(
		&customerRepoMock{customers: &customers},
		&purchaseRepoMock{tributes: &tributes},
		&paymentServiceMock{},
		nil,
		nil,
		nil,
	)
	notifyCalls := 0
	svc.notify = func(ctx context.Context, customer database.Customer) error {
		notifyCalls++
		return nil
	}

	if err := svc.ProcessSubscriptionExpiration(); err != nil {
		t.Fatalf("ProcessSubscriptionExpiration returned error: %v", err)
	}
	if notifyCalls != 1 {
		t.Fatalf("expected one expired notification, got %d", notifyCalls)
	}
}

func TestSubscriptionService_ProcessSubscriptionExpiration_DoesNotSendDuplicate(t *testing.T) {
	expireAt := time.Now().Add(-2 * time.Minute)
	customers := []database.Customer{{ID: 11, ExpireAt: &expireAt}}
	tributes := []database.Purchase{}
	repo := &customerRepoMock{customers: &customers}
	svc := NewSubscriptionService(repo, &purchaseRepoMock{tributes: &tributes}, &paymentServiceMock{}, nil, nil, nil)
	notifyCalls := 0
	svc.notify = func(ctx context.Context, customer database.Customer) error {
		notifyCalls++
		return nil
	}

	if err := svc.ProcessSubscriptionExpiration(); err != nil {
		t.Fatalf("first expiration check failed: %v", err)
	}
	if err := svc.ProcessSubscriptionExpiration(); err != nil {
		t.Fatalf("second expiration check failed: %v", err)
	}
	if notifyCalls != 1 {
		t.Fatalf("expected one deduplicated notification, got %d", notifyCalls)
	}
}

func TestSubscriptionService_ProcessSubscriptionExpiration_RetriesAfterSendFailure(t *testing.T) {
	expireAt := time.Now().Add(-2 * time.Minute)
	customers := []database.Customer{{ID: 12, ExpireAt: &expireAt}}
	tributes := []database.Purchase{}
	repo := &customerRepoMock{customers: &customers}
	svc := NewSubscriptionService(repo, &purchaseRepoMock{tributes: &tributes}, &paymentServiceMock{}, nil, nil, nil)
	notifyCalls := 0
	svc.notify = func(ctx context.Context, customer database.Customer) error {
		notifyCalls++
		if notifyCalls == 1 {
			return errors.New("temporary telegram failure")
		}
		return nil
	}

	if err := svc.ProcessSubscriptionExpiration(); err != nil {
		t.Fatalf("first expiration check failed: %v", err)
	}
	if err := svc.ProcessSubscriptionExpiration(); err != nil {
		t.Fatalf("second expiration check failed: %v", err)
	}
	if notifyCalls != 2 {
		t.Fatalf("expected retry after failed delivery, got %d calls", notifyCalls)
	}
}

func TestSubscriptionService_ProcessSubscriptionExpiration_DoesNotRetryPermanentFailure(t *testing.T) {
	expireAt := time.Now().Add(-2 * time.Minute)
	customers := []database.Customer{{ID: 13, ExpireAt: &expireAt}}
	tributes := []database.Purchase{}
	repo := &customerRepoMock{customers: &customers}
	svc := NewSubscriptionService(repo, &purchaseRepoMock{tributes: &tributes}, &paymentServiceMock{}, nil, nil, nil)
	notifyCalls := 0
	svc.notify = func(ctx context.Context, customer database.Customer) error {
		notifyCalls++
		return errors.New("forbidden, Forbidden: bot was blocked by the user")
	}

	if err := svc.ProcessSubscriptionExpiration(); err != nil {
		t.Fatalf("first expiration check failed: %v", err)
	}
	if err := svc.ProcessSubscriptionExpiration(); err != nil {
		t.Fatalf("second expiration check failed: %v", err)
	}
	if notifyCalls != 1 {
		t.Fatalf("expected permanent delivery failure not to be retried, got %d calls", notifyCalls)
	}
}

func TestMiniAppTariffsURL(t *testing.T) {
	got := miniAppTariffsURL("https://example.com/mini-app/?source=notification")
	want := "https://example.com/mini-app/?page=buy&source=notification"
	if got != want {
		t.Fatalf("unexpected tariffs URL: got %q want %q", got, want)
	}
}

func TestBuildRenewKeyboardWithPremiumEmojiAndColor(t *testing.T) {
	keyboard := buildRenewKeyboardWithOptions(reminderButtonOptions{
		Text:              "Продлить",
		IconCustomEmojiID: "5206222720416643915",
		Style:             "success",
	})
	if len(keyboard) != 1 || len(keyboard[0]) != 1 {
		t.Fatalf("unexpected keyboard shape: %#v", keyboard)
	}
	button := keyboard[0][0]
	if button.Text != "Продлить" || button.IconCustomEmojiID != "5206222720416643915" || button.Style != "success" {
		t.Fatalf("unexpected reminder button: %#v", button)
	}
}

func TestResolveTestReminderButtonTextUsesSafeFallback(t *testing.T) {
	if got := resolveTestReminderButtonText("", nil, nil); got != "Продлить подписку" {
		t.Fatalf("unexpected fallback button text: %q", got)
	}
	if got := resolveTestReminderButtonText("  Мой тариф  ", nil, nil); got != "Мой тариф" {
		t.Fatalf("unexpected custom button text: %q", got)
	}
}

type purchaseRepoMock struct {
	tributes    *[]database.Purchase
	err         error
	receivedIDs []int64
}

func (m *purchaseRepoMock) FindLatestActiveTributesByCustomerIDs(ctx context.Context, customerIDs []int64) (*[]database.Purchase, error) {
	m.receivedIDs = append([]int64(nil), customerIDs...)
	return m.tributes, m.err
}

type paymentServiceMock struct {
	createCalls        int
	processCalls       int
	amounts            []float64
	months             []int
	customers          []int64
	processIDs         []int64
	createErr          error
	processErr         error
	purchaseIDToReturn int64
}

func (m *paymentServiceMock) CreatePurchase(ctx context.Context, amount float64, months int, customer *database.Customer, invoiceType database.InvoiceType) (string, int64, error) {
	m.createCalls++
	m.amounts = append(m.amounts, amount)
	m.months = append(m.months, months)
	if customer != nil {
		m.customers = append(m.customers, customer.ID)
	}
	if m.purchaseIDToReturn == 0 {
		m.purchaseIDToReturn = int64(m.createCalls)
	}
	return "", m.purchaseIDToReturn, m.createErr
}

func (m *paymentServiceMock) ProcessPurchaseById(ctx context.Context, purchaseId int64) error {
	m.processCalls++
	m.processIDs = append(m.processIDs, purchaseId)
	return m.processErr
}

func TestSubscriptionService_ProcessSubscriptionExpiration_ProcessesTribute(t *testing.T) {
	expireAt := time.Now().Add(24 * time.Hour)
	customers := []database.Customer{{ID: 1, ExpireAt: &expireAt}}
	tributes := []database.Purchase{{CustomerID: 1, Amount: 10.5, Month: 2}}

	cRepo := &customerRepoMock{customers: &customers}
	pRepo := &purchaseRepoMock{tributes: &tributes}
	payMock := &paymentServiceMock{purchaseIDToReturn: 77}

	svc := NewSubscriptionService(cRepo, pRepo, payMock, nil, nil, nil)
	svc.notify = func(ctx context.Context, customer database.Customer) error {
		t.Fatalf("sendNotification should not be called in successful tribute processing scenario")
		return nil
	}

	if err := svc.ProcessSubscriptionExpiration(); err != nil {
		t.Fatalf("ProcessSubscriptionExpiration returned error: %v", err)
	}

	if payMock.createCalls != 1 {
		t.Fatalf("expected create purchase to be called once, got %d", payMock.createCalls)
	}
	if payMock.processCalls != 1 {
		t.Fatalf("expected process purchase to be called once, got %d", payMock.processCalls)
	}
	if len(payMock.amounts) != 1 || payMock.amounts[0] != tributes[0].Amount {
		t.Fatalf("unexpected amount used for purchase: %#v", payMock.amounts)
	}
	if len(payMock.months) != 1 || payMock.months[0] != tributes[0].Month {
		t.Fatalf("unexpected months used for purchase: %#v", payMock.months)
	}
	if len(payMock.processIDs) != 1 || payMock.processIDs[0] != payMock.purchaseIDToReturn {
		t.Fatalf("expected process to be called with purchase id %d, got %#v", payMock.purchaseIDToReturn, payMock.processIDs)
	}
	if len(pRepo.receivedIDs) != 1 || pRepo.receivedIDs[0] != customers[0].ID {
		t.Fatalf("expected purchase repository to query by customer id %d, got %#v", customers[0].ID, pRepo.receivedIDs)
	}
}

func TestSubscriptionService_ProcessSubscriptionExpiration_SkipsAutoRenewWhenNotOneDay(t *testing.T) {
	expireAt := time.Now().Add(48 * time.Hour)
	customers := []database.Customer{{ID: 5, ExpireAt: &expireAt}}
	tributes := []database.Purchase{{CustomerID: 5, Amount: 20, Month: 1}}

	cRepo := &customerRepoMock{customers: &customers}
	pRepo := &purchaseRepoMock{tributes: &tributes}
	payMock := &paymentServiceMock{purchaseIDToReturn: 101}

	svc := NewSubscriptionService(cRepo, pRepo, payMock, nil, nil, nil)
	svc.notify = func(ctx context.Context, customer database.Customer) error {
		t.Fatalf("sendNotification should not be called when auto-renew is skipped due to days remaining")
		return nil
	}

	if err := svc.ProcessSubscriptionExpiration(); err != nil {
		t.Fatalf("ProcessSubscriptionExpiration returned error: %v", err)
	}

	if payMock.createCalls != 0 {
		t.Fatalf("expected create purchase not to be called, got %d", payMock.createCalls)
	}
	if payMock.processCalls != 0 {
		t.Fatalf("expected process purchase not to be called, got %d", payMock.processCalls)
	}
	if len(pRepo.receivedIDs) != 1 || pRepo.receivedIDs[0] != customers[0].ID {
		t.Fatalf("expected purchase repository to be queried with customer id %d, got %#v", customers[0].ID, pRepo.receivedIDs)
	}
}

func TestSubscriptionService_ProcessSubscriptionExpiration_SkipsAutoRenewWhenLastTributeCancelled(t *testing.T) {
	expireAt := time.Now().Add(24 * time.Hour)
	customers := []database.Customer{{ID: 9, ExpireAt: &expireAt}}
	tributes := []database.Purchase{}

	cRepo := &customerRepoMock{customers: &customers}
	pRepo := &purchaseRepoMock{tributes: &tributes}
	payMock := &paymentServiceMock{}
	notifyCalls := 0

	svc := NewSubscriptionService(cRepo, pRepo, payMock, nil, nil, nil)
	svc.notify = func(ctx context.Context, customer database.Customer) error {
		notifyCalls++
		return nil
	}

	if err := svc.ProcessSubscriptionExpiration(); err != nil {
		t.Fatalf("ProcessSubscriptionExpiration returned error: %v", err)
	}

	if payMock.createCalls != 0 {
		t.Fatalf("expected create purchase not to be called, got %d", payMock.createCalls)
	}
	if payMock.processCalls != 0 {
		t.Fatalf("expected process purchase not to be called, got %d", payMock.processCalls)
	}
	if notifyCalls != 1 {
		t.Fatalf("expected notification to be sent once, got %d", notifyCalls)
	}
	if len(pRepo.receivedIDs) != 1 || pRepo.receivedIDs[0] != customers[0].ID {
		t.Fatalf("expected purchase repository to query by customer id %d, got %#v", customers[0].ID, pRepo.receivedIDs)
	}
}
