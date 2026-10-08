package database

import (
	"context"
	"github.com/jackc/pgx/v4"
	"testing"
	"time"
)

type observationRow struct {
	updated time.Time
	expiry  *time.Time
	link    *string
	err     error
}

func (r observationRow) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) == 1 {
		*dest[0].(*time.Time) = r.updated
	} else {
		*dest[0].(**time.Time), *dest[1].(**string) = r.expiry, r.link
	}
	return nil
}

type observationTx struct {
	pgx.Tx
	rows  []observationRow
	reads int
}

func (t *observationTx) QueryRow(_ context.Context, _ string, _ ...interface{}) pgx.Row {
	r := t.rows[t.reads]
	t.reads++
	return r
}

func TestPanelObservationRejectsConcurrentChanges(t *testing.T) {
	now := time.Now().UTC()
	expiry := now.Add(time.Hour)
	renewed := expiry.Add(24 * time.Hour)
	link, changedLink := "https://example.com/old", "https://example.com/new"
	for _, tc := range []struct {
		name    string
		primary bool
		rows    []observationRow
		want    bool
	}{
		{"same snapshot", true, []observationRow{{updated: now}, {expiry: &expiry, link: &link}}, true},
		{"purchase changed subscription", true, []observationRow{{updated: now.Add(time.Second)}}, false},
		{"subscription transferred away", true, []observationRow{{err: pgx.ErrNoRows}}, false},
		{"reward changed customer expiry", true, []observationRow{{updated: now}, {expiry: &renewed, link: &link}}, false},
		{"rebind changed customer link", true, []observationRow{{updated: now}, {expiry: &expiry, link: &changedLink}}, false},
		{"secondary independent of primary", false, []observationRow{{updated: now}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &observationTx{rows: tc.rows}
			ok, err := panelObservationCurrent(context.Background(), tx, &Customer{ID: 1, ExpireAt: &expiry, SubscriptionLink: &link}, &CustomerSubscription{ID: 2, CustomerID: 1, IsPrimary: tc.primary, UpdatedAt: now})
			if err != nil || ok != tc.want {
				t.Fatalf("current=%t err=%v, want %t", ok, err, tc.want)
			}
			if tx.reads != len(tc.rows) {
				t.Fatalf("unexpected reads %d", tx.reads)
			}
		})
	}
}
