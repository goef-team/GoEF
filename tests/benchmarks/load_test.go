package benchmarks

import (
	"database/sql"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/goef-team/goef"
	_ "github.com/mattn/go-sqlite3"
)

func TestLoad_ConcurrentUsers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	concurrencyLevels := []int{1, 10, 50, 100}
	operationsPerUser := 100

	for _, concurrency := range concurrencyLevels {
		t.Run(fmt.Sprintf("Concurrency_%d", concurrency), func(t *testing.T) {
			ctx, cleanup := setupLoadTestDB(t)
			defer cleanup()

			var wg sync.WaitGroup
			errors := make(chan error, concurrency*operationsPerUser)
			start := time.Now()

			for i := 0; i < concurrency; i++ {
				wg.Add(1)
				go func(userID int) {
					defer wg.Done()

					for j := 0; j < operationsPerUser; j++ {
						user := &BenchUser{
							Name:     fmt.Sprintf("User_%d_%d", userID, j),
							Email:    fmt.Sprintf("user_%d_%d@example.com", userID, j),
							Age:      25 + (j % 50),
							IsActive: true,
						}
						user.CreatedAt = time.Now()
						user.UpdatedAt = time.Now()

						err := ctx.Add(user)
						if err != nil {
							errors <- err
							return
						}

						_, err = ctx.SaveChanges()
						if err != nil {
							errors <- err
							return
						}
					}
				}(i)
			}

			wg.Wait()
			duration := time.Since(start)
			close(errors)

			// Check for errors
			errorCount := 0
			for err := range errors {
				if err != nil {
					t.Logf("Error: %v", err)
					errorCount++
				}
			}

			totalOps := concurrency * operationsPerUser
			opsPerSecond := float64(totalOps) / duration.Seconds()

			t.Logf("Concurrency: %d, Total Ops: %d, Duration: %v, Ops/sec: %.2f, Errors: %d",
				concurrency, totalOps, duration, opsPerSecond, errorCount)

			if errorCount > totalOps/10 { // Allow up to 10% errors
				t.Errorf("Too many errors: %d/%d", errorCount, totalOps)
			}
		})
	}
}

func TestLoad_MemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory test in short mode")
	}

	ctx, cleanup := setupLoadTestDB(t)
	defer cleanup()

	const entityCount = 100000

	start := time.Now()
	var memBefore, memAfter runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memBefore)

	// Create many entities
	for i := 0; i < entityCount; i++ {
		user := &BenchUser{
			Name:     fmt.Sprintf("User_%d", i),
			Email:    fmt.Sprintf("user_%d@example.com", i),
			Age:      25 + (i % 50),
			IsActive: true,
		}
		user.CreatedAt = time.Now()
		user.UpdatedAt = time.Now()

		err := ctx.Add(user)
		if err != nil {
			t.Fatalf("Error adding user: %v", err)
		}

		if i%1000 == 0 {
			_, err := ctx.SaveChanges()
			if err != nil {
				t.Fatalf("Error saving changes: %v", err)
			}
		}
	}

	_, err := ctx.SaveChanges()
	if err != nil {
		t.Fatalf("Error saving changes: %v", err)
	}
	duration := time.Since(start)

	runtime.GC()
	runtime.ReadMemStats(&memAfter)

	memUsed := memAfter.Alloc - memBefore.Alloc
	memPerEntity := float64(memUsed) / float64(entityCount)

	t.Logf("Created %d entities in %v", entityCount, duration)
	t.Logf("Memory used: %d bytes (%.2f bytes per entity)", memUsed, memPerEntity)
	t.Logf("Total allocations: %d", memAfter.TotalAlloc-memBefore.TotalAlloc)
}

func TestLoad_LongRunningSession(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long-running test in short mode")
	}

	ctx, cleanup := setupLoadTestDB(t)
	defer cleanup()

	duration := 30 * time.Second
	operationInterval := 100 * time.Millisecond

	start := time.Now()
	operationCount := 0

	for time.Since(start) < duration {
		user := &BenchUser{
			Name:     fmt.Sprintf("LongRunUser_%d", operationCount),
			Email:    fmt.Sprintf("longrun_%d@example.com", operationCount),
			Age:      25 + (operationCount % 50),
			IsActive: true,
		}
		user.CreatedAt = time.Now()
		user.UpdatedAt = time.Now()

		err := ctx.Add(user)
		if err != nil {
			t.Fatalf("Failed to add user: %v", err)
		}

		_, err = ctx.SaveChanges()
		if err != nil {
			t.Fatalf("Failed to save user: %v", err)
		}

		operationCount++
		time.Sleep(operationInterval)
	}

	actualDuration := time.Since(start)
	opsPerSecond := float64(operationCount) / actualDuration.Seconds()

	t.Logf("Long-running test completed: %d operations in %v (%.2f ops/sec)",
		operationCount, actualDuration, opsPerSecond)
}

func setupLoadTestDB(t *testing.T) (*goef.DbContext, func()) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE bench_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(100) NOT NULL,
			email VARCHAR(255) NOT NULL UNIQUE,
			age INTEGER,
			is_active BOOLEAN DEFAULT true,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}

	ctx, err := goef.NewDbContext(
		goef.WithDB(db),
		goef.WithSQLite(":memory:"),
		goef.WithChangeTracking(true),
	)
	if err != nil {
		err := db.Close()
		if err != nil {
			t.Fatalf("error on closing db, err: %v", err)
		}
		t.Fatalf("Failed to create DbContext: %v", err)
	}

	cleanup := func() {
		err := ctx.Close()
		if err != nil {
			t.Fatalf("error on closing context, err: %v", err)
		}
	}

	return ctx, cleanup
}
