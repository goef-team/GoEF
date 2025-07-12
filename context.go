// Package goef provides a production-grade, EF Core-style ORM for Go.
//
// GoEF implements type-safe querying, change tracking, migrations, and
// relationship mapping for multiple database backends including PostgreSQL,
// MySQL, SQLite, MSSQL, and MongoDB.
//
// Basic usage:
//
//	type User struct {
//	    ID   int    `goef:"primary_key"`
//	    Name string `goef:"required"`
//	}
//
//	ctx := goef.NewDbContext(goef.WithPostgreSQL("connection_string"))
//	user := &User{Name: "John"}
//	ctx.Add(user)
//	err := ctx.SaveChanges()
package goef

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/goef-team/goef/batch"

	"github.com/goef-team/goef/change"
	"github.com/goef-team/goef/dialects"
	"github.com/goef-team/goef/loader"
	"github.com/goef-team/goef/metadata"
	"github.com/goef-team/goef/query"
)

// DbContext represents a session with the database, providing change tracking,
// querying, and Unit of Work functionality. It is not thread-safe and should
// be used per request/operation scope.
type DbContext struct {
	db            *sql.DB
	dialect       dialects.Dialect
	metadata      *metadata.Registry
	changeTracker *change.Tracker
	loader        *loader.Loader
	options       *Options
	tx            *sql.Tx
	closed        bool
	mu            sync.RWMutex
}

func (dbContext *DbContext) DB() *sql.DB {
	return dbContext.db
}

func (dbContext *DbContext) Dialect() dialects.Dialect {
	return dbContext.dialect
}

func (dbContext *DbContext) Metadata() *metadata.Registry {
	return dbContext.metadata
}

func (dbContext *DbContext) ChangeTracker() *change.Tracker {
	return dbContext.changeTracker
}

func (dbContext *DbContext) Loader() *loader.Loader {
	return dbContext.loader
}

func (dbContext *DbContext) Options() *Options {
	return dbContext.options
}

func (dbContext *DbContext) Tx() *sql.Tx {
	return dbContext.tx
}

func (dbContext *DbContext) Closed() bool {
	return dbContext.closed
}

