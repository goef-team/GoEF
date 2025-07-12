package unit

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/goef-team/goef/dialects"
	"github.com/goef-team/goef/metadata"
	"github.com/goef-team/goef/migration"
	_ "github.com/mattn/go-sqlite3"
)

func TestMigrator_Initialize(t *testing.T) {
	db, err := sql.Open("sqlite3", "file:test.db?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func(db *sql.DB) {
		err = db.Close()
		if err != nil {
			t.Fatalf("error on closing db, err: %v", err)
		}
	}(db)

	dialect := dialects.NewSQLiteDialect(nil)
	metadataRegistry := metadata.NewRegistry()

	migrator := migration.NewMigrator(db, dialect, metadataRegistry, nil)

	ctx := context.Background()
	err = migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize migrator: %v", err)
	}

	// Check that migration table was created
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='__goef_migrations'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query migration table: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected migration table to be created, but count was %d", count)
	}
}

func TestMigrator_ApplyMigrations(t *testing.T) {
	db, err := sql.Open("sqlite3", "file:test.db?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func(db *sql.DB) {
		err = db.Close()
		if err != nil {
			t.Fatalf("error on closing db, err: %v", err)
		}
	}(db)

	dialect := dialects.NewSQLiteDialect(nil)
	metadataRegistry := metadata.NewRegistry()

	migrator := migration.NewMigrator(db, dialect, metadataRegistry, nil)

	ctx := context.Background()
	err = migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize migrator: %v", err)
	}

	// Create test migrations
	migrations := []migration.Migration{
		{
			ID:          "001_create_users",
			Name:        "CreateUsers",
			Version:     1,
			UpSQL:       "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL);",
			DownSQL:     "DROP TABLE users;",
			Description: "Create users table",
			Checksum:    "abc123",
		},
		{
			ID:          "002_add_email_to_users",
			Name:        "AddEmailToUsers",
			Version:     2,
			UpSQL:       "ALTER TABLE users ADD COLUMN email TEXT;",
			DownSQL:     "ALTER TABLE users DROP COLUMN email;",
			Description: "Add email column to users",
			Checksum:    "def456",
		},
	}

	err = migrator.ApplyMigrations(ctx, migrations)
	if err != nil {
		t.Fatalf("Failed to apply migrations: %v", err)
	}

	// Check that users table was created
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='users'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query users table: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected users table to be created, but count was %d", count)
	}

	// Check that migrations were recorded
	applied, err := migrator.GetAppliedMigrations(ctx)
	if err != nil {
		t.Fatalf("Failed to get applied migrations: %v", err)
	}

	if len(applied) != 2 {
		t.Errorf("Expected 2 applied migrations, got %d", len(applied))
	}

	// Check that pending migrations are empty
	pending, err := migrator.GetPendingMigrations(ctx, migrations)
	if err != nil {
		t.Fatalf("Failed to get pending migrations: %v", err)
	}

	if len(pending) != 0 {
		t.Errorf("Expected 0 pending migrations, got %d", len(pending))
	}
}

func TestMigrator_RollbackMigration(t *testing.T) {
	db, err := sql.Open("sqlite3", "file:test.db?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func(db *sql.DB) {
		err = db.Close()
		if err != nil {
			t.Fatalf("error on closing db, err: %v", err)
		}
	}(db)

	dialect := dialects.NewSQLiteDialect(nil)
	metadataRegistry := metadata.NewRegistry()

	migrator := migration.NewMigrator(db, dialect, metadataRegistry, nil)

	ctx := context.Background()
	err = migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize migrator: %v", err)
	}

	// Apply a migration first
	testMigration := migration.Migration{
		ID:          "001_create_test_table",
		Name:        "CreateTestTable",
		Version:     1,
		UpSQL:       "CREATE TABLE test_table (id INTEGER PRIMARY KEY);",
		DownSQL:     "DROP TABLE test_table;",
		Description: "Create test table",
		Checksum:    "test123",
	}

	err = migrator.ApplyMigrations(ctx, []migration.Migration{testMigration})
	if err != nil {
		t.Fatalf("Failed to apply migration: %v", err)
	}

	// Rollback the migration
	err = migrator.RollbackMigration(ctx, testMigration)
	if err != nil {
		t.Fatalf("Failed to rollback migration: %v", err)
	}

	// Check that table was dropped
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='test_table'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query test_table: %v", err)
	}

	if count != 0 {
		t.Errorf("Expected test_table to be dropped, but count was %d", count)
	}
}

func TestSchemaDiffer_GenerateCreateMigration(t *testing.T) {
	dialect := dialects.NewSQLiteDialect(nil)
	metadataRegistry := metadata.NewRegistry()

	differ := migration.NewSchemaDiffer(dialect, metadataRegistry)

	entities := []interface{}{
		&testEntity{},
	}

	migrationObj, err := differ.GenerateCreateMigration(entities)
	if err != nil {
		t.Fatalf("Failed to generate create migration: %v", err)
	}

	if migrationObj.Name != "InitialCreate" {
		t.Errorf("Expected migration name 'InitialCreate', got %s", migrationObj.Name)
	}

	if migrationObj.UpSQL == "" {
		t.Error("Expected non-empty UpSQL")
	}

	if migrationObj.DownSQL == "" {
		t.Error("Expected non-empty DownSQL")
	}

	// Check that UpSQL contains CREATE TABLE
	if !strings.Contains(migrationObj.UpSQL, "CREATE TABLE") {
		t.Error("Expected UpSQL to contain CREATE TABLE")
	}

	// Check that DownSQL contains DROP TABLE
	if !strings.Contains(migrationObj.DownSQL, "DROP TABLE") {
		t.Error("Expected DownSQL to contain DROP TABLE")
	}
}

func TestMigrationStatus_String(t *testing.T) {
	tests := []struct {
		status   migration.MigrationStatus
		expected string
	}{
		{migration.StatusPending, "Pending"},
		{migration.StatusApplied, "Applied"},
		{migration.StatusFailed, "Failed"},
	}

	for _, test := range tests {
		t.Run(test.expected, func(t *testing.T) {
			if test.status.String() != test.expected {
				t.Errorf("Expected: %s, got: %s", test.expected, test.status.String())
			}
		})
	}
}
