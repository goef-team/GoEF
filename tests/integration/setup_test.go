package integration

import (
	"database/sql"
	"testing"
	"time"

	"github.com/goef-team/goef"
	_ "github.com/mattn/go-sqlite3"
)

type User struct {
	ID        int64      `goef:"primary_key,auto_increment" json:"id"`
	CreatedAt time.Time  `goef:"created_at" json:"created_at"`
	UpdatedAt time.Time  `goef:"updated_at" json:"updated_at"`
	DeletedAt *time.Time `goef:"deleted_at,soft_delete" json:"deleted_at,omitempty"`
	Name      string     `goef:"required,max_length:100"`
	Email     string     `goef:"required,unique,max_length:255"`
	Age       int        `goef:"min:0,max:150"`
	IsActive  bool       `goef:"default:true"`
}

// GetID returns the primary key value
func (u *User) GetID() interface{} {
	return u.ID
}

// SetID sets the primary key value
func (u *User) SetID(id interface{}) {
	if v, ok := id.(int64); ok {
		u.ID = v
	}
}

func (u *User) GetTableName() string {
	return "users"
}

// setupTestDB creates a test database with all necessary tables
func setupTestDB(t *testing.T) (*goef.DbContext, func()) {
	// Create a unique database file for each test to avoid conflicts
	dbFile := ":memory:"

	// Create temporary SQLite database
	db, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Verify connection
	if err = db.Ping(); err != nil {
		err = db.Close()
		if err != nil {
			t.Fatalf("error on closing db, err: %v", err)
		}
		t.Fatalf("Failed to ping test database: %v", err)
	}

	// Create the users table manually for all tests
	_, err = db.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL UNIQUE,
			age INTEGER NOT NULL DEFAULT 0,
			is_active BOOLEAN NOT NULL DEFAULT true,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			deleted_at DATETIME
		);
	`)
	if err != nil {
		err = db.Close()
		if err != nil {
			t.Fatalf("error on closing db, err: %v", err)
		}
		t.Fatalf("Failed to create users table: %v", err)
	}

	// Create context - IMPORTANT: Don't set connection string when using WithDB
	ctx, err := goef.NewDbContext(
		goef.WithSQLite(""), // Empty connection string since we're providing the DB
		goef.WithDB(db),
		goef.WithChangeTracking(true),
		goef.WithSQLLogging(true), // Enable for debugging
	)
	if err != nil {
		err = db.Close()
		if err != nil {
			t.Fatalf("error on closing db, err: %v", err)
		}
		t.Fatalf("Failed to create DbContext: %v", err)
	}

	// Critical: Verify the context is using our database
	testDB := ctx.DB()
	if testDB == nil {
		err = db.Close()
		if err != nil {
			t.Fatalf("error on closing db, err: %v", err)
		}
		err = ctx.Close()
		if err != nil {
			t.Fatalf("error on closing eontext, err: %v", err)
		}
		t.Fatal("DbContext database is nil")
	}

	// Verify table exists in the context's database
	var count int
	err = testDB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='users'").Scan(&count)
	if err != nil {
		err = db.Close()
		if err != nil {
			t.Fatalf("error on closing db, err: %v", err)
		}
		err = ctx.Close()
		if err != nil {
			t.Fatalf("error on closing eontext, err: %v", err)
		}
		t.Fatalf("Failed to verify users table exists: %v", err)
	}
	if count != 1 {
		err = db.Close()
		if err != nil {
			t.Fatalf("error on closing db, err: %v", err)
		}
		err = ctx.Close()
		if err != nil {
			t.Fatalf("error on closing eontext, err: %v", err)
		}
		t.Fatalf("Users table was not created properly, count: %d", count)
	}

	cleanup := func() {
		err = ctx.Close()
		if err != nil {
			t.Fatalf("error on closing eontext, err: %v", err)
		}
	}

	return ctx, cleanup
}

// Helper function to create and save a test user with proper change tracking
func createTestUser(t *testing.T, ctx *goef.DbContext, name, email string, age int) *User {
	user := &User{
		Name:     name,
		Email:    email,
		Age:      age,
		IsActive: true,
	}
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()

	err := ctx.Add(user)
	if err != nil {
		t.Fatalf("Failed to add user %s: %v", name, err)
	}

	affected, err := ctx.SaveChanges()
	if err != nil {
		t.Fatalf("Failed to save user %s: %v", name, err)
	}

	if affected != 1 {
		t.Fatalf("Expected 1 affected row for user %s, got %d", name, affected)
	}

	if user.ID == 0 {
		t.Fatalf("User %s should have ID assigned after save", name)
	}

	return user
}

// setupMultiTableTestDB creates a test database with multiple tables for complex integration tests
func setupMultiTableTestDB(t *testing.T) (*goef.DbContext, func()) {
	// Create temporary SQLite database
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Verify connection
	if err = db.Ping(); err != nil {
		err = db.Close()
		if err != nil {
			t.Fatalf("error on closing db, err: %v", err)
		}
		t.Fatalf("Failed to ping test database: %v", err)
	}

	// Create multiple tables including relationships
	_, err = db.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL UNIQUE,
			age INTEGER NOT NULL DEFAULT 0,
			is_active BOOLEAN NOT NULL DEFAULT true,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			deleted_at DATETIME
		);

		CREATE TABLE departments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			deleted_at DATETIME
		);

		CREATE TABLE employees (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			department_id INTEGER NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			deleted_at DATETIME,
			FOREIGN KEY(department_id) REFERENCES departments(id)
		);

               CREATE TABLE __goef_migrations (
                       id TEXT PRIMARY KEY,
                       name TEXT NOT NULL,
                       version INTEGER NOT NULL,
                       applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
                       execution_ms INTEGER NOT NULL DEFAULT 0,
                       checksum TEXT NOT NULL,
                       success BOOLEAN NOT NULL DEFAULT 0,
                       error TEXT,
                       rolled_back_at DATETIME,
                       created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
                       UNIQUE(version)
               );
	`)
	if err != nil {
		err = db.Close()
		if err != nil {
			t.Fatalf("error on closing db, err: %v", err)
		}
		t.Fatalf("Failed to create tables: %v", err)
	}

	// Create context
	ctx, err := goef.NewDbContext(
		goef.WithSQLite(""), // Empty connection string since we're providing the DB
		goef.WithDB(db),
		goef.WithChangeTracking(true),
		goef.WithLazyLoading(true),
		goef.WithSQLLogging(true),
	)
	if err != nil {
		err = db.Close()
		if err != nil {
			t.Fatalf("error on closing db, err: %v", err)
		}
		t.Fatalf("Failed to create DbContext: %v", err)
	}

	// Verify tables exist
	tables := []string{"users", "departments", "employees"}
	for _, table := range tables {
		var count int
		err = ctx.DB().QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count)
		if err != nil {
			err = db.Close()
			if err != nil {
				t.Fatalf("error on closing db, err: %v", err)
			}
			err = ctx.Close()
			if err != nil {
				t.Fatalf("error on closing context, err: %v", err)
			}
			t.Fatalf("Failed to verify table %s exists: %v", table, err)
		}
		if count != 1 {
			err = db.Close()
			if err != nil {
				t.Fatalf("error on closing db, err: %v", err)
			}
			err = ctx.Close()
			if err != nil {
				t.Fatalf("error on closing context, err: %v", err)
			}
			t.Fatalf("Table %s was not created properly, count: %d", table, count)
		}
	}

	cleanup := func() {
		err = ctx.Close()
		if err != nil {
			t.Fatalf("error on closing context, err: %v", err)
		}
	}

	return ctx, cleanup
}

// insertTestUsers creates multiple test users for testing
func insertTestUsers(t *testing.T, ctx *goef.DbContext) []*User {
	users := []*User{
		createTestUser(t, ctx, "John Doe", "john@example.com", 30),
		createTestUser(t, ctx, "Jane Smith", "jane@example.com", 25),
		createTestUser(t, ctx, "Bob Johnson", "bob@example.com", 35),
	}

	return users
}
