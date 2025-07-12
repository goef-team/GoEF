package integration

import (
	"context"
	"testing"
	"time"

	"github.com/goef-team/goef/loader"
	"github.com/goef-team/goef/migration"
	"github.com/goef-team/goef/query"
)

// Define test entities for advanced tests
type Department struct {
	ID        int64      `goef:"primary_key,auto_increment" json:"id"`
	CreatedAt time.Time  `goef:"created_at" json:"created_at"`
	UpdatedAt time.Time  `goef:"updated_at" json:"updated_at"`
	DeletedAt *time.Time `goef:"deleted_at,soft_delete" json:"deleted_at,omitempty"`
	Name      string     `goef:"required"`
	Employees []Employee `goef:"relationship:one_to_many,foreign_key:department_id"`
}

func (d *Department) GetTableName() string {
	return "departments"
}

type Employee struct {
	ID           int64       `goef:"primary_key,auto_increment" json:"id"`
	CreatedAt    time.Time   `goef:"created_at" json:"created_at"`
	UpdatedAt    time.Time   `goef:"updated_at" json:"updated_at"`
	DeletedAt    *time.Time  `goef:"deleted_at,soft_delete" json:"deleted_at,omitempty"`
	Name         string      `goef:"required"`
	DepartmentID int64       `goef:"required"`
	Department   *Department `goef:"relationship:many_to_one,foreign_key:department_id"`
}

func (e *Employee) GetTableName() string {
	return "employees"
}

// FIXED: Add proper interface implementations
func (e *Employee) GetID() interface{} {
	return e.ID
}

func TestAdvancedQueryBuilder_LINQ(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cleanup := setupTestDB(t)
	defer cleanup()

	// Create test users using the helper function
	users := insertTestUsers(t, ctx)

	// Test LINQ-style querying
	queryable := query.NewQueryable[User](ctx.Dialect(), ctx.Metadata(), ctx.DB())

	// Test simple Where clause
	results, err := queryable.Where("age", ">", 25).ToList()
	if err != nil {
		t.Fatalf("Failed to execute LINQ query: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results (John and Bob), got %d", len(results))
	}

	// Test Count
	count, err := queryable.Count()
	if err != nil {
		t.Fatalf("Failed to execute Count query: %v", err)
	}

	if count != 3 {
		t.Errorf("Expected count of 3, got %d", count)
	}

	// Test Any
	hasUsers, err := queryable.Any()
	if err != nil {
		t.Fatalf("Failed to execute Any query: %v", err)
	}

	if !hasUsers {
		t.Error("Expected Any to return true")
	}

	t.Logf("Successfully tested LINQ operations with %d users", len(users))
}

func TestRelationshipLoading_EndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// FIXED: Use setupMultiTableTestDB instead of setupTestDB
	ctx, cleanup := setupMultiTableTestDB(t)
	defer cleanup()

	// Insert test data
	_, err := ctx.DB().Exec(`
		INSERT INTO departments (name, created_at, updated_at) VALUES ('Engineering', datetime('now'), datetime('now'));
		INSERT INTO employees (name, department_id, created_at, updated_at) VALUES 
			('Alice', 1, datetime('now'), datetime('now')),
			('Bob', 1, datetime('now'), datetime('now'));
	`)
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	// Create relationship loader
	relationLoader := loader.New(ctx.DB(), ctx.Dialect(), ctx.Metadata())

	// Load department with ID
	department := &Department{
		Name: "Engineering",
	}
	department.ID = 1 // Set the ID manually for testing

	// Load employees relationship
	err = relationLoader.LoadRelation(context.Background(), department, "Employees")
	if err != nil {
		t.Fatalf("Failed to load employees relationship: %v", err)
	}

	if len(department.Employees) != 2 {
		t.Errorf("Expected 2 employees, got %d", len(department.Employees))
	}

	t.Log("Relationship loading test completed successfully")
}

func TestMigrationSystem_EndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// FIXED: Use setupMultiTableTestDB which includes migrations table
	ctx, cleanup := setupMultiTableTestDB(t)
	defer cleanup()

	// Create migrator
	migrator := migration.NewMigrator(ctx.DB(), ctx.Dialect(), ctx.Metadata(), nil)

	// Initialize migration system (if needed)
	err := migrator.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Failed to initialize migrator: %v", err)
	}

	// Create test migration
	testMigration := migration.Migration{
		ID:          "001_create_products",
		Name:        "CreateProducts",
		Version:     1,
		UpSQL:       "CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT NOT NULL);",
		DownSQL:     "DROP TABLE products;",
		Description: "Create products table",
		Checksum:    "test123",
	}

	// Apply migration
	err = migrator.ApplyMigrations(context.Background(), []migration.Migration{testMigration})
	if err != nil {
		t.Fatalf("Failed to apply migrations: %v", err)
	}

	// Verify table was created
	var count int
	err = ctx.DB().QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='products'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query products table: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected products table to be created, but count was %d", count)
	}

	// Test rollback
	err = migrator.RollbackMigration(context.Background(), testMigration)
	if err != nil {
		t.Fatalf("Failed to rollback migration: %v", err)
	}

	// Verify table was dropped
	err = ctx.DB().QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='products'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query products table after rollback: %v", err)
	}

	if count != 0 {
		t.Errorf("Expected products table to be dropped, but count was %d", count)
	}

	t.Log("Migration system test completed successfully")
}

func TestTransactionWithRollback(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cleanup := setupTestDB(t)
	defer cleanup()

	// Begin transaction
	err := ctx.BeginTransaction()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	// Add some users
	user1 := &User{Name: "TxUser1", Email: "tx1@example.com", Age: 25, IsActive: true}
	user2 := &User{Name: "TxUser2", Email: "tx2@example.com", Age: 30, IsActive: true}

	for _, user := range []*User{user1, user2} {
		user.CreatedAt = time.Now()
		user.UpdatedAt = time.Now()
		err = ctx.Add(user)
		if err != nil {
			t.Fatalf("error on adding user, err: %v", err)
		}
	}

	// Save changes within transaction (should use existing transaction now)
	affected, err := ctx.SaveChanges()
	if err != nil {
		t.Fatalf("Failed to save changes in transaction: %v", err)
	}

	if affected != 2 {
		t.Errorf("Expected 2 affected rows, got %d", affected)
	}

	// Rollback transaction
	err = ctx.RollbackTransaction()
	if err != nil {
		t.Fatalf("Failed to rollback transaction: %v", err)
	}

	// Verify users were not persisted
	var count int
	err = ctx.DB().QueryRow("SELECT COUNT(*) FROM users WHERE name LIKE 'TxUser%'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count users: %v", err)
	}

	if count != 0 {
		t.Errorf("Expected 0 users after rollback, got %d", count)
	}

	t.Log("Transaction rollback test completed successfully")
}
