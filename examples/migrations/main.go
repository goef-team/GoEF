package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/goef-team/goef"
	"github.com/goef-team/goef/migration"
)

// User entity for migration example
type User struct {
	goef.BaseEntity
	Name  string `goef:"required,max_length:100"`
	Email string `goef:"required,unique,max_length:255"`
	Age   int    `goef:"min:0,max:150"`
	Phone string `goef:"max_length:20"` // Will be added in migration 003
}

func (u *User) GetTableName() string {
	return "users"
}

// FIXED: Add proper interface implementations
func (u *User) GetID() interface{} {
	return u.ID
}

func (u *User) SetID(id interface{}) {
	if v, ok := id.(int64); ok {
		u.ID = v
	}
}

// Product entity to demonstrate additional migration
type Product struct {
	goef.BaseEntity
	Name        string  `goef:"required,max_length:200"`
	Description string  `goef:"max_length:1000"`
	Price       float64 `goef:"required"`
	SKU         string  `goef:"required,unique,max_length:50"`
}

func (p *Product) GetTableName() string {
	return "products"
}

// FIXED: Add proper interface implementations
func (p *Product) GetID() interface{} {
	return p.ID
}

func (p *Product) SetID(id interface{}) {
	if v, ok := id.(int64); ok {
		p.ID = v
	}
}

func main() {
	fmt.Println("GoEF Migration Example")
	fmt.Println("=====================")

	// Create database context
	ctx, err := goef.NewDbContext(
		goef.WithSQLite("migrations_example.db"),
		goef.WithChangeTracking(true),
		goef.WithSQLLogging(true),
	)
	if err != nil {
		log.Fatalf("Failed to create DbContext: %v", err)
	}
	defer func() {
		if err = ctx.Close(); err != nil {
			log.Printf("Error closing context: %v", err)
		}
	}()

	// Initialize migrator
	migrator := migration.NewMigrator(
		ctx.DB(),
		ctx.Dialect(),
		ctx.Metadata(),
		&migration.MigratorOptions{
			TableName:          "__goef_migrations",
			ChecksumValidation: true,
			TransactionMode:    true,
			LogVerbose:         true,
		},
	)

	// Initialize migration system
	if err = migrator.Initialize(context.Background()); err != nil {
		log.Fatalf("Failed to initialize migration system: %v", err)
	}

	// Define migrations
	migrations := []migration.Migration{
		{
			ID:          "001_create_users",
			Name:        "CreateUsersTable",
			Version:     1,
			UpSQL:       `CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, name VARCHAR(100) NOT NULL, email VARCHAR(255) NOT NULL UNIQUE, age INTEGER, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME);`,
			DownSQL:     `DROP TABLE users;`,
			Description: "Create users table",
			CreatedAt:   time.Now(),
		},
		{
			ID:          "002_create_products",
			Name:        "CreateProductsTable",
			Version:     2,
			UpSQL:       `CREATE TABLE products (id INTEGER PRIMARY KEY AUTOINCREMENT, name VARCHAR(200) NOT NULL, description VARCHAR(1000), price DECIMAL(10,2) NOT NULL, sku VARCHAR(50) NOT NULL UNIQUE, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME);`,
			DownSQL:     `DROP TABLE products;`,
			Description: "Create products table",
			CreatedAt:   time.Now(),
		},
		{
			ID:          "003_add_user_phone",
			Name:        "AddPhoneToUsers",
			Version:     3,
			UpSQL:       `ALTER TABLE users ADD COLUMN phone VARCHAR(20);`,
			DownSQL:     `ALTER TABLE users DROP COLUMN phone;`,
			Description: "Add phone column to users table",
			CreatedAt:   time.Now(),
		},
	}

	// Get migration status before applying
	fmt.Println("\nMigration Status Before:")
	if err = printMigrationStatus(migrator, migrations); err != nil {
		log.Fatalf("Failed to get migration status: %v", err)
	}

	// Apply migrations
	fmt.Println("\nApplying migrations...")
	if err = migrator.ApplyMigrations(context.Background(), migrations); err != nil {
		log.Fatalf("Failed to apply migrations: %v", err)
	}

	// Get migration status after applying
	fmt.Println("\nMigration Status After:")
	if err = printMigrationStatus(migrator, migrations); err != nil {
		log.Fatalf("Failed to get migration status: %v", err)
	}

	// Create some test data
	fmt.Println("\nCreating test data...")
	user := &User{
		Name:  "John Doe",
		Email: "john@example.com",
		Age:   30,
		Phone: "+1-555-1234", // Using the new phone field
	}
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()

	if err = ctx.Add(user); err != nil {
		log.Fatalf("Failed to add user: %v", err)
	}

	product := &Product{
		Name:        "Sample Product",
		Description: "A sample product for testing",
		Price:       29.99,
		SKU:         "SAMPLE-001",
	}
	product.CreatedAt = time.Now()
	product.UpdatedAt = time.Now()

	if err = ctx.Add(product); err != nil {
		log.Fatalf("Failed to add product: %v", err)
	}

	affected, err := ctx.SaveChanges()
	if err != nil {
		log.Fatalf("Failed to save changes: %v", err)
	}

	fmt.Printf("✅ Created %d entities successfully\n", affected)

	// Demonstrate getting applied migrations
	fmt.Println("\nApplied Migration History:")
	applied, err := migrator.GetAppliedMigrations(context.Background())
	if err != nil {
		log.Fatalf("Failed to get applied migrations: %v", err)
	}

	for _, appliedMig := range applied {
		fmt.Printf("  ✓ %s: %s (applied at %s, took %dms)\n",
			appliedMig.ID,
			appliedMig.Name,
			appliedMig.AppliedAt.Format("2006-01-02 15:04:05"),
			appliedMig.ExecutionMS)
	}

	// Demonstrate rollback (optional - comment out to keep data)
	fmt.Println("\nDemonstrating migration rollback...")
	lastMigration := migrations[len(migrations)-1]
	if err = migrator.RollbackMigration(context.Background(), lastMigration); err != nil {
		log.Printf("Failed to rollback migration (this might be expected for SQLite): %v", err)
	} else {
		fmt.Println("✅ Successfully rolled back last migration")

		// Show status after rollback
		fmt.Println("\nMigration Status After Rollback:")
		if err := printMigrationStatus(migrator, migrations); err != nil {
			log.Fatalf("Failed to get migration status: %v", err)
		}
	}

	fmt.Println("\n🎉 Migration example completed successfully!")
}

// printMigrationStatus displays the current status of all migrations
func printMigrationStatus(migrator *migration.Migrator, migrations []migration.Migration) error {
	status, err := migrator.GetMigrationStatus(context.Background(), migrations)
	if err != nil {
		return err
	}

	fmt.Println("ID                 | Name                    | Status")
	fmt.Println("-------------------|-------------------------|----------")

	for _, mig := range migrations {
		migStatus, exists := status[mig.ID]
		if !exists {
			migStatus = migration.StatusPending
		}

		fmt.Printf("%-18s | %-23s | %s\n",
			mig.ID,
			truncateString(mig.Name, 23),
			migStatus.String())
	}

	return nil
}

// Helper function to truncate strings for display
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
