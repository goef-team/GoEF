package unit

import (
	"testing"
	"time"

	"github.com/goef-team/goef/change"
)

func TestChangeTracker_Add(t *testing.T) {
	tracker := change.NewTracker(true)

	entity := &testEntity{
		Name:  "John Doe",
		Email: "john@example.com",
	}

	err := tracker.Add(entity)
	if err != nil {
		t.Fatalf("Failed to add entity: %v", err)
	}

	changes := tracker.GetChanges()
	if len(changes) != 1 {
		t.Fatalf("Expected 1 change, got %d", len(changes))
	}

	entityChange := changes[0]
	if entityChange.State != change.StateAdded {
		t.Errorf("Expected StateAdded, got %v", entityChange.State)
	}

	if entityChange.Entity != entity {
		t.Error("Change should reference the original entity")
	}
}

func TestChangeTracker_Update(t *testing.T) {
	tracker := change.NewTracker(true)

	entity := &testEntity{
		ID:    1,
		Name:  "John Doe",
		Email: "john@example.com",
	}

	// Track the entity first
	err := tracker.Track(entity)
	if err != nil {
		t.Fatalf("Failed to track entity: %v", err)
	}

	// Modify the entity
	entity.Name = "Jane Doe"

	err = tracker.Update(entity)
	if err != nil {
		t.Fatalf("Failed to update entity: %v", err)
	}

	changes := tracker.GetChanges()
	if len(changes) != 1 {
		t.Fatalf("Expected 1 change, got %d", len(changes))
	}

	entityChange := changes[0]
	if entityChange.State != change.StateModified {
		t.Errorf("Expected StateModified, got %v", entityChange.State)
	}

	if len(entityChange.ModifiedProperties) == 0 {
		t.Error("Expected modified properties to be tracked")
	}

	found := false
	for _, prop := range entityChange.ModifiedProperties {
		if prop == "Name" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'Name' to be in modified properties")
	}
}

func TestChangeTracker_Remove(t *testing.T) {
	tracker := change.NewTracker(true)

	entity := &testEntity{
		ID:    1,
		Name:  "John Doe",
		Email: "john@example.com",
	}

	// Track the entity first
	err := tracker.Track(entity)
	if err != nil {
		t.Fatalf("Failed to track entity: %v", err)
	}

	err = tracker.Remove(entity)
	if err != nil {
		t.Fatalf("Failed to remove entity: %v", err)
	}

	changes := tracker.GetChanges()
	if len(changes) != 1 {
		t.Fatalf("Expected 1 change, got %d", len(changes))
	}

	entityChange := changes[0]
	if entityChange.State != change.StateDeleted {
		t.Errorf("Expected StateDeleted, got %v", entityChange.State)
	}
}

func TestChangeTracker_AddThenRemove(t *testing.T) {
	tracker := change.NewTracker(true)

	entity := &testEntity{
		Name:  "John Doe",
		Email: "john@example.com",
	}

	// Add entity
	err := tracker.Add(entity)
	if err != nil {
		t.Fatalf("Failed to add entity: %v", err)
	}

	// Remove entity (should just untrack it since it was never saved)
	err = tracker.Remove(entity)
	if err != nil {
		t.Fatalf("Failed to remove entity: %v", err)
	}

	changes := tracker.GetChanges()
	if len(changes) != 0 {
		t.Errorf("Expected no changes after add+remove, got %d", len(changes))
	}
}

func TestChangeTracker_AcceptChanges(t *testing.T) {
	tracker := change.NewTracker(true)

	entity := &testEntity{
		Name:  "John Doe",
		Email: "john@example.com",
	}

	err := tracker.Add(entity)
	if err != nil {
		t.Fatalf("Failed to add entity: %v", err)
	}

	// Should have changes before accepting
	if !tracker.HasChanges() {
		t.Error("Expected to have changes")
	}

	tracker.AcceptChanges()

	// Should have no changes after accepting
	if tracker.HasChanges() {
		t.Error("Expected no changes after AcceptChanges")
	}

	// Entity should now be tracked as unchanged
	entry, exists := tracker.GetEntry(entity)
	if !exists {
		t.Fatal("Expected entity to still be tracked")
	}

	if entry.State != change.StateUnchanged {
		t.Errorf("Expected StateUnchanged after AcceptChanges, got %v", entry.State)
	}
}

func TestChangeTracker_DetectChanges(t *testing.T) {
	tracker := change.NewTracker(true)

	entity := &testEntity{
		ID:    1,
		Name:  "John Doe",
		Email: "john@example.com",
	}

	err := tracker.Track(entity)
	if err != nil {
		t.Fatalf("Failed to track entity: %v", err)
	}

	// Modify entity directly (without calling Update)
	entity.Name = "Jane Doe"
	entity.Age = 25

	// Manually detect changes
	err = tracker.DetectChanges()
	if err != nil {
		t.Fatalf("Failed to detect changes: %v", err)
	}

	changes := tracker.GetChanges()
	if len(changes) != 1 {
		t.Fatalf("Expected 1 change, got %d", len(changes))
	}

	entityChange := changes[0]
	if entityChange.State != change.StateModified {
		t.Errorf("Expected StateModified, got %v", entityChange.State)
	}

	expectedModified := []string{"Name", "Age"}
	if len(entityChange.ModifiedProperties) != len(expectedModified) {
		t.Errorf("Expected %d modified properties, got %d", len(expectedModified), len(entityChange.ModifiedProperties))
	}
}

func TestChangeTracker_Disabled(t *testing.T) {
	tracker := change.NewTracker(false) // Disabled

	entity := &testEntity{
		Name:  "John Doe",
		Email: "john@example.com",
	}

	err := tracker.Add(entity)
	if err != nil {
		t.Fatalf("Failed to add entity: %v", err)
	}

	// Should have no changes when disabled
	changes := tracker.GetChanges()
	if len(changes) != 0 {
		t.Errorf("Expected no changes when tracking disabled, got %d", len(changes))
	}

	if tracker.HasChanges() {
		t.Error("Expected no changes when tracking disabled")
	}
}

func TestChangeTracker_ValuesEqual(t *testing.T) {
	tracker := change.NewTracker(true)

	// Test time comparison
	time1 := time.Now()
	time2 := time1
	time3 := time1.Add(time.Second)

	if !tracker.ValuesEqual(time1, time2) {
		t.Error("Expected equal times to be equal")
	}

	if tracker.ValuesEqual(time1, time3) {
		t.Error("Expected different times to be different")
	}

	// Test nil comparison
	if !tracker.ValuesEqual(nil, nil) {
		t.Error("Expected nil values to be equal")
	}

	if tracker.ValuesEqual(nil, "value") {
		t.Error("Expected nil and non-nil to be different")
	}

	// Test basic types
	if !tracker.ValuesEqual("test", "test") {
		t.Error("Expected equal strings to be equal")
	}

	if tracker.ValuesEqual("test1", "test2") {
		t.Error("Expected different strings to be different")
	}
}
