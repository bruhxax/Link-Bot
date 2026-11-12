package database

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
)

func sessionLockTestPool(t *testing.T, size int32) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("SESSION_LOCK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set SESSION_LOCK_TEST_DATABASE_URL to an isolated PostgreSQL database")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = size
	config.MinConns = 0
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestSessionLockLeavesPoolAvailable(t *testing.T) {
	for _, kind := range []string{"purchase", "auto-payment", "free-plan"} {
		t.Run(kind, func(t *testing.T) {
			pool := sessionLockTestPool(t, 1)
			repo := &PurchaseRepository{pool: pool}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			id := time.Now().UnixNano()%1000000000 + 1
			lock := func(ctx context.Context) (func() error, error) {
				switch kind {
				case "purchase":
					return repo.LockForProcessing(ctx, id)
				case "auto-payment":
					return repo.LockAutoPaymentCustomer(ctx, id)
				default:
					return repo.LockFreePlanActivation(ctx, id, "regression")
				}
			}
			release, err := lock(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			queryCtx, queryCancel := context.WithTimeout(ctx, 500*time.Millisecond)
			err = pool.Ping(queryCtx)
			queryCancel()
			if err != nil {
				t.Fatalf("lock holder cannot use its one-connection pool: %v", err)
			}

			waitCtx, waitCancel := context.WithTimeout(ctx, 150*time.Millisecond)
			unexpectedRelease, err := lock(waitCtx)
			waitCancel()
			if err == nil {
				_ = unexpectedRelease()
				t.Fatal("same lock was acquired twice before its holder released it")
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("expected cancelled waiter, got %v", err)
			}
			if err := pool.Ping(ctx); err != nil {
				t.Fatalf("cancelled waiter exhausted the pool: %v", err)
			}
			if err := release(); err != nil {
				t.Fatal(err)
			}
			nextRelease, err := lock(ctx)
			if err != nil {
				t.Fatalf("lock was not released: %v", err)
			}
			if err := nextRelease(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSessionLockConcurrentWaitersDoNotExhaustPool(t *testing.T) {
	pool := sessionLockTestPool(t, 2)
	repo := &PurchaseRepository{pool: pool}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	id := time.Now().UnixNano()%1000000000 + 1
	release, err := repo.LockForProcessing(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	const workers = 24 // More than the production pool capacity.
	var wg sync.WaitGroup
	var active atomic.Int32
	errorsCh := make(chan error, workers*3)
	started := make(chan struct{}, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			started <- struct{}{}
			release, err := repo.LockForProcessing(ctx, id)
			if err != nil {
				errorsCh <- err
				return
			}
			if active.Add(1) != 1 {
				errorsCh <- errors.New("purchase fulfillment overlapped")
			}
			if err := pool.Ping(ctx); err != nil {
				errorsCh <- err
			}
			time.Sleep(5 * time.Millisecond)
			active.Add(-1)
			if err := release(); err != nil {
				errorsCh <- err
			}
		}()
	}
	for i := 0; i < workers; i++ {
		<-started
	}
	// Let all competing attempts reach PostgreSQL before protected work queries it.
	time.Sleep(150 * time.Millisecond)
	queryCtx, queryCancel := context.WithTimeout(ctx, time.Second)
	queryErr := pool.Ping(queryCtx)
	queryCancel()
	if err := release(); err != nil {
		t.Error(err)
	}
	wg.Wait()
	close(errorsCh)
	if queryErr != nil {
		t.Errorf("waiters blocked the lock holder's database work: %v", queryErr)
	}
	for err := range errorsCh {
		t.Error(err)
	}
}

func TestSessionLockClosesSessionAfterUnlockFailure(t *testing.T) {
	pool := sessionLockTestPool(t, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	key := time.Now().UnixNano()
	release, err := acquireSessionLock(ctx, pool, "SELECT pg_try_advisory_lock($1)", "SELECT 1/0 WHERE $1::bigint IS NOT NULL", key)
	if err != nil {
		t.Fatal(err)
	}
	if err := release(); err == nil {
		t.Fatal("expected unlock failure")
	}
	nextRelease, err := acquireSessionLock(ctx, pool, "SELECT pg_try_advisory_lock($1)", "SELECT pg_advisory_unlock($1)", key)
	if err != nil {
		t.Fatalf("failed unlock leaked its session lock: %v", err)
	}
	if err := nextRelease(); err != nil {
		t.Fatal(err)
	}
}
