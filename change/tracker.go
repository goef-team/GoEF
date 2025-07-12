// Package change provides entity state management and change tracking for GoEF
package change

import (
	"fmt"
	"reflect"
	"sync"
	"time"
)

// EntityState represents the state of an entity in the change tracker
type EntityState int

const (
	// StateUnchanged indicates the entity hasn't been modified
	StateUnchanged EntityState = iota
	// StateAdded indicates the entity is new and should be inserted
	StateAdded
	// StateModified indicates the entity has been modified and should be updated
	StateModified
	// StateDeleted indicates the entity should be deleted
	StateDeleted
)

// String returns the string representation of the entity state
func (s EntityState) String() string {
	switch s {
	case StateUnchanged:
		return "Unchanged"
	case StateAdded:
		return "Added"
	case StateModified:
		return "Modified"
	case StateDeleted:
		return "Deleted"
	default:
		return "Unknown"
	}
}

// EntityEntry represents a tracked entity with its state and metadata
type EntityEntry struct {
	Entity             interface{}
	State              EntityState
	OriginalValues     map[string]interface{}
	CurrentValues      map[string]interface{}
	ModifiedProperties []string
	Timestamp          time.Time
}

// Tracker manages entity change tracking for the Unit of Work pattern
type Tracker struct {
	enabled  bool
	entities map[string]*EntityEntry // keyed by entity identity
	mu       sync.RWMutex
}

// NewTracker creates a new change tracker
func NewTracker(enabled bool) *Tracker {
	return &Tracker{
		enabled:  enabled,
		entities: make(map[string]*EntityEntry),
	}
}

