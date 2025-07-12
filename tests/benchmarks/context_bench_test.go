package benchmarks

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/goef-team/goef"
	_ "github.com/mattn/go-sqlite3"
)

type BenchUser struct {
	ID        int64      `goef:"primary_key,auto_increment" json:"id"`
	CreatedAt time.Time  `goef:"created_at" json:"created_at"`
	UpdatedAt time.Time  `goef:"updated_at" json:"updated_at"`
	DeletedAt *time.Time `goef:"deleted_at,soft_delete" json:"deleted_at,omitempty"`
	Name      string     `goef:"required,max_length:100"`
	Email     string     `goef:"required,unique,max_length:255"`
	Age       int        `goef:"min:0,max:150"`
	IsActive  bool       `goef:"default:true"`
}

func (u *BenchUser) GetTableName() string {
	return "bench_users"
}

func (u *BenchUser) GetID() interface{} {
	return u.ID
}

func (u *BenchUser) SetID(id interface{}) {
	if v, ok := id.(int64); ok {
		u.ID = v
	}
}

func setupBenchDB(b *testing.B) (*goef.DbContext, func()) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		b.Fatalf("Failed to open database: %v", err)
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
		b.Fatalf("Failed to create test table: %v", err)
	}

	ctx, err := goef.NewDbContext(
		goef.WithDB(db),
		goef.WithSQLite(":memory:"),
		goef.WithChangeTracking(true),
	)
	if err != nil {
		err := db.Close()
		if err != nil {
			return nil, nil
		}
		b.Fatalf("Failed to create DbContext: %v", err)
	}

	cleanup := func() {
		err := ctx.Close()
		if err != nil {
			return
		}
	}

	return ctx, cleanup
}

func BenchmarkDbContext_Add(b *testing.B) {
	ctx, cleanup := setupBenchDB(b)
	defer cleanup()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		user := &BenchUser{
			Name:     "User",
			Email:    fmt.Sprintf("user_%d@example.com", i),
			Age:      25,
			IsActive: true,
		}
		user.CreatedAt = time.Now()
		user.UpdatedAt = time.Now()

		err := ctx.Add(user)
		if err != nil {
			b.Fatalf("Failed to add user: %v", err)
		}
	}
}

func BenchmarkDbContext_SaveChanges_Single(b *testing.B) {
	ctx, cleanup := setupBenchDB(b)
	defer cleanup()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		user := &BenchUser{
			Name:     "User",
			Email:    fmt.Sprintf("user_%d@example.com", i),
			Age:      25,
			IsActive: true,
		}
		user.CreatedAt = time.Now()
		user.UpdatedAt = time.Now()

		if err := ctx.Add(user); err != nil {
			b.Fatalf("Error adding user: %v", err)
		}

		b.StartTimer()
		_, err := ctx.SaveChanges()
		b.StopTimer()

		if err != nil {
			b.Fatalf("Failed to save changes: %v", err)
		}
	}
}

func BenchmarkDbContext_SaveChanges_Batch(b *testing.B) {
	batchSizes := []int{10, 50}

	for _, batchSize := range batchSizes {
		b.Run(fmt.Sprintf("BatchSize_%d", batchSize), func(b *testing.B) {
			ctx, cleanup := setupBenchDB(b)
			defer cleanup()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				for j := 0; j < batchSize; j++ {
					user := &BenchUser{
						Name:     fmt.Sprintf("User_%d", j),
						Email:    fmt.Sprintf("user_%d_%d@example.com", i, j),
						Age:      25,
						IsActive: true,
					}
					user.CreatedAt = time.Now()
					user.UpdatedAt = time.Now()
					if err := ctx.Add(user); err != nil {
						b.Fatalf("Error adding user: %v", err)
					}
				}

				b.StartTimer()
				_, err := ctx.SaveChanges()
				b.StopTimer()

				if err != nil {
					b.Fatalf("Failed to save changes: %v", err)
				}
			}
		})
	}
}

