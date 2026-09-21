package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"link-bot/internal/bedolagaimport"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	var sourceURL string
	var targetURL string
	var apply bool
	flag.StringVar(&sourceURL, "source-database-url", os.Getenv("BEDOLAGA_DATABASE_URL"), "Bedolaga PostgreSQL connection URL")
	flag.StringVar(&targetURL, "target-database-url", os.Getenv("DATABASE_URL"), "Link-Bot PostgreSQL connection URL")
	flag.BoolVar(&apply, "apply", false, "write the import; omitted means dry run")
	flag.Parse()

	if os.Getenv("DISABLE_ENV_FILE") != "true" {
		_ = godotenv.Load()
		if sourceURL == "" {
			sourceURL = os.Getenv("BEDOLAGA_DATABASE_URL")
		}
		if targetURL == "" {
			targetURL = os.Getenv("DATABASE_URL")
		}
	}
	sourceURL = strings.TrimSpace(sourceURL)
	targetURL = strings.TrimSpace(targetURL)
	if sourceURL == "" || targetURL == "" {
		log.Fatal("both BEDOLAGA_DATABASE_URL and DATABASE_URL are required; database URLs are never printed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	source, err := pgxpool.Connect(ctx, sourceURL)
	if err != nil {
		log.Fatalf("connect to Bedolaga database: %v", err)
	}
	defer source.Close()
	target, err := pgxpool.Connect(ctx, targetURL)
	if err != nil {
		log.Fatalf("connect to Link-Bot database: %v", err)
	}
	defer target.Close()

	report, err := bedolagaimport.Run(ctx, bedolagaimport.Options{Source: source, Target: target, Apply: apply})
	if err != nil {
		log.Fatalf("Bedolaga migration failed: %v", err)
	}
	mode := "dry run: no data changed"
	if apply {
		mode = "applied"
	}
	fmt.Printf("Bedolaga migration %s\nusers=%d active_or_trial_subscriptions=%d skipped_nontransferable_subscriptions=%d balances=%d referrals=%d\n", mode, report.Users, report.ActiveSubscriptions, report.SkippedSubscriptions, report.Balances, report.Referrals)
	if apply {
		fmt.Printf("created_customers=%d created_subscriptions=%d updated_subscriptions=%d applied_balances=%d applied_referrals=%d\n", report.CreatedCustomers, report.CreatedSubscriptions, report.UpdatedSubscriptions, report.AppliedBalances, report.AppliedReferrals)
	}
}
