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
				checkCtx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
				if err := pool.Ping(checkCtx); err != nil {
					failures["database"]++
					if failures["database"] >= 2 {
						reporter.Report(checkCtx, ReportInput{Category: "База данных", Severity: "critical", Operation: "healthcheck", Message: "База данных не отвечает", Err: err})
					}
				} else {
					failures["database"] = 0
				}
				if rw != nil {
					if err := rw.Ping(checkCtx); err != nil {
						failures["remnawave"]++
						if failures["remnawave"] >= 2 {
							reporter.Report(checkCtx, ReportInput{Category: "Remnawave", Severity: "critical", Operation: "healthcheck", Message: "Панель Remnawave не отвечает", Err: err})
						}
					} else {
						failures["remnawave"] = 0
					}
				}
				cancel()
			}
		}
	}()
}
