package database

import (
	"context"
	"github.com/google/uuid"
)

type PanelPlanObservation struct {
	CustomerID     int64
	SubscriptionID int64
	PanelID        *int64
	PanelUUID      *uuid.UUID
	Link           string
	Months         int
	Days           int
	PlanID         string
	IsTrial        bool
}

// Purchase history supplies duration only. Identity and current unlimited
// traffic are taken from the panel, so renamed/duplicate plan IDs merge safely.
func (cr *CustomerRepository) PanelPlanObservations(ctx context.Context) ([]PanelPlanObservation, error) {
	rows, err := cr.pool.Query(ctx, `WITH latest AS (
	 SELECT DISTINCT ON (subscription_id) subscription_id,month,days,plan_id
	 FROM purchase WHERE status='paid' AND (month>0 OR days>0)
	 ORDER BY subscription_id,COALESCE(paid_at,created_at) DESC,id DESC)
	 SELECT s.customer_id,s.id,s.panel_user_id,s.panel_user_uuid,COALESCE(s.subscription_link,''),
	 COALESCE(p.month,0),COALESCE(p.days,0),COALESCE(p.plan_id,''),
	 COALESCE(c.trial_used,FALSE) AND s.is_primary AND p.subscription_id IS NULL
	 FROM customer_subscription s JOIN customer c ON c.id=s.customer_id LEFT JOIN latest p ON p.subscription_id=s.id
	 WHERE s.panel_user_id IS NOT NULL OR s.panel_user_uuid IS NOT NULL OR NULLIF(s.subscription_link,'') IS NOT NULL ORDER BY s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []PanelPlanObservation{}
	for rows.Next() {
		var item PanelPlanObservation
		if err = rows.Scan(&item.CustomerID, &item.SubscriptionID, &item.PanelID, &item.PanelUUID, &item.Link, &item.Months, &item.Days, &item.PlanID, &item.IsTrial); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
