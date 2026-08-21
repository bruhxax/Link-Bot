package operations

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"

	"link-bot/internal/remnawave"
)

const healthFailureThreshold = 3

func StartHealthMonitor(ctx context.Context, pool *pgxpool.Pool, rw *remnawave.Client, reporter *Reporter) {
	if reporter == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		failures := map[string]int{}
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				databaseCtx, databaseCancel := context.WithTimeout(ctx, 5*time.Second)
				err := pool.Ping(databaseCtx)
				databaseCancel()
				if recordHealthResult(failures, "database", err) {
					reportHealthFailure(reporter, ReportInput{Category: "База данных", Severity: "critical", Operation: "healthcheck", Message: "База данных не отвечает", Err: err})
				}
				if rw != nil {
					panelCtx, panelCancel := context.WithTimeout(ctx, 7*time.Second)
					err = rw.Ping(panelCtx)
					panelCancel()
					if recordHealthResult(failures, "remnawave", err) {
						reportHealthFailure(reporter, ReportInput{Category: "Remnawave", Severity: "critical", Operation: "healthcheck", Message: "Панель Remnawave не отвечает", Err: err})
					}
				}
			}
		}
	}()
}

func recordHealthResult(failures map[string]int, key string, err error) bool {
	if err == nil {
		failures[key] = 0
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	failures[key]++
	// Persist and notify once per continuous outage. A successful probe resets
	// the counter and allows a later, separate outage to be reported again.
	return failures[key] == healthFailureThreshold
}

func reportHealthFailure(reporter *Reporter, input ReportInput) {
	reportCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reporter.Report(reportCtx, input)
}
