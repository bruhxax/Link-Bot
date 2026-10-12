package database

import (
	"context"
	"time"
)

type OverviewPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}
type OverviewUsers struct {
	Source   string          `json:"source,omitempty"`
	Limited  int             `json:"limited"`
	Disabled int             `json:"disabled"`
	Other    int             `json:"other"`
	Total    int             `json:"total"`
	Active   int             `json:"active"`
	Expired  int             `json:"expired"`
	Without  int             `json:"without"`
	Blocked  int             `json:"blocked"`
	Today    int             `json:"today"`
	Week     int             `json:"week"`
	Daily    []OverviewPoint `json:"daily"`
}
type OverviewPlan struct {
	Label     string `json:"label,omitempty"`
	Months    int    `json:"months,omitempty"`
	Days      int    `json:"days,omitempty"`
	Unlimited bool   `json:"unlimited"`
	Key       string `json:"key"`
	Count     int    `json:"count"`
}
type OverviewRevenue struct {
	TodayRub   float64         `json:"todayRub"`
	WeekRub    float64         `json:"weekRub"`
	MonthRub   float64         `json:"monthRub"`
	AllRub     float64         `json:"allRub"`
	AverageRub float64         `json:"averageRub"`
	AllStars   float64         `json:"allStars"`
	Daily      []OverviewPoint `json:"daily"`
}
type AdminOverview struct {
	Pending       bool             `json:"pending,omitempty"`
	RevenueError  string           `json:"revenueError,omitempty"`
	PanelError    string           `json:"panelError,omitempty"`
	Days          int              `json:"days"`
	GeneratedAt   time.Time        `json:"generatedAt"`
	Users         *OverviewUsers   `json:"users"`
	Subscriptions []OverviewPlan   `json:"subscriptions"`
	Revenue       *OverviewRevenue `json:"revenue"`
}

