package database

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// Waiting for a session lock must not exhaust the pool: the lock holder needs
// other connections to finish its work. Only a successful lock gets a dedicated
// session; unsuccessful attempts immediately return their connection to the pool.
func acquireSessionLock(ctx context.Context, pool *pgxpool.Pool, trySQL, unlockSQL string, key interface{}) (func() error, error) {
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	for {
		conn, err := pool.Acquire(waitCtx)
		if err != nil {
			return nil, err
		}
		var locked bool
		err = conn.QueryRow(waitCtx, trySQL, key).Scan(&locked)
		if err != nil {
			// A cancelled query may have acquired the lock before its response
			// was lost. Never return that session to the pool.
			closeLockSession(conn.Hijack())
			return nil, err
		}
		if locked {
			lockConn := conn.Hijack()
			var once sync.Once
			var releaseErr error
			return func() error {
				once.Do(func() {
					cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					_, releaseErr = lockConn.Exec(cleanup, unlockSQL, key)
					// Closing also releases the lock if explicit unlock failed.
					closeLockSession(lockConn)
				})
				return releaseErr
			}, nil
		}
		conn.Release()

		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			return nil, waitCtx.Err()
		case <-timer.C:
		}
	}
}

func closeLockSession(conn *pgx.Conn) {
	cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = conn.Close(cleanup)
}
