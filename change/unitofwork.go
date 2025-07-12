// Package change provides Unit of Work pattern implementation for GoEF.
package change

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"github.com/goef-team/goef/dialects"
	"github.com/goef-team/goef/metadata"
)

// UnitOfWork represents a unit of work that maintains a list of objects
// affected by a business transaction and coordinates writing out changes
type UnitOfWork struct {
	db              *sql.DB
	dialect         dialects.Dialect
	metadata        *metadata.Registry
	tracker         *Tracker
	stateManager    *EntityStateManager
	insertQueue     []interface{}
	updateQueue     []interface{}
	deleteQueue     []interface{}
	tx              *sql.Tx
	isInTransaction bool
	mu              sync.RWMutex
}

// NewUnitOfWork creates a new unit of work
func NewUnitOfWork(db *sql.DB, dialect dialects.Dialect, metadata *metadata.Registry) *UnitOfWork {
	return &UnitOfWork{
		db:           db,
		dialect:      dialect,
		metadata:     metadata,
		tracker:      NewTracker(true),
		stateManager: NewEntityStateManager(),
		insertQueue:  make([]interface{}, 0),
		updateQueue:  make([]interface{}, 0),
		deleteQueue:  make([]interface{}, 0),
	}
}

// RegisterNew registers a new entity for insertion
func (uow *UnitOfWork) RegisterNew(entity interface{}) error {
	uow.mu.Lock()
	defer uow.mu.Unlock()

	if err := uow.tracker.Add(entity); err != nil {
		return fmt.Errorf("failed to track new entity: %w", err)
	}

	uow.insertQueue = append(uow.insertQueue, entity)

	entityKey := uow.generateEntityKey(entity)
	uow.stateManager.SetState(entityKey, StateAdded)

	return nil
}

// RegisterDirty registers an entity for update
func (uow *UnitOfWork) RegisterDirty(entity interface{}) error {
	uow.mu.Lock()
	defer uow.mu.Unlock()

	if err := uow.tracker.Update(entity); err != nil {
		return fmt.Errorf("failed to track dirty entity: %w", err)
	}

	// Check if already in update queue
	for _, existing := range uow.updateQueue {
		if uow.entitiesEqual(existing, entity) {
			return nil
		}
	}

	uow.updateQueue = append(uow.updateQueue, entity)

	entityKey := uow.generateEntityKey(entity)
	uow.stateManager.SetState(entityKey, StateModified)

	return nil
}

// RegisterRemoved registers an entity for deletion
func (uow *UnitOfWork) RegisterRemoved(entity interface{}) error {
	uow.mu.Lock()
	defer uow.mu.Unlock()

	if err := uow.tracker.Remove(entity); err != nil {
		return fmt.Errorf("failed to track removed entity: %w", err)
	}

	uow.deleteQueue = append(uow.deleteQueue, entity)

	entityKey := uow.generateEntityKey(entity)
	uow.stateManager.SetState(entityKey, StateDeleted)

	return nil
}

