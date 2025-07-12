package integration

import (
	"testing"
	"time"

	"github.com/goef-team/goef/change"
)

func TestDbContext_IntegrationCRUD(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cleanup := setupTestDB(t)
	defer cleanup()

	// Test Create
	user := createTestUser(t, ctx, "John Doe", "john@example.com", 30)

	// Test that change tracking is working
	if ctx.ChangeTracker().HasChanges() {
		t.Error("Expected no changes after SaveChanges")
	}

	// Test Update - FIXED: Proper change tracking sequence
	originalID := user.ID

	// Start tracking for update
	err := ctx.ChangeTracker().Track(user)
	if err != nil {
		t.Fatalf("Failed to track user for update: %v", err)
	}

	// Modify the entity
	user.Name = "Jane Doe"
	user.Age = 25
	user.UpdatedAt = time.Now()

	// Mark as modified
	err = ctx.Update(user)
	if err != nil {
		t.Fatalf("Failed to update user: %v", err)
	}

	affected, err := ctx.SaveChanges()
	if err != nil {
		t.Fatalf("Failed to save update: %v", err)
	}

	if affected != 1 {
		t.Errorf("Expected 1 affected row for update, got %d", affected)
	}

	// Verify ID didn't change
	if user.ID != originalID {
		t.Errorf("User ID changed during update: %d -> %d", originalID, user.ID)
	}

	// Test Delete
	err = ctx.Remove(user)
	if err != nil {
		t.Fatalf("Failed to remove user: %v", err)
	}

	affected, err = ctx.SaveChanges()
	if err != nil {
		t.Fatalf("Failed to save delete: %v", err)
	}

	if affected != 1 {
		t.Errorf("Expected 1 affected row for delete, got %d", affected)
	}
}

func TestDbContext_Transaction(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cleanup := setupTestDB(t)
	defer cleanup()

	// Test successful transaction
	err := ctx.BeginTransaction()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	user1 := &User{
		Name:     "John Doe",
		Email:    "john@example.com",
		Age:      30,
		IsActive: true,
	}
	user1.CreatedAt = time.Now()
	user1.UpdatedAt = time.Now()

	user2 := &User{
		Name:     "Jane Doe",
		Email:    "jane@example.com",
		Age:      25,
		IsActive: true,
	}
	user2.CreatedAt = time.Now()
	user2.UpdatedAt = time.Now()

	err = ctx.Add(user1)
	if err != nil {
		t.Fatalf("Failed to add user1: %v", err)
	}

	err = ctx.Add(user2)
	if err != nil {
		t.Fatalf("Failed to add user2: %v", err)
	}

	// FIXED: SaveChanges should now use the existing transaction
	affected, err := ctx.SaveChanges()
	if err != nil {
		t.Fatalf("Failed to save changes in transaction: %v", err)
	}

	if affected != 2 {
		t.Errorf("Expected 2 affected rows, got %d", affected)
	}

	err = ctx.CommitTransaction()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	// Verify both users have IDs
	if user1.ID == 0 || user2.ID == 0 {
		t.Error("Users should have IDs assigned after transaction commit")
	}

	// Test rollback transaction
	err = ctx.BeginTransaction()
	if err != nil {
		t.Fatalf("Failed to begin second transaction: %v", err)
	}

	user3 := &User{
		Name:     "Bob Smith",
		Email:    "bob@example.com",
		Age:      35,
		IsActive: false,
	}
	user3.CreatedAt = time.Now()
	user3.UpdatedAt = time.Now()

	err = ctx.Add(user3)
	if err != nil {
		t.Fatalf("Failed to add user3: %v", err)
	}

	// Don't save - just rollback
	err = ctx.RollbackTransaction()
	if err != nil {
		t.Fatalf("Failed to rollback transaction: %v", err)
	}

	// Changes should be rolled back
	if ctx.ChangeTracker().HasChanges() {
		t.Error("Expected no changes after rollback")
	}

	t.Log("Transaction test completed successfully")
}

func TestDbContext_ChangeTrackingIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cleanup := setupTestDB(t)
	defer cleanup()

	// Create and save a user
	user := createTestUser(t, ctx, "John Doe", "john@example.com", 30)

	// Start tracking for update
	err := ctx.ChangeTracker().Track(user)
	if err != nil {
		t.Fatalf("Failed to track user: %v", err)
	}

	// Modify the user
	user.Name = "Jane Doe"
	user.Age = 25
	user.UpdatedAt = time.Now()

	// Manually trigger update tracking
	err = ctx.Update(user)
	if err != nil {
		t.Fatalf("Failed to mark user as updated: %v", err)
	}

	// Changes should be detected
	if !ctx.ChangeTracker().HasChanges() {
		t.Error("Expected changes to be detected")
	}

	changes := ctx.ChangeTracker().GetChanges()
	if len(changes) != 1 {
		t.Errorf("Expected 1 change, got %d", len(changes))
	}

	entityChange := changes[0]
	if entityChange.State != change.StateModified {
		t.Errorf("Expected StateModified, got %v", entityChange.State)
	}

	// Save the changes
	affected, err := ctx.SaveChanges()
	if err != nil {
		t.Fatalf("Failed to save changes: %v", err)
	}

	if affected != 1 {
		t.Errorf("Expected 1 affected row, got %d", affected)
	}

	// Should have no changes after saving
	if ctx.ChangeTracker().HasChanges() {
		t.Error("Expected no changes after SaveChanges")
	}
}
