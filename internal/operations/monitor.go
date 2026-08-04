package operations

import (
	"context"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"

	"link-bot/internal/remnawave"
)

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
				if err != nil {
					failures["database"]++
					if failures["database"] >= 3 {
						reportHealthFailure(reporter, ReportInput{Category: "База данных", Severity: "critical", Operation: "healthcheck", Message: "База данных не отвечает", Err: err})
					}
				} else {
					failures["database"] = 0
				}
				if rw != nil {
					panelCtx, panelCancel := context.WithTimeout(ctx, 7*time.Second)
					err = rw.Ping(panelCtx)
					panelCancel()
					if err != nil {
						failures["remnawave"]++
						if failures["remnawave"] >= 3 {
							reportHealthFailure(reporter, ReportInput{Category: "Remnawave", Severity: "critical", Operation: "healthcheck", Message: "Панель Remnawave не отвечает", Err: err})
						}
					} else {
						failures["remnawave"] = 0
					}
				}
			}
		}
	}()
}

func reportHealthFailure(reporter *Reporter, input ReportInput) {
	reportCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reporter.Report(reportCtx, input)
}
