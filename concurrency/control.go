// Package concurrency provides concurrency control mechanisms for GoEF including
// optimistic locking, conflict detection, and resolution strategies.
package concurrency

import (
	"fmt"
	"reflect"
	"time"

	"github.com/goef-team/goef/metadata"
)

// ConcurrencyMode defines the concurrency control strategy
type ConcurrencyMode int

const (
	// OptimisticLocking uses version fields to detect conflicts
	OptimisticLocking ConcurrencyMode = iota
	// PessimisticLocking uses database locks
	PessimisticLocking
	// TimestampBased uses last modified timestamps
	TimestampBased
)

// ConflictResolutionStrategy defines how to handle conflicts
type ConflictResolutionStrategy int

const (
	// ThrowException throws an error on conflict
	ThrowException ConflictResolutionStrategy = iota
	// ClientWins overwrites server data with client data
	ClientWins
	// ServerWins discards client changes
	ServerWins
	// MergeChanges attempts to merge non-conflicting changes
	MergeChanges
)

// ConcurrencyManager handles concurrency control
type ConcurrencyManager struct {
	metadata *metadata.Registry
	mode     ConcurrencyMode
	strategy ConflictResolutionStrategy
}

// NewConcurrencyManager creates a new concurrency manager
func NewConcurrencyManager(metadata *metadata.Registry, mode ConcurrencyMode, strategy ConflictResolutionStrategy) *ConcurrencyManager {
	return &ConcurrencyManager{
		metadata: metadata,
		mode:     mode,
		strategy: strategy,
	}
}

// ConcurrencyToken represents a concurrency control token
type ConcurrencyToken struct {
	Version   int64
	Timestamp time.Time
	Checksum  string
}

// ConflictInfo represents information about a concurrency conflict
type ConflictInfo struct {
	Entity       interface{}
	Property     string
	ClientValue  interface{}
	ServerValue  interface{}
	LastModified time.Time
}

// ConcurrencyViolationError represents a concurrency violation
type ConcurrencyViolationError struct {
	Entity    interface{}
	Conflicts []ConflictInfo
	Token     ConcurrencyToken
}

func (e *ConcurrencyViolationError) Error() string {
	return fmt.Sprintf("concurrency violation detected for entity %T with %d conflicts", e.Entity, len(e.Conflicts))
}

// GetConcurrencyToken extracts the concurrency token from an entity
func (cm *ConcurrencyManager) GetConcurrencyToken(entity interface{}) (*ConcurrencyToken, error) {
	entityValue := reflect.ValueOf(entity)
	if entityValue.Kind() == reflect.Ptr {
		entityValue = entityValue.Elem()
	}

	token := &ConcurrencyToken{}

	switch cm.mode {
	case OptimisticLocking:
		// Look for version field
		if versionField := entityValue.FieldByName("Version"); versionField.IsValid() {
			if version, ok := versionField.Interface().(int64); ok {
				token.Version = version
			}
		}

	case TimestampBased:
		// Look for timestamp field
		if timestampField := entityValue.FieldByName("UpdatedAt"); timestampField.IsValid() {
			if timestamp, ok := timestampField.Interface().(time.Time); ok {
				token.Timestamp = timestamp
			}
		}

	case PessimisticLocking:
		// Pessimistic locking doesn't use tokens
		return nil, nil
	}

	// Calculate checksum of entity values
	token.Checksum = cm.calculateChecksum(entity)

	return token, nil
}

// SetConcurrencyToken sets the concurrency token on an entity
func (cm *ConcurrencyManager) SetConcurrencyToken(entity interface{}, token *ConcurrencyToken) error {
	entityValue := reflect.ValueOf(entity)
	if entityValue.Kind() == reflect.Ptr {
		entityValue = entityValue.Elem()
	}

	switch cm.mode {
	case OptimisticLocking:
		if versionField := entityValue.FieldByName("Version"); versionField.IsValid() && versionField.CanSet() {
			versionField.SetInt(token.Version)
		}

	case TimestampBased:
		if timestampField := entityValue.FieldByName("UpdatedAt"); timestampField.IsValid() && timestampField.CanSet() {
			timestampField.Set(reflect.ValueOf(token.Timestamp))
		}
	}

	return nil
}

