package broadcast

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-telegram/bot/models"
	"link-bot/internal/config"
	"link-bot/internal/database"
)

// Source messages are captured in Telegram, outside the mini-app middleware.
// Record only sources that were actually saved; never duplicate their contents.
func (s *Service) recordCapturedActivity(ctx context.Context, message *models.Message, kind string, customerID int64) {
	if s.customerRepository == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	entry := database.AdminActivity{ActorTelegramID: message.From.ID, ActorName: message.From.Username, Action: "broadcast/source", Category: "communication", Title: "Сохранил сообщение для рассылки из Telegram", Status: "success"}
	if message.From.ID == config.GetAdminTelegramId() {
		entry.ActorRole = "Главный администратор"
	} else if admin, err := s.customerRepository.AdministratorForTelegramID(ctx, message.From.ID); err == nil && admin != nil {
		entry.ActorRole = admin.Role
	}
	labels := map[string]string{"text": "Текст", "html": "HTML-текст", "photo": "Фото", "video": "Видео", "audio": "Аудио", "document": "Файл", "voice": "Голосовое сообщение", "animation": "Анимация"}
	label := labels[kind]
	if label == "" {
		label = kind
	}
	entry.Details = []database.AdminActivityDetail{{Label: "Тип сообщения", Value: label}}
	if customerID > 0 {
		entry.Action = "users/message/source"
		entry.Title = "Сохранил личное сообщение из Telegram"
		entry.TargetCustomerID = customerID
		if customer, err := s.customerRepository.FindById(ctx, customerID); err == nil && customer != nil {
			entry.TargetTelegramID = customer.TelegramID
			if customer.TelegramUsername != nil {
				entry.TargetName = *customer.TelegramUsername
			}
		}
	}
	if _, err := s.customerRepository.CreateAdminActivity(ctx, entry); err != nil {
		slog.Error("broadcast: record administrator activity", "error", err, "actor", entry.ActorTelegramID)
	}
}