// Aggregate the whole account population rather than the current users-table
// page. Permission flags also prevent querying financial data for other roles.
func (cr *CustomerRepository) LoadAdminOverview(ctx context.Context, days int, now time.Time, users, finance bool) (*AdminOverview, error) {
	zone := time.FixedZone("MSK", 3*60*60)
	local := now.In(zone)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)
	week := today.AddDate(0, 0, -6)
	month := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, zone)
	from := today.AddDate(0, 0, -(days - 1))
	result := &AdminOverview{Days: days, GeneratedAt: now.UTC(), Subscriptions: []OverviewPlan{}}
	if users {
		u := &OverviewUsers{Daily: []OverviewPoint{}}
		if err := cr.pool.QueryRow(ctx, `WITH states AS (
			SELECT c.is_blocked,c.created_at,
			EXISTS(SELECT 1 FROM customer_subscription s WHERE s.customer_id=c.id AND s.expire_at>$1 AND NULLIF(s.subscription_link,'') IS NOT NULL)
			 OR COALESCE(c.expire_at>$1 AND NULLIF(c.subscription_link,'') IS NOT NULL,FALSE) AS active,
			EXISTS(SELECT 1 FROM customer_subscription s WHERE s.customer_id=c.id AND s.expire_at IS NOT NULL) OR c.expire_at IS NOT NULL AS has_subscription
			FROM customer c)
			SELECT COUNT(*),COUNT(*) FILTER(WHERE NOT is_blocked AND active),
			COUNT(*) FILTER(WHERE NOT is_blocked AND NOT active AND has_subscription),
			COUNT(*) FILTER(WHERE NOT is_blocked AND NOT active AND NOT has_subscription),
			COUNT(*) FILTER(WHERE is_blocked),COUNT(*) FILTER(WHERE created_at >= $2),COUNT(*) FILTER(WHERE created_at >= $3)
			FROM states`, now, today, week).Scan(&u.Total, &u.Active, &u.Expired, &u.Without, &u.Blocked, &u.Today, &u.Week); err != nil {
			return nil, err
		}
		rows, err := cr.pool.Query(ctx, `SELECT to_char(d.day,'YYYY-MM-DD'),COUNT(c.id)::double precision
			FROM generate_series($1::date,$2::date,INTERVAL '1 day') d(day)
			LEFT JOIN customer c ON (c.created_at AT TIME ZONE 'Europe/Moscow')::date=d.day::date
			GROUP BY d.day ORDER BY d.day`, from.Format("2006-01-02"), today.Format("2006-01-02"))
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var p OverviewPoint
			if err = rows.Scan(&p.Date, &p.Value); err != nil {
				rows.Close()
				return nil, err
			}
			u.Daily = append(u.Daily, p)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		result.Users = u
		rows, err = cr.pool.Query(ctx, `SELECT CASE WHEN p.id IS NULL THEN 'manual' WHEN p.amount<=0 THEN 'free' ELSE COALESCE(NULLIF(p.plan_id,''),'paid') END,COUNT(*)
			FROM customer_subscription s JOIN customer c ON c.id=s.customer_id
			LEFT JOIN LATERAL (SELECT p.id,p.amount,p.plan_id FROM purchase p
			 WHERE p.subscription_id=s.id AND p.status='paid' AND (p.month>0 OR p.days>0)
			 ORDER BY COALESCE(p.paid_at,p.created_at) DESC,p.id DESC LIMIT 1) p ON TRUE
			WHERE NOT c.is_blocked AND s.expire_at>$1 AND NULLIF(s.subscription_link,'') IS NOT NULL
			GROUP BY 1 ORDER BY COUNT(*) DESC,1`, now)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var p OverviewPlan
			if err = rows.Scan(&p.Key, &p.Count); err != nil {
				rows.Close()
				return nil, err
			}
			result.Subscriptions = append(result.Subscriptions, p)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	if finance {
		r := &OverviewRevenue{Daily: []OverviewPoint{}}
		if err := cr.pool.QueryRow(ctx, `WITH payments AS (
			SELECT amount,UPPER(COALESCE(NULLIF(currency,''),'RUB')) AS currency,COALESCE(paid_at,created_at) AS occurred_at
			FROM purchase WHERE status='paid' AND invoice_type NOT IN ('free','balance'))
			SELECT COALESCE(SUM(amount) FILTER(WHERE currency='RUB' AND occurred_at >= $1),0),
			COALESCE(SUM(amount) FILTER(WHERE currency='RUB' AND occurred_at >= $2),0),
			COALESCE(SUM(amount) FILTER(WHERE currency='RUB' AND occurred_at >= $3),0),
			COALESCE(SUM(amount) FILTER(WHERE currency='RUB'),0),COALESCE(AVG(amount) FILTER(WHERE currency='RUB' AND amount>0),0),
			COALESCE(SUM(amount) FILTER(WHERE currency='STARS'),0) FROM payments`, today, week, month).Scan(&r.TodayRub, &r.WeekRub, &r.MonthRub, &r.AllRub, &r.AverageRub, &r.AllStars); err != nil {
			return nil, err
		}
		rows, err := cr.pool.Query(ctx, `WITH daily AS (
		 SELECT (COALESCE(paid_at,created_at) AT TIME ZONE 'Europe/Moscow')::date AS day,SUM(amount) AS amount
		 FROM purchase WHERE COALESCE(paid_at,created_at)>=$3 AND COALESCE(paid_at,created_at)<$4
		 AND status='paid' AND invoice_type NOT IN ('free','balance') AND UPPER(COALESCE(NULLIF(currency,''),'RUB'))='RUB'
		 GROUP BY 1)
		 SELECT to_char(d.day,'YYYY-MM-DD'),COALESCE(p.amount,0)
			FROM generate_series($1::date,$2::date,INTERVAL '1 day') d(day)
			LEFT JOIN daily p ON p.day=d.day::date ORDER BY d.day`, from.Format("2006-01-02"), today.Format("2006-01-02"), from, today.AddDate(0, 0, 1))
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var p OverviewPoint
			if err = rows.Scan(&p.Date, &p.Value); err != nil {
				rows.Close()
				return nil, err
			}
			r.Daily = append(r.Daily, p)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		result.Revenue = r
	}
	return result, nil
}
