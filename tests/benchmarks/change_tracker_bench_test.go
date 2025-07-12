package benchmarks

import (
	"testing"
	"time"

	"github.com/goef-team/goef/change"
)

type TestEntityForBench struct {
	ID        int64      `goef:"primary_key,auto_increment" json:"id"`
	CreatedAt time.Time  `goef:"created_at" json:"created_at"`
	UpdatedAt time.Time  `goef:"updated_at" json:"updated_at"`
	DeletedAt *time.Time `goef:"deleted_at,soft_delete" json:"deleted_at,omitempty"`
	Name      string     `goef:"required,max_length:100"`
	Email     string     `goef:"required,unique,max_length:255"`
	Age       int        `goef:"min:0,max:150"`
	IsActive  bool       `goef:"default:true"`
}

func (e *TestEntityForBench) GetTableName() string {
	return "test_entities_bench"
}

func (e *TestEntityForBench) GetID() interface{} {
	return e.ID
}

func (e *TestEntityForBench) SetID(id interface{}) {
	if v, ok := id.(int64); ok {
		e.ID = v
	}
}

func BenchmarkChangeTracker_Add(b *testing.B) {
	tracker := change.NewTracker(true)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		entity := &TestEntityForBench{
			ID:        1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Name:      "Test User",
			Email:     "test@example.com",
			Age:       25,
			IsActive:  true,
		}

		err := tracker.Add(entity)
		if err != nil {
			b.Fatalf("Failed to add entity: %v", err)
		}
	}
}

func BenchmarkChangeTracker_Update(b *testing.B) {
	tracker := change.NewTracker(true)

	// Pre-populate tracker
	for i := 0; i < 1000; i++ {
		entity := &TestEntityForBench{
			ID:        1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Name:      "Test User",
			Email:     "test@example.com",
			Age:       25,
			IsActive:  true,
		}
		if err := tracker.Track(entity); err != nil {
			b.Fatalf("Error tracking entity: %v", err)
		}

	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		entity := &TestEntityForBench{
			ID:        1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Name:      "Updated User",
			Email:     "updated@example.com",
			Age:       26,
			IsActive:  false,
		}

		err := tracker.Update(entity)
		if err != nil {
			b.Fatalf("Failed to update entity: %v", err)
		}
	}
}

func BenchmarkChangeTracker_GetChanges(b *testing.B) {
	tracker := change.NewTracker(true)

	// Pre-populate with changes
	for i := 0; i < 1000; i++ {
		entity := &TestEntityForBench{
			ID:        1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Name:      "Test User",
			Email:     "test@example.com",
			Age:       25,
			IsActive:  true,
		}
		if err := tracker.Add(entity); err != nil {
			b.Fatalf("Error adding entity: %v", err)
		}

	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		changes := tracker.GetChanges()
		_ = changes
	}
}

func BenchmarkChangeTracker_DetectChanges(b *testing.B) {
	tracker := change.NewTracker(true)

	// Pre-populate entities
	entities := make([]*TestEntityForBench, 1000)
	for i := 0; i < 1000; i++ {
		entities[i] = &TestEntityForBench{
			ID:        1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Name:      "Test User",
			Email:     "test@example.com",
			Age:       25,
			IsActive:  true,
		}
		if err := tracker.Track(entities[i]); err != nil {
			b.Fatalf("Error tracking entity: %v", err)
		}

	}

	// Modify some entities
	for i := 0; i < 500; i++ {
		entities[i].Name = "Modified User"
		entities[i].Age = 30
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err := tracker.DetectChanges()
		if err != nil {
			b.Fatalf("Failed to detect changes: %v", err)
		}
	}
}

func BenchmarkChangeTracker_AcceptChanges(b *testing.B) {
	tracker := change.NewTracker(true)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Add some changes
		entity := &TestEntityForBench{
			ID:        1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Name:      "Test User",
			Email:     "test@example.com",
			Age:       25,
			IsActive:  true,
		}
		if err := tracker.Add(entity); err != nil {
			b.Fatalf("Error adding entity: %v", err)
		}

		b.StartTimer()
		tracker.AcceptChanges()
		b.StopTimer()
	}
}

func BenchmarkChangeTracker_MemoryUsage(b *testing.B) {
	tracker := change.NewTracker(true)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		entity := &TestEntityForBench{
			ID:        1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Name:      "Test User",
			Email:     "test@example.com",
			Age:       25,
			IsActive:  true,
		}
		if err := tracker.Add(entity); err != nil {
			b.Fatalf("Error adding entity: %v", err)
		}
	}
}

func BenchmarkChangeTracker_LargeNumberOfEntities(b *testing.B) {
	tracker := change.NewTracker(true)

	const entityCount = 10000

	// Pre-populate
	for i := 0; i < entityCount; i++ {
		entity := &TestEntityForBench{
			ID:        1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Name:      "Test User",
			Email:     "test@example.com",
			Age:       25,
			IsActive:  true,
		}
		if err := tracker.Track(entity); err != nil {
			b.Fatalf("Error tracking entity: %v", err)
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		entity := &TestEntityForBench{
			ID:        int64(i % entityCount),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Name:      "Updated User",
			Email:     "updated@example.com",
			Age:       26,
			IsActive:  false,
		}

		if err := tracker.Update(entity); err != nil {
			b.Fatalf("Error updating entity: %v", err)
		}
	}
}
