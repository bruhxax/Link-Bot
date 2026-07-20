package sync

import (
	"context"
	"fmt"
	"log/slog"
	"link-bot/internal/database"
	"link-bot/internal/remnawave"
)

type SyncService struct {
	client             *remnawave.Client
	customerRepository *database.CustomerRepository
}

func NewSyncService(client *remnawave.Client, customerRepository *database.CustomerRepository) *SyncService {
	return &SyncService{
		client: client, customerRepository: customerRepository,
	}
}

func (s SyncService) Sync() error {
	slog.Info("Starting sync")
	ctx := context.Background()
	var telegramIDs []int64
	telegramIDsSet := make(map[int64]int64)
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

	for _, user := range *users {
		if user.TelegramId.Null {
			continue
		}
		if _, exists := telegramIDsSet[int64(user.TelegramId.Value)]; exists {
			continue
		}

		telegramIDsSet[int64(user.TelegramId.Value)] = int64(user.TelegramId.Value)

		telegramIDs = append(telegramIDs, int64(user.TelegramId.Value))

		mappedUsers = append(mappedUsers, database.Customer{
			TelegramID:       int64(user.TelegramId.Value),
			ExpireAt:         &user.ExpireAt,
			SubscriptionLink: &user.SubscriptionUrl,
		})
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
	var toUpdate []database.Customer

	for _, cust := range mappedUsers {
		if existing, found := existingMap[cust.TelegramID]; found {
			cust.ID = existing.ID
			cust.CreatedAt = existing.CreatedAt
			cust.Language = existing.Language
			toUpdate = append(toUpdate, cust)
		} else {
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

	if len(toUpdate) > 0 {
		if err := s.customerRepository.UpdateBatch(ctx, toUpdate); err != nil {
			slog.Error("Error while updating users", "error", err)
			return fmt.Errorf("update customers: %w", err)
		} else {
			slog.Info("Updated clients", "count", len(toUpdate))
		}
	}
	slog.Info("Synchronization completed")
	return nil
}

// RefreshSubscriptionState updates notification-critical subscription data
// without deleting bot customers that are not currently present in Remnawave.
func (s SyncService) RefreshSubscriptionState() error {
	slog.Info("Refreshing subscription state")
	ctx := context.Background()
	users, err := s.client.GetUsers(ctx)
	if err != nil {
		return fmt.Errorf("get users from remnawave: %w", err)
	}
	if users == nil || len(*users) == 0 {
		return fmt.Errorf("no users found in remnawave")
	}

	telegramIDs := make([]int64, 0, len(*users))
	mappedByTelegramID := make(map[int64]database.Customer, len(*users))
	for _, user := range *users {
		if user.TelegramId.Null {
			continue
		}
		telegramID := int64(user.TelegramId.Value)
		if _, exists := mappedByTelegramID[telegramID]; exists {
			continue
		}
		telegramIDs = append(telegramIDs, telegramID)
		mappedByTelegramID[telegramID] = database.Customer{
			TelegramID:       telegramID,
			ExpireAt:         &user.ExpireAt,
			SubscriptionLink: &user.SubscriptionUrl,
		}
	}
	if len(telegramIDs) == 0 {
		return fmt.Errorf("no users with telegram ids found in remnawave")
	}

	existingCustomers, err := s.customerRepository.FindByTelegramIds(ctx, telegramIDs)
	if err != nil {
		return fmt.Errorf("find customers by telegram ids: %w", err)
	}
	toUpdate := make([]database.Customer, 0, len(existingCustomers))
	for _, existing := range existingCustomers {
		mapped, ok := mappedByTelegramID[existing.TelegramID]
		if !ok {
			continue
		}
		expireAtUnchanged := existing.ExpireAt != nil && mapped.ExpireAt != nil && existing.ExpireAt.Equal(*mapped.ExpireAt)
		linkUnchanged := existing.SubscriptionLink != nil && mapped.SubscriptionLink != nil && *existing.SubscriptionLink == *mapped.SubscriptionLink
		if expireAtUnchanged && linkUnchanged {
			continue
		}
		mapped.ID = existing.ID
		toUpdate = append(toUpdate, mapped)
	}
	if err := s.customerRepository.UpdateBatch(ctx, toUpdate); err != nil {
		return fmt.Errorf("update subscription state: %w", err)
	}

	slog.Info("Subscription state refreshed", "updated", len(toUpdate))
	return nil
}