func BenchmarkDbContext_Update(b *testing.B) {
	ctx, cleanup := setupBenchDB(b)
	defer cleanup()

	// Create initial user
	user := &BenchUser{
		Name:     "Original User",
		Email:    "original@example.com",
		Age:      25,
		IsActive: true,
	}
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()

	if err := ctx.Add(user); err != nil {
		b.Fatalf("Error adding user: %v", err)
	}
	if _, err := ctx.SaveChanges(); err != nil {
		b.Fatalf("Error saving changes: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		user.Name = fmt.Sprintf("Updated User %d", i)
		user.Age = 26 + (i % 50)

		b.StartTimer()
		err := ctx.Update(user)
		if err != nil {
			b.Fatalf("Failed to update user: %v", err)
		}

		_, err = ctx.SaveChanges()
		b.StopTimer()

		if err != nil {
			b.Fatalf("Failed to save update: %v", err)
		}
	}
}

func BenchmarkDbContext_Find(b *testing.B) {
	ctx, cleanup := setupBenchDB(b)
	defer cleanup()

	// Create test data
	for i := 0; i < 1000; i++ {
		user := &BenchUser{
			Name:     fmt.Sprintf("User_%d", i),
			Email:    fmt.Sprintf("user_%d@example.com", i),
			Age:      25 + (i % 50),
			IsActive: true,
		}
		user.CreatedAt = time.Now()
		user.UpdatedAt = time.Now()
		if err := ctx.Add(user); err != nil {
			b.Fatalf("Error adding user: %v", err)
		}
	}
	if _, err := ctx.SaveChanges(); err != nil {
		b.Fatalf("Error saving changes: %v", err)
	}

	users := ctx.Set(&BenchUser{})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		id := int64(1 + (i % 1000))

		b.StartTimer()
		_, err := users.Find(id)
		b.StopTimer()

		if err != nil {
			b.Fatalf("Failed to find user: %v", err)
		}
	}
}

func BenchmarkDbContext_Transaction(b *testing.B) {
	ctx, cleanup := setupBenchDB(b)
	defer cleanup()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StartTimer()
		err := ctx.BeginTransaction()
		if err != nil {
			b.Fatalf("Failed to begin transaction: %v", err)
		}

		for j := 0; j < 10; j++ {
			user := &BenchUser{
				Name:     fmt.Sprintf("TxUser_%d_%d", i, j),
				Email:    fmt.Sprintf("txuser_%d_%d@example.com", i, j),
				Age:      25,
				IsActive: true,
			}
			user.CreatedAt = time.Now()
			user.UpdatedAt = time.Now()
			if err = ctx.Add(user); err != nil {
				b.Fatalf("Error adding user: %v", err)
			}
		}

		_, err = ctx.SaveChanges()
		if err != nil {
			if err = ctx.RollbackTransaction(); err != nil {
				b.Fatalf("Error rolling back transaction: %v", err)
			}
			b.Fatalf("Failed to save in transaction: %v", err)
		}

		err = ctx.CommitTransaction()
		b.StopTimer()

		if err != nil {
			b.Fatalf("Failed to commit transaction: %v", err)
		}
	}
}

func BenchmarkDbContext_ConcurrentAdd(b *testing.B) {
	ctx, cleanup := setupBenchDB(b)
	defer cleanup()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		counter := 0
		for pb.Next() {
			user := &BenchUser{
				Name:     fmt.Sprintf("ConcurrentUser_%d", counter),
				Email:    fmt.Sprintf("concurrent_%d@example.com", counter),
				Age:      25,
				IsActive: true,
			}
			user.CreatedAt = time.Now()
			user.UpdatedAt = time.Now()

			err := ctx.Add(user)
			if err != nil {
				b.Fatalf("Failed to add user concurrently: %v", err)
			}
			counter++
		}
	})
}

// Memory allocation benchmarks
func BenchmarkDbContext_MemoryAllocation(b *testing.B) {
	ctx, cleanup := setupBenchDB(b)
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		user := &BenchUser{
			Name:     "Memory Test User",
			Email:    fmt.Sprintf("memory_%d@example.com", i),
			Age:      25,
			IsActive: true,
		}
		user.CreatedAt = time.Now()
		user.UpdatedAt = time.Now()

		if err := ctx.Add(user); err != nil {
			b.Fatalf("Error adding user: %v", err)
		}
		if _, err := ctx.SaveChanges(); err != nil {
			b.Fatalf("Error saving changes: %v", err)
		}
	}
}

// Stress test with large datasets
func BenchmarkDbContext_LargeDataset(b *testing.B) {
	ctx, cleanup := setupBenchDB(b)
	defer cleanup()

	const datasetSize = 10000

	// Populate with large dataset
	for i := 0; i < datasetSize; i++ {
		user := &BenchUser{
			Name:     fmt.Sprintf("LargeUser_%d", i),
			Email:    fmt.Sprintf("large_%d@example.com", i),
			Age:      25 + (i % 50),
			IsActive: i%2 == 0,
		}
		user.CreatedAt = time.Now()
		user.UpdatedAt = time.Now()
		err := ctx.Add(user)
		if err != nil {
			b.Fatalf("Error adding user: %v", err)
		}

		if i%1000 == 0 {
			_, err = ctx.SaveChanges()
			if err != nil {
				b.Fatalf("Error saving changes: %v", err)
			}
		}
	}
	_, err := ctx.SaveChanges()
	if err != nil {
		b.Fatalf("Error saving changes: %v", err)
	}

	users := ctx.Set(&BenchUser{})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		id := int64(1 + (i % datasetSize))
		_, err := users.Find(id)
		if err != nil {
			b.Fatalf("Error finding user: %v", err)
		}
	}
}
