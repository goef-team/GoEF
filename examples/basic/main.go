package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/goef-team/goef"
	"github.com/goef-team/goef/migration"
)

// User entity
type User struct {
	goef.BaseEntity
	Name     string `goef:"required,max_length:100"`
	Email    string `goef:"required,unique,max_length:255"`
	Age      int    `goef:"min:0,max:150"`
	IsActive bool   `goef:"default:true"`
}

func (u *User) GetTableName() string {
	return "users"
}

func main() {
	// Create database context
	ctx, err := goef.NewDbContext(
		goef.WithSQLite("example.db"),
		goef.WithChangeTracking(true),
		goef.WithSQLLogging(true),
	)
	if err != nil {
		log.Fatal("Failed to create context:", err)
	}
	defer func() {
		if err = ctx.Close(); err != nil {
			log.Printf("Error closing context: %v", err)
		}
	}()

	migrator := migration.NewMigrator(ctx.DB(), ctx.Dialect(), ctx.Metadata(), nil)

	// Initialize migration system
	if err = migrator.Initialize(context.Background()); err != nil {
		log.Fatal("failed to initialize migrator: %w", err)
	}

	// Generate create migration for all entities
	differ := migration.NewSchemaDiffer(ctx.Dialect(), ctx.Metadata())
	entities := []interface{}{
		&User{},
	}

	createMigration, err := differ.GenerateCreateMigration(entities)
	if err != nil {
		log.Fatal("failed to generate create migration: %w", err)
	}

	// Apply migration
	if err = migrator.ApplyMigrations(context.Background(), []migration.Migration{createMigration}); err != nil {
		log.Fatal("failed to apply migrations: %w", err)
	}

	// Create a new user
	user := &User{
		Name:     "John Doe",
		Email:    "john@example.com",
		Age:      30,
		IsActive: true,
	}
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()

	// Add user to context
	err = ctx.Add(user)
	if err != nil {
		log.Fatal("Failed to add user:", err)
	}

	// Save changes
	affected, err := ctx.SaveChanges()
	if err != nil {
		log.Fatal("Failed to save changes:", err)
	}

	fmt.Printf("Saved %d changes\n", affected)
	fmt.Printf("User ID: %d\n", user.ID)

	// Update user
	user.Name = "Jane Doe"
	user.Age = 25

	err = ctx.Update(user)
	if err != nil {
		log.Fatal("Failed to update user:", err)
	}

	affected, err = ctx.SaveChanges()
	if err != nil {
		log.Fatal("Failed to save update:", err)
	}

	fmt.Printf("Updated %d records\n", affected)

	// Query users
	users := ctx.Set(&User{})
	foundUser, err := users.Find(user.ID)
	if err != nil {
		log.Fatal("Failed to find user:", err)
	}

	if foundUser != nil {
		fmt.Printf("Found user: %+v\n", foundUser)
	}

	// Delete user (soft delete)
	err = ctx.Remove(user)
	if err != nil {
		log.Fatal("Failed to remove user:", err)
	}

	affected, err = ctx.SaveChanges()
	if err != nil {
		log.Fatal("Failed to save delete:", err)
	}

	fmt.Printf("Deleted %d records\n", affected)
}
