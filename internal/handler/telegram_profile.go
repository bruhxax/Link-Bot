package handler

import (
	"context"
	"strings"

	"github.com/go-telegram/bot/models"
)

func contextWithTelegramProfile(ctx context.Context, user models.User) context.Context {
	ctx = context.WithValue(ctx, "username", strings.TrimSpace(user.Username))
	ctx = context.WithValue(ctx, "telegramName", telegramDisplayName(user.FirstName, user.LastName))
	return ctx
}

func telegramDisplayName(firstName, lastName string) string {
	return strings.TrimSpace(strings.TrimSpace(firstName) + " " + strings.TrimSpace(lastName))
}
