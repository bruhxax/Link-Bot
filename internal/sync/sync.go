package sync

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"link-bot/internal/database"
	"link-bot/internal/remnawave"
	"log/slog"
	"time"
)

type subscriptionStateRepository interface {
	PrimaryPanelSyncCandidates(context.Context) ([]database.PrimaryPanelSyncCandidate, error)
	SyncPrimaryPanelObservation(context.Context, database.PrimaryPanelSyncCandidate, int64, uuid.UUID, *string, *time.Time) (bool, error)
}

type SyncService struct {
	client             *remnawave.Client
	customerRepository *database.CustomerRepository
	stateRepository    subscriptionStateRepository
}

func NewSyncService(client *remnawave.Client, customerRepository *database.CustomerRepository) *SyncService {
	return &SyncService{
		client: client, customerRepository: customerRepository, stateRepository: customerRepository,
	}
}

func (s SyncService) Sync() error {
	slog.Info("Starting sync")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	candidates, err := s.stateRepository.PrimaryPanelSyncCandidates(ctx)
	if err != nil {
		return err
	}
	var telegramIDs []int64
	var mappedUsers []database.Customer
	users, err := s.client.GetUsers(ctx)
	if err != nil {
		slog.Error("Error while getting users from remnawave", "error", err)
		return fmt.Errorf("get users from remnawave: %w", err)
	}
	if users == nil || len(*users) == 0 {
		slog.Error("No users found in remnawave")
		return fmt.Errorf("no users found in remnawave")
	}

	byTelegram := map[int64][]remnawave.PanelUser{}
	for _, user := range *users {
		if user.TelegramID != nil && *user.TelegramID > 0 {
			byTelegram[*user.TelegramID] = append(byTelegram[*user.TelegramID], user)
		}
	}
	for telegramID, group := range byTelegram {
		user := remnawave.PrimaryPanelUserForTelegram(group, telegramID)
		if user == nil {
			continue
		}
		telegramIDs = append(telegramIDs, telegramID)
		mappedUsers = append(mappedUsers, database.Customer{TelegramID: telegramID, ExpireAt: &user.ExpireAt, SubscriptionLink: &user.SubscriptionURL})
	}

	existingCustomers, err := s.customerRepository.FindByTelegramIds(ctx, telegramIDs)
	if err != nil {
		slog.Error("Error while searching users by telegram ids", "error", err)
		return fmt.Errorf("find customers by telegram ids: %w", err)
	}
	existingMap := make(map[int64]database.Customer)
	for _, cust := range existingCustomers {
		existingMap[cust.TelegramID] = cust
	}

	var toCreate []database.Customer

	for _, cust := range mappedUsers {
		if _, found := existingMap[cust.TelegramID]; !found {
			toCreate = append(toCreate, cust)
		}
	}

	err = s.customerRepository.DeleteByNotInTelegramIds(ctx, telegramIDs)
	if err != nil {
		slog.Error("Error while deleting users", "error", err)
		return fmt.Errorf("delete stale customers: %w", err)
	}
	slog.Info("Deleted clients which not exist in panel")

	if len(toCreate) > 0 {
		if err := s.customerRepository.CreateBatch(ctx, toCreate); err != nil {
			slog.Error("Error while creating users", "error", err)
			return fmt.Errorf("create customers: %w", err)
		} else {
			slog.Info("Created clients", "count", len(toCreate))
		}
	}

	if err := s.refreshSubscriptionSnapshots(ctx, candidates, *users); err != nil {
		return err
	}

	slog.Info("Synchronization completed")
	return nil
}

// RefreshSubscriptionState updates notification-critical subscription data
// without deleting bot customers that are not currently present in Remnawave.
func (s SyncService) RefreshSubscriptionState() error {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	candidates, err := s.stateRepository.PrimaryPanelSyncCandidates(ctx)
	if err != nil {
		return err
	}
	users, err := s.client.GetUsers(ctx)
	if err != nil {
		return fmt.Errorf("get users from remnawave: %w", err)
	}
	if users == nil || len(*users) == 0 {
		return fmt.Errorf("no users found in remnawave")
	}
	return s.refreshSubscriptionSnapshots(ctx, candidates, *users)
}

func (s SyncService) refreshSubscriptionSnapshots(ctx context.Context, candidates []database.PrimaryPanelSyncCandidate, users []remnawave.PanelUser) error {
	byID := map[int64]*remnawave.PanelUser{}
	byUUID := map[uuid.UUID]*remnawave.PanelUser{}
	byTelegram := map[int64][]remnawave.PanelUser{}
	for i := range users {
		user := &users[i]
		if user.ID > 0 {
			byID[user.ID] = user
		}
		if user.UUID != uuid.Nil {
			byUUID[user.UUID] = user
		}
		if user.TelegramID != nil {
			byTelegram[*user.TelegramID] = append(byTelegram[*user.TelegramID], *user)
		}
	}
	updated := 0
	for _, candidate := range candidates {
		subscription := candidate.Subscription
		var user *remnawave.PanelUser
		switch {
		case subscription.PanelUserUUID != nil && *subscription.PanelUserUUID != uuid.Nil:
			user = byUUID[*subscription.PanelUserUUID]
			if user == nil && subscription.PanelUserID != nil {
				if numeric := byID[*subscription.PanelUserID]; numeric != nil && numeric.UUID == uuid.Nil {
					user = numeric
				}
			}
		case subscription.PanelUserID != nil && *subscription.PanelUserID > 0:
			user = byID[*subscription.PanelUserID]
		default:
			group := byTelegram[candidate.Customer.TelegramID]
			// Imported subscriptions can already have a reliable link before the
			// panel ID is stored. Prefer that link over ambiguous Telegram matches.
			for i := range group {
				if subscription.SubscriptionLink != nil && group[i].SubscriptionURL == *subscription.SubscriptionLink {
					user = &group[i]
					break
				}
			}
			if user == nil {
				user = remnawave.PrimaryPanelUserForTelegram(group, candidate.Customer.TelegramID)
			}
		}
		// Catalog omissions are not proof of deletion. Interactive identity reads
		// can confirm deletion without attaching a different Telegram subscription.
		if user == nil || user.ExpireAt.IsZero() {
			continue
		}
		if panelObservationUnchanged(candidate, user) {
			continue
		}
		link := user.SubscriptionURL
		expire := user.ExpireAt.UTC()
		applied, err := s.stateRepository.SyncPrimaryPanelObservation(ctx, candidate, user.ID, user.UUID, &link, &expire)
		if err != nil {
			return fmt.Errorf("sync primary subscription %d: %w", subscription.ID, err)
		}
		if applied {
			updated++
		}
	}
	slog.Info("Subscription state refreshed", "checked", updated)
	return nil
}

func panelObservationUnchanged(candidate database.PrimaryPanelSyncCandidate, user *remnawave.PanelUser) bool {
	s := candidate.Subscription
	if s.PanelUserID == nil || *s.PanelUserID != user.ID {
		return false
	}
	if (s.PanelUserUUID == nil && user.UUID != uuid.Nil) || (s.PanelUserUUID != nil && *s.PanelUserUUID != user.UUID) {
		return false
	}
	for _, expiry := range []*time.Time{candidate.Customer.ExpireAt, s.ExpireAt} {
		if expiry == nil || !expiry.Equal(user.ExpireAt) {
			return false
		}
	}
	for _, link := range []*string{candidate.Customer.SubscriptionLink, s.SubscriptionLink} {
		if link == nil || *link != user.SubscriptionURL {
			return false
		}
	}
	return true
}