// Add tracks a new entity for insertion - FIXED VERSION
func (t *Tracker) Add(entity interface{}) error {
	if !t.enabled {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	key := t.getEntityKey(entity)
	if key == "" {
		return fmt.Errorf("cannot track entity without valid key")
	}

	// FIXED: Check if entity is already tracked
	if existing, exists := t.entities[key]; exists {
		// Allow re-adding entities that are marked for deletion
		if existing.State == StateDeleted {
			existing.State = StateAdded
			existing.ModifiedProperties = nil
			existing.Timestamp = time.Now()
			existing.CurrentValues = t.captureValues(entity)
			return nil
		}

		// If already being tracked in any other state, return error
		return fmt.Errorf("entity is already being tracked")
	}

	// Create new entry
	entry := &EntityEntry{
		Entity:         entity,
		State:          StateAdded,
		CurrentValues:  t.captureValues(entity),
		OriginalValues: t.captureValues(entity), // For new entities, original = current
		Timestamp:      time.Now(),
	}

	t.entities[key] = entry
	return nil
}

// Track starts tracking an existing entity (from database)
func (t *Tracker) Track(entity interface{}) error {
	if !t.enabled {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	key := t.getEntityKey(entity)
	if key == "" {
		return fmt.Errorf("cannot track entity without valid key")
	}

	// Don't track if already tracking
	if _, exists := t.entities[key]; exists {
		return nil
	}

	values := t.captureValues(entity)
	entry := &EntityEntry{
		Entity:         entity,
		State:          StateUnchanged,
		OriginalValues: values,
		CurrentValues:  values,
		Timestamp:      time.Now(),
	}

	t.entities[key] = entry
	return nil
}

// Update marks an entity for update - FIXED VERSION
func (t *Tracker) Update(entity interface{}) error {
	if !t.enabled {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	key := t.getEntityKey(entity)
	if key == "" {
		return fmt.Errorf("cannot track entity without valid key")
	}

	if existing, exists := t.entities[key]; exists {
		// If entity is already being tracked
		switch existing.State {
		case StateAdded:
			// If it's a new entity, keep it as added but update the current values
			existing.Entity = entity
			existing.CurrentValues = t.captureValues(entity)
			existing.Timestamp = time.Now()
		case StateModified:
			// Update the current values and modified time
			existing.Entity = entity
			existing.CurrentValues = t.captureValues(entity)
			existing.Timestamp = time.Now()
			// Recalculate modified properties
			existing.ModifiedProperties = t.detectChanges(existing.OriginalValues, existing.CurrentValues)
		case StateUnchanged:
			// Mark as modified
			existing.State = StateModified
			existing.Entity = entity
			existing.CurrentValues = t.captureValues(entity)
			existing.Timestamp = time.Now()
			existing.ModifiedProperties = t.detectChanges(existing.OriginalValues, existing.CurrentValues)
		case StateDeleted:
			return fmt.Errorf("cannot update a deleted entity")
		}
	} else {
		// Entity not being tracked, start tracking it as modified
		currentValues := t.captureValues(entity)
		entry := &EntityEntry{
			Entity:             entity,
			State:              StateModified,
			OriginalValues:     currentValues, // We don't have original values, so use current
			CurrentValues:      currentValues,
			ModifiedProperties: []string{}, // Will be set by DetectChanges if needed
			Timestamp:          time.Now(),
		}
		t.entities[key] = entry
	}

	return nil
}

// Remove marks an entity for deletion
func (t *Tracker) Remove(entity interface{}) error {
	if !t.enabled {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	key := t.getEntityKey(entity)
	if key == "" {
		return fmt.Errorf("cannot track entity without valid key")
	}

	if existing, exists := t.entities[key]; exists {
		if existing.State == StateAdded {
			// If it was just added, remove it completely
			delete(t.entities, key)
		} else {
			// Mark for deletion
			existing.State = StateDeleted
			existing.Timestamp = time.Now()
		}
	} else {
		// Entity not tracked, track it as deleted
		entry := &EntityEntry{
			Entity:         entity,
			State:          StateDeleted,
			OriginalValues: t.captureValues(entity),
			CurrentValues:  t.captureValues(entity),
			Timestamp:      time.Now(),
		}
		t.entities[key] = entry
	}

	return nil
}

// GetChanges returns all tracked changes
func (t *Tracker) GetChanges() []*EntityEntry {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var changes []*EntityEntry
	for _, entry := range t.entities {
		if entry.State != StateUnchanged {
			changes = append(changes, entry)
		}
	}

	return changes
}

// HasChanges returns true if there are tracked changes
func (t *Tracker) HasChanges() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, entry := range t.entities {
		if entry.State != StateUnchanged {
			return true
		}
	}
	return false
}

// GetEntry returns the entry for a specific entity
func (t *Tracker) GetEntry(entity interface{}) (*EntityEntry, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	key := t.getEntityKey(entity)
	entry, exists := t.entities[key]
	return entry, exists
}

// AcceptChanges marks all changes as accepted and resets tracking state
func (t *Tracker) AcceptChanges() {
	t.mu.Lock()
	defer t.mu.Unlock()

	for key, entry := range t.entities {
		if entry.State == StateDeleted {
			// Remove deleted entities from tracking
			delete(t.entities, key)
		} else {
			// Mark as unchanged and update original values
			entry.State = StateUnchanged
			entry.OriginalValues = entry.CurrentValues
			entry.ModifiedProperties = nil
		}
	}
}

// DetectChanges detects changes in tracked entities
func (t *Tracker) DetectChanges() error {
	if !t.enabled {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	for _, entry := range t.entities {
		if entry.State == StateUnchanged || entry.State == StateModified {
			currentValues := t.captureValues(entry.Entity)
			modifiedProps := t.detectChanges(entry.OriginalValues, currentValues)

			if len(modifiedProps) > 0 {
				entry.State = StateModified
				entry.ModifiedProperties = modifiedProps
				entry.CurrentValues = currentValues
				entry.Timestamp = time.Now()
			}
		}
	}

	return nil
}

// getEntityKey generates a unique key for an entity based on its primary key - FIXED VERSION
func (t *Tracker) getEntityKey(entity interface{}) string {
	v := reflect.ValueOf(entity)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return ""
	}

	entityType := v.Type()

	// Try to get ID field
	if idField := v.FieldByName("ID"); idField.IsValid() && idField.CanInterface() {
		id := idField.Interface()
		// For new entities (ID = 0), use memory address as key
		if isZeroID(id) {
			return fmt.Sprintf("%s:@%p", entityType.Name(), entity)
		}
		return fmt.Sprintf("%s:%v", entityType.Name(), id)
	}

	// Try to call GetID method
	if method := reflect.ValueOf(entity).MethodByName("GetID"); method.IsValid() {
		results := method.Call(nil)
		if len(results) > 0 {
			id := results[0].Interface()
			// For new entities (ID = 0), use memory address as key
			if isZeroID(id) {
				return fmt.Sprintf("%s:@%p", entityType.Name(), entity)
			}
			return fmt.Sprintf("%s:%v", entityType.Name(), id)
		}
	}

	// Fallback to memory address
	return fmt.Sprintf("%s:@%p", entityType.Name(), entity)
}

// isZeroID checks if an ID value is considered "zero" (unassigned)
func isZeroID(id interface{}) bool {
	if id == nil {
		return true
	}

	v := reflect.ValueOf(id)
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.String:
		return v.String() == ""
	default:
		return false
	}
}

// captureValues captures all field values of an entity
func (t *Tracker) captureValues(entity interface{}) map[string]interface{} {
	values := make(map[string]interface{})

	v := reflect.ValueOf(entity)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return values
	}

	entityType := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := entityType.Field(i)
		if !field.IsExported() {
			continue
		}

		fieldValue := v.Field(i)
		if fieldValue.CanInterface() {
			values[field.Name] = fieldValue.Interface()
		}
	}

	return values
}

// detectChanges compares original and current values to find modifications
func (t *Tracker) detectChanges(original, current map[string]interface{}) []string {
	var modified []string

	for key, currentValue := range current {
		originalValue, exists := original[key]
		if !exists || !t.valuesEqual(originalValue, currentValue) {
			modified = append(modified, key)
		}
	}

	return modified
}

// valuesEqual compares two values for equality, handling special cases
func (t *Tracker) valuesEqual(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Handle time.Time specially for precision issues
	if timeA, okA := a.(time.Time); okA {
		if timeB, okB := b.(time.Time); okB {
			return timeA.Equal(timeB)
		}
	}

	// Handle pointers to time.Time
	if reflect.TypeOf(a) == reflect.TypeOf((*time.Time)(nil)) {
		ptrA := a.(*time.Time)
		ptrB := b.(*time.Time)
		if ptrA == nil && ptrB == nil {
			return true
		}
		if ptrA == nil || ptrB == nil {
			return false
		}
		return ptrA.Equal(*ptrB)
	}

	// Use reflect.DeepEqual for other types
	return reflect.DeepEqual(a, b)
}

// ValuesEqual is a public method to expose value comparison logic
func (t *Tracker) ValuesEqual(a, b interface{}) bool {
	return t.valuesEqual(a, b)
}

// Clear removes all tracked entities
func (t *Tracker) Clear() {
	if !t.enabled {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.entities = make(map[string]*EntityEntry)
}