// Commit commits all changes in the unit of work
func (uow *UnitOfWork) Commit(ctx context.Context) error {
	uow.mu.Lock()
	defer uow.mu.Unlock()

	if !uow.hasChanges() {
		return nil
	}

	// Begin transaction if not already in one
	if !uow.isInTransaction {
		tx, err := uow.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
		uow.tx = tx
		uow.isInTransaction = true
	}

	defer func() {
		if uow.isInTransaction {
			if r := recover(); r != nil {
				if err := uow.tx.Rollback(); err != nil {
					_ = err
				}
				uow.isInTransaction = false
				panic(r)
			}
		}
	}()

	// Execute inserts
	if err := uow.executeInserts(ctx); err != nil {
		if err = uow.tx.Rollback(); err != nil {
			_ = err
		}
		uow.isInTransaction = false
		return fmt.Errorf("failed to execute inserts: %w", err)
	}

	// Execute updates
	if err := uow.executeUpdates(ctx); err != nil {
		if err = uow.tx.Rollback(); err != nil {
			_ = err
		}
		uow.isInTransaction = false
		return fmt.Errorf("failed to execute updates: %w", err)
	}

	// Execute deletes
	if err := uow.executeDeletes(ctx); err != nil {
		if err = uow.tx.Rollback(); err != nil {
			_ = err
		}
		uow.isInTransaction = false
		return fmt.Errorf("failed to execute deletes: %w", err)
	}

	// Commit transaction
	if err := uow.tx.Commit(); err != nil {
		uow.isInTransaction = false
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	uow.isInTransaction = false
	uow.clearQueues()
	uow.tracker.AcceptChanges()

	return nil
}

// Rollback rolls back all changes in the unit of work
func (uow *UnitOfWork) Rollback() error {
	uow.mu.Lock()
	defer uow.mu.Unlock()

	if uow.isInTransaction && uow.tx != nil {
		if err := uow.tx.Rollback(); err != nil {
			return fmt.Errorf("failed to rollback transaction: %w", err)
		}
		uow.isInTransaction = false
	}

	uow.clearQueues()
	uow.tracker.Clear()
	uow.stateManager.Clear()

	return nil
}

// hasChanges checks if there are any pending changes
func (uow *UnitOfWork) hasChanges() bool {
	return len(uow.insertQueue) > 0 || len(uow.updateQueue) > 0 || len(uow.deleteQueue) > 0
}

// executeInserts executes all pending inserts
func (uow *UnitOfWork) executeInserts(ctx context.Context) error {
	for _, entity := range uow.insertQueue {
		if err := uow.executeInsert(ctx, entity); err != nil {
			return fmt.Errorf("failed to insert entity: %w", err)
		}
	}
	return nil
}

// executeUpdates executes all pending updates
func (uow *UnitOfWork) executeUpdates(ctx context.Context) error {
	for _, entity := range uow.updateQueue {
		if err := uow.executeUpdate(ctx, entity); err != nil {
			return fmt.Errorf("failed to update entity: %w", err)
		}
	}
	return nil
}

// executeDeletes executes all pending deletes
func (uow *UnitOfWork) executeDeletes(ctx context.Context) error {
	for _, entity := range uow.deleteQueue {
		if err := uow.executeDelete(ctx, entity); err != nil {
			return fmt.Errorf("failed to delete entity: %w", err)
		}
	}
	return nil
}

// executeInsert executes a single insert
func (uow *UnitOfWork) executeInsert(ctx context.Context, entity interface{}) error {
	meta := uow.metadata.GetOrCreate(entity)

	var fields []string
	var values []interface{}

	// Extract field values (simplified implementation)
	for _, field := range meta.Fields {
		if !field.IsAutoIncrement {
			fields = append(fields, field.ColumnName)
			values = append(values, uow.getFieldValue(entity, field.Name))
		}
	}

	buildInsert, args, err := uow.dialect.BuildInsert(meta.TableName, fields, values)
	if err != nil {
		return err
	}

	_, err = uow.tx.ExecContext(ctx, buildInsert, args...)
	return err
}

// executeUpdate executes a single update
func (uow *UnitOfWork) executeUpdate(ctx context.Context, entity interface{}) error {
	meta := uow.metadata.GetOrCreate(entity)

	// Get modified fields from tracker
	entry, exists := uow.tracker.GetEntry(entity)
	if !exists {
		return fmt.Errorf("entity not tracked")
	}

	var fields []string
	var values []interface{}

	for _, fieldName := range entry.ModifiedProperties {
		if field := meta.FieldsByName[fieldName]; field != nil && !field.IsPrimaryKey {
			fields = append(fields, field.ColumnName)
			values = append(values, uow.getFieldValue(entity, field.Name))
		}
	}

	// Build WHERE clause for primary keys
	whereClause := "id = ?"
	whereArgs := []interface{}{uow.getFieldValue(entity, "ID")}

	buildUpdate, args, err := uow.dialect.BuildUpdate(meta.TableName, fields, values, whereClause, whereArgs)
	if err != nil {
		return err
	}

	_, err = uow.tx.ExecContext(ctx, buildUpdate, args...)
	return err
}

// executeDelete executes a single delete
func (uow *UnitOfWork) executeDelete(ctx context.Context, entity interface{}) error {
	meta := uow.metadata.GetOrCreate(entity)

	whereClause := "id = ?"
	whereArgs := []interface{}{uow.getFieldValue(entity, "ID")}

	buildDelete, args, err := uow.dialect.BuildDelete(meta.TableName, whereClause, whereArgs)
	if err != nil {
		return err
	}

	_, err = uow.tx.ExecContext(ctx, buildDelete, args...)
	return err
}

// Helper methods
func (uow *UnitOfWork) clearQueues() {
	uow.insertQueue = uow.insertQueue[:0]
	uow.updateQueue = uow.updateQueue[:0]
	uow.deleteQueue = uow.deleteQueue[:0]
}

func (uow *UnitOfWork) generateEntityKey(entity interface{}) string {
	return fmt.Sprintf("%T:%v", entity, uow.getFieldValue(entity, "ID"))
}

func (uow *UnitOfWork) entitiesEqual(a, b interface{}) bool {
	return uow.generateEntityKey(a) == uow.generateEntityKey(b)
}

func (uow *UnitOfWork) getFieldValue(entity interface{}, fieldName string) interface{} {
	// Simplified reflection-based field access
	// In production, use cached reflection
	return nil
}
