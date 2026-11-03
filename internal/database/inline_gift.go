package database

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
)

type InlineGift struct {
	ID               uuid.UUID
	SenderTelegramID int64
	PlanID           string
	InlineMessageID  string
	HasBanner        bool
	PurchaseID       *int64
}

func (pr *PurchaseRepository) FindInlineGift(ctx context.Context, id uuid.UUID) (*InlineGift, error) {
	// A payment provider may finish before the checkout request binds its purchase.
	// Recover that link also after a restart so the paid gift can still be published.
	_, err := pr.pool.Exec(ctx, `UPDATE inline_gift g SET purchase_id = p.id
		FROM purchase p JOIN customer c ON c.id = p.customer_id
		WHERE g.id = $1 AND g.purchase_id IS NULL AND p.gift_token = g.id
		AND p.purchase_kind = 'gift' AND p.plan_id = g.plan_id AND c.telegram_id = g.sender_telegram_id`, id)
	if err != nil {
		return nil, err
	}
	var item InlineGift
	err = pr.pool.QueryRow(ctx, `SELECT id, sender_telegram_id, plan_id, inline_message_id, has_banner, purchase_id FROM inline_gift WHERE id = $1`, id).
		Scan(&item.ID, &item.SenderTelegramID, &item.PlanID, &item.InlineMessageID, &item.HasBanner, &item.PurchaseID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (pr *PurchaseRepository) GetOrCreateInlineGift(ctx context.Context, item InlineGift) (*InlineGift, error) {
	_, err := pr.pool.Exec(ctx, `INSERT INTO inline_gift (id, sender_telegram_id, plan_id, inline_message_id, has_banner)
		VALUES ($1, $2, $3, $4, $5) ON CONFLICT (inline_message_id) DO NOTHING`, item.ID, item.SenderTelegramID, item.PlanID, item.InlineMessageID, item.HasBanner)
	if err != nil {
		return nil, err
	}
	var saved InlineGift
	err = pr.pool.QueryRow(ctx, `SELECT id, sender_telegram_id, plan_id, inline_message_id, has_banner, purchase_id FROM inline_gift WHERE inline_message_id = $1`, item.InlineMessageID).
		Scan(&saved.ID, &saved.SenderTelegramID, &saved.PlanID, &saved.InlineMessageID, &saved.HasBanner, &saved.PurchaseID)
	if err != nil {
		return nil, err
	}
	return pr.FindInlineGift(ctx, saved.ID)
}

func (pr *PurchaseRepository) BindInlineGiftPurchase(ctx context.Context, id uuid.UUID, purchaseID int64) (bool, error) {
	result, err := pr.pool.Exec(ctx, `UPDATE inline_gift SET purchase_id = $2 WHERE id = $1 AND (purchase_id IS NULL OR purchase_id = $2) AND checkout_started_at IS NOT NULL`, id, purchaseID)
	if err != nil {
		return false, err
	}
	return result.RowsAffected() == 1, nil
}

func (pr *PurchaseRepository) ReserveInlineGiftCheckout(ctx context.Context, id uuid.UUID, senderTelegramID int64, planID string) (bool, error) {
	result, err := pr.pool.Exec(ctx, `UPDATE inline_gift SET checkout_started_at = NOW()
		WHERE id = $1 AND sender_telegram_id = $2 AND plan_id = $3 AND purchase_id IS NULL AND checkout_started_at IS NULL`, id, senderTelegramID, planID)
	if err != nil {
		return false, err
	}
	return result.RowsAffected() == 1, nil
}

func (pr *PurchaseRepository) ReleaseInlineGiftCheckout(ctx context.Context, id uuid.UUID) error {
	_, err := pr.pool.Exec(ctx, `UPDATE inline_gift SET checkout_started_at = NULL WHERE id = $1 AND purchase_id IS NULL`, id)
	return err
}

func (pr *PurchaseRepository) ListUnpublishedPaidInlineGifts(ctx context.Context, limit int) ([]InlineGift, error) {
	_, err := pr.pool.Exec(ctx, `UPDATE inline_gift g SET purchase_id = p.id
		FROM purchase p JOIN customer c ON c.id = p.customer_id
		WHERE g.purchase_id IS NULL AND g.checkout_started_at IS NOT NULL AND p.gift_token = g.id
		AND p.purchase_kind = 'gift' AND p.plan_id = g.plan_id AND c.telegram_id = g.sender_telegram_id`)
	if err != nil {
		return nil, err
	}
	rows, err := pr.pool.Query(ctx, `SELECT g.id, g.sender_telegram_id, g.plan_id, g.inline_message_id, g.has_banner, g.purchase_id
		FROM inline_gift g JOIN purchase p ON p.id = g.purchase_id
		WHERE p.status = 'paid' AND g.published_at IS NULL
		ORDER BY g.created_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]InlineGift, 0)
	for rows.Next() {
		var item InlineGift
		if err := rows.Scan(&item.ID, &item.SenderTelegramID, &item.PlanID, &item.InlineMessageID, &item.HasBanner, &item.PurchaseID); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (pr *PurchaseRepository) MarkInlineGiftPublished(ctx context.Context, id uuid.UUID) error {
	_, err := pr.pool.Exec(ctx, `UPDATE inline_gift SET published_at = COALESCE(published_at, NOW()) WHERE id = $1`, id)
	return err
}