// IncrementVersion increments the version field of an entity
func (cm *ConcurrencyManager) IncrementVersion(entity interface{}) error {
	if cm.mode != OptimisticLocking {
		return nil
	}

	entityValue := reflect.ValueOf(entity)
	if entityValue.Kind() == reflect.Ptr {
		entityValue = entityValue.Elem()
	}

	if versionField := entityValue.FieldByName("Version"); versionField.IsValid() && versionField.CanSet() {
		currentVersion := versionField.Int()
		versionField.SetInt(currentVersion + 1)
	}

	return nil
}

// UpdateTimestamp updates the timestamp field of an entity
func (cm *ConcurrencyManager) UpdateTimestamp(entity interface{}) error {
	entityValue := reflect.ValueOf(entity)
	if entityValue.Kind() == reflect.Ptr {
		entityValue = entityValue.Elem()
	}

	now := time.Now()

	if timestampField := entityValue.FieldByName("UpdatedAt"); timestampField.IsValid() && timestampField.CanSet() {
		timestampField.Set(reflect.ValueOf(now))
	}

	return nil
}

// DetectConflicts detects concurrency conflicts between client and server entities
func (cm *ConcurrencyManager) DetectConflicts(clientEntity, serverEntity interface{}) ([]ConflictInfo, error) {
	var conflicts []ConflictInfo

	clientValue := reflect.ValueOf(clientEntity)
	if clientValue.Kind() == reflect.Ptr {
		clientValue = clientValue.Elem()
	}

	serverValue := reflect.ValueOf(serverEntity)
	if serverValue.Kind() == reflect.Ptr {
		serverValue = serverValue.Elem()
	}

	if clientValue.Type() != serverValue.Type() {
		return nil, fmt.Errorf("entity types do not match")
	}

	// Compare concurrency tokens first
	clientToken, _ := cm.GetConcurrencyToken(clientEntity)
	serverToken, _ := cm.GetConcurrencyToken(serverEntity)

	if cm.hasTokenConflict(clientToken, serverToken) {
		// Detailed field-by-field comparison
		entityType := clientValue.Type()
		for i := 0; i < entityType.NumField(); i++ {
			field := entityType.Field(i)
			if !field.IsExported() {
				continue
			}

			clientFieldValue := clientValue.Field(i)
			serverFieldValue := serverValue.Field(i)

			if !reflect.DeepEqual(clientFieldValue.Interface(), serverFieldValue.Interface()) {
				conflict := ConflictInfo{
					Entity:       clientEntity,
					Property:     field.Name,
					ClientValue:  clientFieldValue.Interface(),
					ServerValue:  serverFieldValue.Interface(),
					LastModified: time.Now(), // In practice, this would come from the server entity
				}
				conflicts = append(conflicts, conflict)
			}
		}
	}

	return conflicts, nil
}

// hasTokenConflict checks if there's a conflict between concurrency tokens
func (cm *ConcurrencyManager) hasTokenConflict(clientToken, serverToken *ConcurrencyToken) bool {
	if clientToken == nil || serverToken == nil {
		return false
	}

	switch cm.mode {
	case OptimisticLocking:
		return clientToken.Version != serverToken.Version
	case TimestampBased:
		return !clientToken.Timestamp.Equal(serverToken.Timestamp)
	default:
		return clientToken.Checksum != serverToken.Checksum
	}
}

// ResolveConflicts resolves conflicts based on the configured strategy
func (cm *ConcurrencyManager) ResolveConflicts(conflicts []ConflictInfo, clientEntity, serverEntity interface{}) (interface{}, error) {
	if len(conflicts) == 0 {
		return clientEntity, nil
	}

	switch cm.strategy {
	case ThrowException:
		return nil, &ConcurrencyViolationError{
			Entity:    clientEntity,
			Conflicts: conflicts,
		}

	case ClientWins:
		// Client entity wins - just return it
		return clientEntity, nil

	case ServerWins:
		// Server entity wins - return server version
		return serverEntity, nil

	case MergeChanges:
		// Attempt to merge non-conflicting changes
		return cm.mergeEntities(conflicts, clientEntity, serverEntity)

	default:
		return nil, fmt.Errorf("unsupported conflict resolution strategy")
	}
}