// NewDbContext creates a new database context with the specified configuration.
func NewDbContext(opts ...Option) (*DbContext, error) {
	options := &Options{
		ChangeTracking: true,
		LazyLoading:    false,
		AutoMigrate:    false,
		LogSQL:         false,
		Logger:         &DefaultLogger{},
	}

	for _, opt := range opts {
		if err := opt(options); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	var db *sql.DB
	var err error

	if options.DB != nil {
		db = options.DB
		if options.DriverName == "" {
			// DriverName is crucial for selecting the correct dialect.
			// If using an existing DB, DriverName should still be provided via options.
			return nil, fmt.Errorf("DriverName must be specified when using WithDB option")
		}
		// It's assumed the provided db is already pinged and valid.
		// If not, pinging here could be an option:
		// if err := db.Ping(); err != nil {
		// 	return nil, fmt.Errorf("failed to ping provided database: %w", err)
		// }
	} else {
		// Fallback to original logic if options.DB is not provided
		if options.DriverName == "mongodb" {
			// MongoDB dialect handles its own client, doesn't need sql.Open here for the core DB context's db field.
			// The db field might remain nil for MongoDB if not used by other parts of DbContext.
			// However, the loader.New currently takes *sql.DB. This needs to be consistent.
			// For now, we'll let db be nil for mongo if not using WithDB.
			// This part of the logic might need a broader review for MongoDB consistency.
		} else {
			if options.ConnectionString == "" {
				return nil, ErrInvalidConnectionString
			}
			db, err = sql.Open(options.DriverName, options.ConnectionString)
			if err != nil {
				return nil, fmt.Errorf("failed to open database: %w", err)
			}

			if err = db.Ping(); err != nil {
				if cerr := db.Close(); cerr != nil {
					_ = cerr
				}
				return nil, fmt.Errorf("failed to ping database: %w", err)
			}
		}
	}

	// Ensure DriverName is available for dialect creation
	if options.DriverName == "" {
		return nil, fmt.Errorf("DriverName is required to create a dialect")
	}

	dialect, err := dialects.New(options.DriverName, options.DialectOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create dialect: %w", err)
	}

	metadataRegistry := metadata.NewRegistry()
	changeTracker := change.NewTracker(options.ChangeTracking)
	loader := loader.New(db, dialect, metadataRegistry)

	return &DbContext{
		db:            db,
		dialect:       dialect,
		metadata:      metadataRegistry,
		changeTracker: changeTracker,
		loader:        loader,
		options:       options,
	}, nil
}

// Set returns a DbSet for the specified entity type
func (dbContext *DbContext) Set(entityType interface{}) *DbSet {
	return &DbSet{
		ctx:      dbContext,
		metadata: dbContext.metadata.GetOrCreate(entityType),
		builder:  query.NewBuilder(dbContext.dialect, dbContext.metadata, dbContext.db),
	}
}

// Add marks the specified entity for insertion
func (dbContext *DbContext) Add(entity Entity) error {
	dbContext.mu.Lock()
	defer dbContext.mu.Unlock()

	if dbContext.closed {
		return ErrContextClosed
	}

	return dbContext.changeTracker.Add(entity)
}

// Update marks the specified entity for update
func (dbContext *DbContext) Update(entity Entity) error {
	dbContext.mu.Lock()
	defer dbContext.mu.Unlock()

	if dbContext.closed {
		return ErrContextClosed
	}

	return dbContext.changeTracker.Update(entity)
}

// Remove marks the specified entity for deletion
func (dbContext *DbContext) Remove(entity Entity) error {
	dbContext.mu.Lock()
	defer dbContext.mu.Unlock()

	if dbContext.closed {
		return ErrContextClosed
	}

	return dbContext.changeTracker.Remove(entity)
}

// SaveChanges persists all changes tracked by this context to the database
func (dbContext *DbContext) SaveChanges() (int, error) {
	return dbContext.SaveChangesWithContext(context.Background())
}

// SaveChangesWithContext persists all changes with the provided context
func (dbContext *DbContext) SaveChangesWithContext(ctxCancel context.Context) (int, error) {
	dbContext.mu.Lock()
	defer dbContext.mu.Unlock()

	if dbContext.closed {
		return 0, ErrContextClosed
	}

	changes := dbContext.changeTracker.GetChanges()
	if len(changes) == 0 {
		return 0, nil
	}

	// FIXED: Use existing transaction if available, otherwise create new one
	var tx *sql.Tx
	var shouldCommit bool
	var err error

	if dbContext.tx != nil {
		// Use existing transaction (started with BeginTransaction)
		tx = dbContext.tx
		shouldCommit = false // Don't commit here, let explicit CommitTransaction() handle it
	} else {
		// Create new transaction for this SaveChanges operation
		tx, err = dbContext.db.BeginTx(ctxCancel, nil)
		if err != nil {
			return 0, fmt.Errorf("failed to begin transaction: %w", err)
		}
		shouldCommit = true // We created it, so we should commit it
	}

	// Handle rollback on error (only if we created the transaction)
	var affectedRows int
	var processingError error

	defer func() {
		if processingError != nil && shouldCommit {
			if err := tx.Rollback(); err != nil {
				_ = err
			}
		}
	}()

	// Use the Batch Executor within the transaction
	batchExecutor := batch.NewExecutor(dbContext.dialect, dbContext.metadata, tx)
	affectedRows, processingError = batchExecutor.ProcessChanges(ctxCancel, changes)

	if processingError != nil {
		return 0, processingError
	}

	// Only commit if we created the transaction (not part of user-managed transaction)
	if shouldCommit {
		if err := tx.Commit(); err != nil {
			return 0, fmt.Errorf("failed to commit transaction: %w", err)
		}
	}

	dbContext.changeTracker.AcceptChanges()
	return affectedRows, nil
}

// executeInsert executes an INSERT statement for the given entity
func (dbContext *DbContext) executeInsert(ctxCancel context.Context, tx *sql.Tx, entity Entity) (int, error) {
	meta := dbContext.metadata.GetOrCreate(entity)

	var fields []string
	var values []interface{}

	entityValue := reflect.ValueOf(entity)
	if entityValue.Kind() == reflect.Ptr {
		entityValue = entityValue.Elem()
	}

	// Find auto-increment field
	var autoIncrementField *metadata.FieldMetadata
	for _, field := range meta.Fields {
		if field.IsAutoIncrement {
			autoIncrementField = &field
			continue // Skip auto-increment fields in INSERT
		}

		fieldValue := entityValue.FieldByName(field.Name)
		if !fieldValue.IsValid() {
			continue
		}

		fields = append(fields, field.ColumnName)
		values = append(values, fieldValue.Interface())
	}

	sql, args, err := dbContext.dialect.BuildInsert(meta.TableName, fields, values)
	if err != nil {
		return 0, err
	}

	if dbContext.options.LogSQL {
		dbContext.LogSQL(sql, args)
	}

	result, err := tx.ExecContext(ctxCancel, sql, args...)
	if err != nil {
		return 0, err
	}

	// FIXED: Assign the generated ID back to the entity
	if autoIncrementField != nil {
		lastID, err := result.LastInsertId()
		if err == nil {
			idField := entityValue.FieldByName(autoIncrementField.Name)
			if idField.IsValid() && idField.CanSet() {
				idField.SetInt(lastID)
			}
		}
	}

	rows, _ := result.RowsAffected()
	return int(rows), nil
}

// executeUpdate executes an UPDATE statement for the given entity
func (dbContext *DbContext) executeUpdate(ctxCancel context.Context, tx *sql.Tx, entity Entity, modifiedFields []string) (int, error) {
	meta := dbContext.metadata.GetOrCreate(entity)

	// Get the entity entry from change tracker
	entry, exists := dbContext.changeTracker.GetEntry(entity)
	if !exists {
		return 0, fmt.Errorf("entity not tracked for update")
	}

	if len(entry.ModifiedProperties) == 0 {
		return 0, nil // Nothing to update
	}

	var fields []string
	var values []interface{}

	entityValue := reflect.ValueOf(entity)
	if entityValue.Kind() == reflect.Ptr {
		entityValue = entityValue.Elem()
	}

	// Only update modified fields
	for _, propName := range entry.ModifiedProperties {
		for _, field := range meta.Fields {
			if field.Name == propName && !field.IsPrimaryKey && !field.IsRelationship {
				fieldValue := entityValue.FieldByName(field.Name)
				if fieldValue.IsValid() {
					fields = append(fields, field.ColumnName)
					values = append(values, fieldValue.Interface())
				}
				break
			}
		}
	}

	if len(fields) == 0 {
		return 0, nil // Nothing to update
	}

	// Build WHERE clause using primary key
	if len(meta.PrimaryKeys) == 0 {
		return 0, fmt.Errorf("cannot update entity without primary key")
	}

	pkField := meta.PrimaryKeys[0]
	pkValue := entityValue.FieldByName(pkField.Name)
	if !pkValue.IsValid() {
		return 0, fmt.Errorf("primary key field %s not found", pkField.Name)
	}

	whereClause := fmt.Sprintf("%s = %s", dbContext.dialect.Quote(pkField.ColumnName), dbContext.dialect.Placeholder(len(values)+1))
	whereArgs := []interface{}{pkValue.Interface()}

	sql, args, err := dbContext.dialect.BuildUpdate(meta.TableName, fields, values, whereClause, whereArgs)
	if err != nil {
		return 0, err
	}

	if dbContext.options.LogSQL {
		dbContext.LogSQL(sql, args)
	}

	result, err := tx.ExecContext(ctxCancel, sql, args...)
	if err != nil {
		return 0, err
	}

	rows, _ := result.RowsAffected()
	return int(rows), nil
}

// executeDelete executes a DELETE statement for the given entity
func (dbContext *DbContext) executeDelete(ctxCancel context.Context, tx *sql.Tx, entity Entity) (int, error) {
	meta := dbContext.metadata.GetOrCreate(entity)

	entityValue := reflect.ValueOf(entity)
	if entityValue.Kind() == reflect.Ptr {
		entityValue = entityValue.Elem()
	}

	// Build WHERE clause for primary keys
	var whereClause strings.Builder
	var whereArgs []interface{}
	argIndex := 1

	for i, pkField := range meta.PrimaryKeys {
		if i > 0 {
			whereClause.WriteString(" AND ")
		}
		whereClause.WriteString(fmt.Sprintf("%s = %s", dbContext.dialect.Quote(pkField.ColumnName), dbContext.dialect.Placeholder(argIndex)))
		fieldValue := entityValue.FieldByName(pkField.Name)
		whereArgs = append(whereArgs, fieldValue.Interface())
		argIndex++
	}

	sql, args, err := dbContext.dialect.BuildDelete(meta.TableName, whereClause.String(), whereArgs)
	if err != nil {
		return 0, err
	}

	if dbContext.options.LogSQL {
		dbContext.LogSQL(sql, args)
	}

	result, err := tx.ExecContext(ctxCancel, sql, args...)
	if err != nil {
		return 0, err
	}

	rows, _ := result.RowsAffected()
	return int(rows), nil
}

func (dbContext *DbContext) LogSQL(sql string, args []interface{}) {
	if !dbContext.options.LogSQL {
		return
	}

	// Enhanced logging with timestamp and context
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")

	// Always provide basic console output if enabled
	fmt.Printf("[%s] SQL: %s\nArgs: %v\n", timestamp, sql, args)

	// FIXED: Use logger if available and not nil
	if dbContext.options.Logger != nil {
		dbContext.options.Logger.Debug("SQL executed: %s with args: %v", sql, args)
	}
}

// BeginTransaction begins a new transaction
func (dbContext *DbContext) BeginTransaction() error {
	return dbContext.BeginTransactionWithContext(context.Background())
}

// BeginTransactionWithContext begins a new transaction with context
func (dbContext *DbContext) BeginTransactionWithContext(ctxCancel context.Context) error {
	dbContext.mu.Lock()
	defer dbContext.mu.Unlock()

	if dbContext.closed {
		return ErrContextClosed
	}

	if dbContext.tx != nil {
		return ErrTransactionAlreadyStarted
	}

	tx, err := dbContext.db.BeginTx(ctxCancel, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	dbContext.tx = tx
	return nil
}

// CommitTransaction commits the current transaction
func (dbContext *DbContext) CommitTransaction() error {
	dbContext.mu.Lock()
	defer dbContext.mu.Unlock()

	if dbContext.tx == nil {
		return ErrNoActiveTransaction
	}

	err := dbContext.tx.Commit()
	dbContext.tx = nil

	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// RollbackTransaction rolls back the current transaction
func (dbContext *DbContext) RollbackTransaction() error {
	dbContext.mu.Lock()
	defer dbContext.mu.Unlock()

	if dbContext.tx == nil {
		return ErrNoActiveTransaction
	}

	err := dbContext.tx.Rollback()
	dbContext.tx = nil

	if err != nil {
		return fmt.Errorf("failed to rollback transaction: %w", err)
	}

	// FIXED: Clear change tracker on rollback since changes were not persisted
	dbContext.changeTracker.Clear()

	return nil
}

// Close closes the database connection
func (dbContext *DbContext) Close() error {
	dbContext.mu.Lock()
	defer dbContext.mu.Unlock()

	if dbContext.closed {
		return nil
	}

	if dbContext.tx != nil {
		if err := dbContext.tx.Rollback(); err != nil {
			_ = err
		}
	}

	dbContext.closed = true
	return dbContext.db.Close()
}

// In context.go - Add methods to use the single operations for specific cases

// Add a new method for single entity operations
func (dbContext *DbContext) SaveEntity(entity Entity) (int, error) {
	return dbContext.SaveEntityWithContext(context.Background(), entity)
}

// SaveEntityWithContext saves a single entity using legacy methods
func (dbContext *DbContext) SaveEntityWithContext(ctx context.Context, entity Entity) (int, error) {
	dbContext.mu.Lock()
	defer dbContext.mu.Unlock()

	if dbContext.closed {
		return 0, ErrContextClosed
	}

	entry, exists := dbContext.changeTracker.GetEntry(entity)
	if !exists {
		return 0, fmt.Errorf("entity not tracked")
	}

	// FIXED: Use existing transaction if available, otherwise create new one
	var tx *sql.Tx
	var shouldCommit bool
	var err error

	if dbContext.tx != nil {
		tx = dbContext.tx
		shouldCommit = false
	} else {
		tx, err = dbContext.db.BeginTx(ctx, nil)
		if err != nil {
			return 0, fmt.Errorf("failed to begin transaction: %w", err)
		}
		shouldCommit = true
	}

	var affectedRows int
	var processingError error

	defer func() {
		if processingError != nil && shouldCommit {
			if err := tx.Rollback(); err != nil {
				dbContext.LogSQL("ROLLBACK FAILED", []interface{}{err})
			}
		}
	}()

	// FIXED: Use the individual execute methods based on entity state
	switch entry.State {
	case change.StateAdded:
		affectedRows, processingError = dbContext.executeInsert(ctx, tx, entity)
	case change.StateModified:
		affectedRows, processingError = dbContext.executeUpdate(ctx, tx, entity, entry.ModifiedProperties)
	case change.StateDeleted:
		affectedRows, processingError = dbContext.executeDelete(ctx, tx, entity)
	default:
		return 0, nil // No changes
	}

	if processingError != nil {
		return 0, processingError
	}

	// Only commit if we created the transaction
	if shouldCommit {
		if err := tx.Commit(); err != nil {
			return 0, fmt.Errorf("failed to commit transaction: %w", err)
		}
	}

	// FIXED: Use logSQL for tracking
	if dbContext.options.LogSQL {
		dbContext.LogSQL("SINGLE_ENTITY_OPERATION_COMPLETED", []interface{}{entry.State, affectedRows})
	}

	return affectedRows, nil
}
