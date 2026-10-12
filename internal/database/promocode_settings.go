package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgconn"
)

var ErrPromoCodeChanged = errors.New("promo code changed or removed")
var ErrPromoCodePersonalLimit = errors.New("personal promo code requires a limit of one")

// UpdateSettings preserves the reward, its ownership and every redemption.
// The revision check prevents an old editor from overwriting another admin.
func (r *PromoCodeRepository) UpdateSettings(ctx context.Context, id int64, active bool, limit *int, expiry *time.Time, expected time.Time) error {
	result, err := r.pool.Exec(ctx, `UPDATE promo_code SET is_active=$2,max_redemptions=$3,expires_at=$4,
 updated_at=GREATEST(NOW(),updated_at+INTERVAL '1 microsecond') WHERE id=$1 AND deleted_at IS NULL AND updated_at=$5`, id, active, limit, expiry, expected)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23514" && pgErr.ConstraintName == "promo_code_personal_limit" {
			return ErrPromoCodePersonalLimit
		}
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrPromoCodeChanged
	}
	return nil
}