// mergeEntities attempts to merge entities, keeping non-conflicting changes
func (cm *ConcurrencyManager) mergeEntities(conflicts []ConflictInfo, clientEntity, serverEntity interface{}) (interface{}, error) {
	// Create a copy of the server entity as the base
	mergedEntity := cm.cloneEntity(serverEntity)

	clientValue := reflect.ValueOf(clientEntity)
	if clientValue.Kind() == reflect.Ptr {
		clientValue = clientValue.Elem()
	}

	mergedValue := reflect.ValueOf(mergedEntity)
	if mergedValue.Kind() == reflect.Ptr {
		mergedValue = mergedValue.Elem()
	}

	// Create a map of conflicting properties
	conflictMap := make(map[string]bool)
	for _, conflict := range conflicts {
		conflictMap[conflict.Property] = true
	}

	// Copy non-conflicting fields from client to merged entity
	entityType := clientValue.Type()
	for i := 0; i < entityType.NumField(); i++ {
		field := entityType.Field(i)
		if !field.IsExported() {
			continue
		}

		// Skip conflicting fields and concurrency control fields
		if conflictMap[field.Name] || cm.isConcurrencyField(field.Name) {
			continue
		}

		clientFieldValue := clientValue.Field(i)
		mergedFieldValue := mergedValue.Field(i)

		if mergedFieldValue.CanSet() {
			mergedFieldValue.Set(clientFieldValue)
		}
	}

	// Update concurrency token
	if serverToken, _ := cm.GetConcurrencyToken(serverEntity); serverToken != nil {
		if err := cm.SetConcurrencyToken(mergedEntity, serverToken); err != nil {
			return nil, err
		}
	}

	return mergedEntity, nil
}

// isConcurrencyField checks if a field is used for concurrency control
func (cm *ConcurrencyManager) isConcurrencyField(fieldName string) bool {
	concurrencyFields := []string{"Version", "UpdatedAt", "RowVersion", "Timestamp"}
	for _, cf := range concurrencyFields {
		if fieldName == cf {
			return true
		}
	}
	return false
}

// cloneEntity creates a deep copy of an entity
func (cm *ConcurrencyManager) cloneEntity(entity interface{}) interface{} {
	entityValue := reflect.ValueOf(entity)
	if entityValue.Kind() == reflect.Ptr {
		// Create a new pointer to the same type
		newEntity := reflect.New(entityValue.Elem().Type())
		newEntity.Elem().Set(entityValue.Elem())
		return newEntity.Interface()
	}

	// For non-pointer types, just return a copy
	return entity
}

// calculateChecksum calculates a simple checksum of entity values
func (cm *ConcurrencyManager) calculateChecksum(entity interface{}) string {
	// This is a simplified checksum implementation
	// In production, you'd use a proper hash function
	entityValue := reflect.ValueOf(entity)
	if entityValue.Kind() == reflect.Ptr {
		entityValue = entityValue.Elem()
	}

	checksum := 0
	entityType := entityValue.Type()
	for i := 0; i < entityType.NumField(); i++ {
		field := entityType.Field(i)
		if !field.IsExported() || cm.isConcurrencyField(field.Name) {
			continue
		}

		fieldValue := entityValue.Field(i)
		checksum += int(reflect.ValueOf(fieldValue.Interface()).Pointer())
	}

	return fmt.Sprintf("%x", checksum)
}

// ConcurrencyMiddleware provides middleware for automatic concurrency control
type ConcurrencyMiddleware struct {
	manager *ConcurrencyManager
}

// NewConcurrencyMiddleware creates concurrency middleware
func NewConcurrencyMiddleware(manager *ConcurrencyManager) *ConcurrencyMiddleware {
	return &ConcurrencyMiddleware{
		manager: manager,
	}
}

// BeforeUpdate is called before updating an entity
func (cm *ConcurrencyMiddleware) BeforeUpdate(clientEntity, serverEntity interface{}) error {
	conflicts, err := cm.manager.DetectConflicts(clientEntity, serverEntity)
	if err != nil {
		return err
	}

	if len(conflicts) > 0 {
		_, err = cm.manager.ResolveConflicts(conflicts, clientEntity, serverEntity)
		if err != nil {
			return err
		}
	}

	// Update concurrency control fields
	if err := cm.manager.IncrementVersion(clientEntity); err != nil {
		return err
	}
	if err := cm.manager.UpdateTimestamp(clientEntity); err != nil {
		return err
	}

	return nil
}
