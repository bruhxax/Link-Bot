package database

import (
	"context"
	"github.com/google/uuid"
)

type PanelPlanObservation struct {
	PanelID   *int64
	PanelUUID *uuid.UUID
	Link      string
	Months    int
	Days      int
	PlanID    string
}

// Purchase history supplies duration only. Identity and current unlimited
// traffic are taken from the panel, so renamed/duplicate plan IDs merge safely.
func (cr *CustomerRepository) PanelPlanObservations(ctx context.Context) ([]PanelPlanObservation, error) {
	rows, err := cr.pool.Query(ctx, `SELECT s.panel_user_id,s.panel_user_uuid,COALESCE(s.subscription_link,''),
	 COALESCE(p.month,0),COALESCE(p.days,0),COALESCE(p.plan_id,'')
	 FROM customer_subscription s LEFT JOIN LATERAL (
	 SELECT month,days,plan_id FROM purchase WHERE subscription_id=s.id AND status='paid' AND (month>0 OR days>0)
	 ORDER BY COALESCE(paid_at,created_at) DESC,id DESC LIMIT 1) p ON TRUE
	 WHERE s.panel_user_id IS NOT NULL OR s.panel_user_uuid IS NOT NULL OR NULLIF(s.subscription_link,'') IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []PanelPlanObservation{}
	for rows.Next() {
		var item PanelPlanObservation
		if err = rows.Scan(&item.PanelID, &item.PanelUUID, &item.Link, &item.Months, &item.Days, &item.PlanID); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
