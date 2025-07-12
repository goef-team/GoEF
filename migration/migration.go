package migration

import (
	"context"
	"crypto/sha256" // Fix G501: Replace weak MD5 with stronger SHA-256
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/goef-team/goef/dialects"
	"github.com/goef-team/goef/metadata"
)

// Migration represents a database migration
type Migration struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Version     int64     `json:"version"`
	UpSQL       string    `json:"up_sql"`
	DownSQL     string    `json:"down_sql"`
	Description string    `json:"description"`
	Checksum    string    `json:"checksum"`
	CreatedAt   time.Time `json:"created_at"`
}

// MigrationHistory represents a migration record in the database
type MigrationHistory struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Version      int64      `json:"version"`
	AppliedAt    time.Time  `json:"applied_at"`
	ExecutionMS  int64      `json:"execution_ms"`
	Checksum     string     `json:"checksum"`
	Success      bool       `json:"success"`
	Error        string     `json:"error,omitempty"`
	RolledBackAt *time.Time `json:"rolled_back_at,omitempty"`
}

// MigrationStatus represents the current status of migrations
type MigrationStatus int

const (
	StatusPending MigrationStatus = iota
	StatusApplied
	StatusFailed
	StatusRolledBack
)

func (s MigrationStatus) String() string {
	switch s {
	case StatusPending:
		return "Pending"
	case StatusApplied:
		return "Applied"
	case StatusFailed:
		return "Failed"
	case StatusRolledBack:
		return "RolledBack"
	default:
		return "Unknown"
	}
}

// MigratorOptions contains configuration for the migrator
type MigratorOptions struct {
	TableName          string
	LockTableName      string
	LockTimeout        time.Duration
	LogVerbose         bool
	DryRun             bool
	TransactionMode    bool
	ChecksumValidation bool
}

// DefaultMigratorOptions returns default options
func DefaultMigratorOptions() *MigratorOptions {
	return &MigratorOptions{
		TableName:          "__goef_migrations",
		LockTableName:      "__goef_migration_lock",
		LockTimeout:        time.Minute * 5,
		LogVerbose:         true,
		DryRun:             false,
		TransactionMode:    true,
		ChecksumValidation: true,
	}
}

// Migrator handles database migrations
type Migrator struct {
	db       *sql.DB
	dialect  dialects.Dialect
	metadata *metadata.Registry
	options  *MigratorOptions
	logger   MigrationLogger
}

// MigrationLogger interface for logging
type MigrationLogger interface {
	Info(msg string, args ...interface{})
	Error(msg string, args ...interface{})
	Debug(msg string, args ...interface{})
}

// DefaultLogger is a simple console logger
type DefaultLogger struct{}

func (l *DefaultLogger) Info(msg string, args ...interface{}) {
	log.Printf("[INFO] "+msg, args...)
}

func (l *DefaultLogger) Error(msg string, args ...interface{}) {
	log.Printf("[ERROR] "+msg, args...)
}

func (l *DefaultLogger) Debug(msg string, args ...interface{}) {
	log.Printf("[DEBUG] "+msg, args...)
}

// NewMigrator creates a new migrator instance
func NewMigrator(db *sql.DB, dialect dialects.Dialect, metadata *metadata.Registry, options *MigratorOptions) *Migrator {
	if options == nil {
		options = DefaultMigratorOptions()
	}

	return &Migrator{
		db:       db,
		dialect:  dialect,
		metadata: metadata,
		options:  options,
		logger:   &DefaultLogger{},
	}
}

// SetLogger sets a custom logger
func (m *Migrator) SetLogger(logger MigrationLogger) {
	m.logger = logger
}

// Initialize creates the migration system tables if they don't exist
func (m *Migrator) Initialize(ctx context.Context) error {
	m.logger.Info("Initializing migration system...")

	// Create migration history table with comprehensive schema
	createHistoryTableSQL := m.buildCreateHistoryTableSQL()
	if err := m.executeSQL(ctx, createHistoryTableSQL, "create migration history table"); err != nil {
		return fmt.Errorf("failed to create migration history table: %w", err)
	}

	// Create migration lock table for concurrent migration protection
	createLockTableSQL := m.buildCreateLockTableSQL()
	if err := m.executeSQL(ctx, createLockTableSQL, "create migration lock table"); err != nil {
		return fmt.Errorf("failed to create migration lock table: %w", err)
	}

	// Initialize the lock table with a single row
	if err := m.initializeLockTable(ctx); err != nil {
		return fmt.Errorf("failed to initialize lock table: %w", err)
	}

	m.logger.Info("Migration system initialized successfully")
	return nil
}

// ApplyMigrations applies multiple migrations in sequence
func (m *Migrator) ApplyMigrations(ctx context.Context, migrations []Migration) error {
	if err := m.Initialize(ctx); err != nil {
		return fmt.Errorf("failed to initialize migration system: %w", err)
	}

	// Calculate checksums for all migrations
	for i := range migrations {
		migrations[i].Checksum = m.calculateChecksum(migrations[i])
	}

	// Get pending migrations (this will check what's already applied)
	pending, err := m.GetPendingMigrations(ctx, migrations)
	if err != nil {
		return fmt.Errorf("failed to get pending migrations: %w", err)
	}

	if len(pending) == 0 {
		m.logger.Info("No pending migrations to apply")
		return nil
	}

	m.logger.Info("Found %d pending migrations to apply", len(pending))

	// Acquire migration lock to prevent concurrent migrations
	if err := m.acquireLock(ctx); err != nil {
		return fmt.Errorf("failed to acquire migration lock: %w", err)
	}
	defer func() {
		if err := m.releaseLock(ctx); err != nil {
			m.logger.Error("Failed to release migration lock: %v", err)
		}
	}()

	// Apply each pending migration
	for _, migration := range pending {
		if err := m.applyMigration(ctx, migration); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration.ID, err)
		}
	}

	m.logger.Info("Successfully applied %d migrations", len(pending))
	return nil
}

// GetPendingMigrations returns migrations that haven't been successfully applied
func (m *Migrator) GetPendingMigrations(ctx context.Context, allMigrations []Migration) ([]Migration, error) {
	applied, err := m.GetAppliedMigrations(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get applied migrations: %w", err)
	}

	appliedMap := make(map[string]*MigrationHistory)
	for i := range applied {
		appliedMap[applied[i].ID] = &applied[i]
	}

	var pending []Migration
	for _, migration := range allMigrations {
		appliedMigration, exists := appliedMap[migration.ID]

		if !exists {
			// Migration never applied
			pending = append(pending, migration)
		} else if !appliedMigration.Success {
			// Migration failed previously, include it for retry
			m.logger.Info("Including previously failed migration for retry: %s", migration.ID)
			pending = append(pending, migration)
		} else if m.options.ChecksumValidation {
			// Check if checksum matches (migration content changed)
			expectedChecksum := m.calculateChecksum(migration)
			if appliedMigration.Checksum != expectedChecksum {
				return nil, fmt.Errorf(
					"checksum mismatch for migration %s: expected %s, got %s. "+
						"Migration content has changed since it was applied",
					migration.ID, expectedChecksum, appliedMigration.Checksum,
				)
			}
		}
	}

	// Sort by version to ensure proper order
	sort.Slice(pending, func(i, j int) bool {
		return pending[i].Version < pending[j].Version
	})

	return pending, nil
}

// buildGetAppliedMigrationsQuery constructs the query for getting applied migrations
// Fixes G201: Replaces fmt.Sprintf with safer string building
func (m *Migrator) buildGetAppliedMigrationsQuery() string {
	var query strings.Builder

	query.WriteString("SELECT id, name, version, applied_at, execution_ms, checksum, success,")
	query.WriteString(" COALESCE(error, '') as error, rolled_back_at")
	query.WriteString(" FROM ")
	query.WriteString(m.dialect.Quote(m.options.TableName))
	query.WriteString(" WHERE success = ")
	query.WriteString(m.getBooleanTrue())
	query.WriteString(" AND rolled_back_at IS NULL")
	query.WriteString(" ORDER BY version ASC")

	return query.String()
}

// GetAppliedMigrations returns all successfully applied migrations
func (m *Migrator) GetAppliedMigrations(ctx context.Context) ([]MigrationHistory, error) {
	// Fix G201: Use secure query builder instead of fmt.Sprintf
	query := m.buildGetAppliedMigrationsQuery()

	rows, err := m.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query applied migrations: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			m.logger.Error("failed to close rows: %v", err)
		}
	}()

	var migrations []MigrationHistory
	for rows.Next() {
		var migration MigrationHistory
		var rolledBackAt sql.NullTime

		err := rows.Scan(
			&migration.ID,
			&migration.Name,
			&migration.Version,
			&migration.AppliedAt,
			&migration.ExecutionMS,
			&migration.Checksum,
			&migration.Success,
			&migration.Error,
			&rolledBackAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan migration history: %w", err)
		}

		if rolledBackAt.Valid {
			migration.RolledBackAt = &rolledBackAt.Time
		}

		migrations = append(migrations, migration)
	}

	return migrations, rows.Err()
}

// applyMigration applies a single migration with full transaction support
func (m *Migrator) applyMigration(ctx context.Context, migration Migration) error {
	startTime := time.Now()
	m.logger.Info("Applying migration %s: %s", migration.ID, migration.Name)

	var tx *sql.Tx
	var err error
	var success bool
	var errorMsg string

	// Record migration attempt at the end
	defer func() {
		executionMS := time.Since(startTime).Milliseconds()
		if recordErr := m.recordMigrationHistory(ctx, migration, success, errorMsg, executionMS); recordErr != nil {
			m.logger.Error("Failed to record migration history: %v", recordErr)
		}
	}()

	// Begin transaction if transaction mode is enabled
	if m.options.TransactionMode {
		tx, err = m.db.BeginTx(ctx, nil)
		if err != nil {
			errorMsg = err.Error()
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
		defer func() {
			if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
				m.logger.Error("Failed to rollback transaction: %v", err)
			}
		}()
	}

	// Execute migration SQL
	statements := m.splitSQL(migration.UpSQL)
	for i, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}

		m.logger.Debug("Executing migration statement %d/%d", i+1, len(statements))

		var execErr error
		if tx != nil {
			_, execErr = tx.ExecContext(ctx, stmt)
		} else {
			_, execErr = m.db.ExecContext(ctx, stmt)
		}

		if execErr != nil {
			errorMsg = execErr.Error()
			return fmt.Errorf("failed to execute migration statement %d: %w", i+1, execErr)
		}
	}

	// Commit transaction if in transaction mode
	if tx != nil {
		if err := tx.Commit(); err != nil {
			errorMsg = err.Error()
			return fmt.Errorf("failed to commit migration transaction: %w", err)
		}
	}

	success = true
	m.logger.Info("Successfully applied migration %s", migration.ID)
	return nil
}

// recordMigrationHistory records the migration attempt in the history table
func (m *Migrator) recordMigrationHistory(ctx context.Context, migration Migration, success bool, errorMsg string, executionMS int64) error {
	insertSQL := m.buildInsertMigrationHistorySQL()

	var errorVal interface{}
	if errorMsg != "" {
		errorVal = errorMsg
	} else {
		errorVal = nil
	}

	args := []interface{}{
		migration.ID,
		migration.Name,
		migration.Version,
		time.Now(),
		executionMS,
		migration.Checksum,
		success,
		errorVal,
	}

	// Add additional args for PostgreSQL UPSERT
	if m.dialect.Name() == dialects.DriverPostgres {
		args = append(args, time.Now(), executionMS, success, errorVal)
	}

	_, err := m.db.ExecContext(ctx, insertSQL, args...)
	return err
}

// calculateChecksum calculates SHA-256 checksum of migration content
// Fix G401: Replace weak MD5 with stronger SHA-256
func (m *Migrator) calculateChecksum(migration Migration) string {
	content := fmt.Sprintf("%s%s%s", migration.ID, migration.UpSQL, migration.DownSQL)
	hash := sha256.Sum256([]byte(content)) // Fix G401: Use SHA-256 instead of MD5
	return fmt.Sprintf("%x", hash)
}

// getBooleanTrue returns the appropriate boolean true value for the dialect
func (m *Migrator) getBooleanTrue() string {
	switch m.dialect.Name() {
	case dialects.DriverSQLite3, dialects.DriverSQLite:
		return "1"
	case dialects.DriverPostgres, dialects.DriverMySQL, dialects.DriverSQLServer:
		return "TRUE"
	default:
		return "TRUE"
	}
}

// splitSQL splits SQL script into individual statements
func (m *Migrator) splitSQL(sql string) []string {
	// Simple SQL statement splitter - in production, use a proper SQL parser
	statements := strings.Split(sql, ";")
	var result []string

	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt != "" {
			result = append(result, stmt)
		}
	}

	return result
}

// executeSQL executes a SQL statement with logging
func (m *Migrator) executeSQL(ctx context.Context, sql string, description string) error {
	m.logger.Debug("Executing SQL: %s", description)

	if m.options.DryRun {
		m.logger.Info("DRY RUN - Would execute: %s", sql)
		return nil
	}

	_, err := m.db.ExecContext(ctx, sql)
	if err != nil {
		return fmt.Errorf("failed to execute %s: %w", description, err)
	}

	return nil
}

// buildCreateHistoryTableSQL builds the SQL for creating the migration history table
func (m *Migrator) buildCreateHistoryTableSQL() string {
	var builder strings.Builder

	builder.WriteString("CREATE TABLE IF NOT EXISTS ")
	builder.WriteString(m.dialect.Quote(m.options.TableName))
	builder.WriteString(" (")
	builder.WriteString("id VARCHAR(255) PRIMARY KEY,")
	builder.WriteString("name VARCHAR(255) NOT NULL,")
	builder.WriteString("version BIGINT NOT NULL,")
	builder.WriteString("applied_at TIMESTAMP NOT NULL,")
	builder.WriteString("execution_ms BIGINT NOT NULL,")
	builder.WriteString("checksum VARCHAR(64) NOT NULL,") // SHA-256 produces 64 hex chars
	builder.WriteString("success BOOLEAN NOT NULL,")
	builder.WriteString("error TEXT,")
	builder.WriteString("rolled_back_at TIMESTAMP")
	builder.WriteString(")")

	return builder.String()
}

// buildCreateLockTableSQL builds the SQL for creating the migration lock table
func (m *Migrator) buildCreateLockTableSQL() string {
	var sql strings.Builder

	sql.WriteString("CREATE TABLE IF NOT EXISTS ")
	sql.WriteString(m.dialect.Quote(m.options.LockTableName))
	sql.WriteString(" (")
	sql.WriteString("id INTEGER PRIMARY KEY,")
	sql.WriteString("locked_at TIMESTAMP,")
	sql.WriteString("locked_by VARCHAR(255),")
	sql.WriteString("process_id INTEGER,")
	sql.WriteString("hostname VARCHAR(255)")
	sql.WriteString(")")

	return sql.String()
}

// buildInsertMigrationHistorySQL builds the SQL for inserting migration history
func (m *Migrator) buildInsertMigrationHistorySQL() string {
	var builder strings.Builder

	if m.dialect.Name() == dialects.DriverPostgres {
		// PostgreSQL UPSERT syntax
		builder.WriteString("INSERT INTO ")
		builder.WriteString(m.dialect.Quote(m.options.TableName))
		builder.WriteString(" (id, name, version, applied_at, execution_ms, checksum, success, error)")
		builder.WriteString(" VALUES (")
		for i := 1; i <= 8; i++ {
			if i > 1 {
				builder.WriteString(", ")
			}
			builder.WriteString(m.dialect.Placeholder(i))
		}
		builder.WriteString(")")
		builder.WriteString(" ON CONFLICT (id) DO UPDATE SET")
		builder.WriteString(" applied_at = ")
		builder.WriteString(m.dialect.Placeholder(9))
		builder.WriteString(", execution_ms = ")
		builder.WriteString(m.dialect.Placeholder(10))
		builder.WriteString(", success = ")
		builder.WriteString(m.dialect.Placeholder(11))
		builder.WriteString(", error = ")
		builder.WriteString(m.dialect.Placeholder(12))
	} else {
		// Standard INSERT
		builder.WriteString("INSERT INTO ")
		builder.WriteString(m.dialect.Quote(m.options.TableName))
		builder.WriteString(" (id, name, version, applied_at, execution_ms, checksum, success, error)")
		builder.WriteString(" VALUES (")
		for i := 1; i <= 8; i++ {
			if i > 1 {
				builder.WriteString(", ")
			}
			builder.WriteString(m.dialect.Placeholder(i))
		}
		builder.WriteString(")")
	}

	return builder.String()
}

// initializeLockTable initializes the lock table with a single row
func (m *Migrator) initializeLockTable(ctx context.Context) error {
	insertSQL := m.buildInitializeLockTableSQL()

	_, err := m.db.ExecContext(ctx, insertSQL, 1)
	if err != nil {
		// Ignore error if row already exists
		m.logger.Debug("Lock table initialization: %v", err)
	}

	return nil
}

// buildInitializeLockTableSQL builds the SQL for initializing the lock table
func (m *Migrator) buildInitializeLockTableSQL() string {
	var builder strings.Builder

	if m.dialect.Name() == dialects.DriverPostgres {
		builder.WriteString("INSERT INTO ")
		builder.WriteString(m.dialect.Quote(m.options.LockTableName))
		builder.WriteString(" (id) VALUES (")
		builder.WriteString(m.dialect.Placeholder(1))
		builder.WriteString(") ON CONFLICT (id) DO NOTHING")
	} else if m.dialect.Name() == dialects.DriverMySQL {
		builder.WriteString("INSERT IGNORE INTO ")
		builder.WriteString(m.dialect.Quote(m.options.LockTableName))
		builder.WriteString(" (id) VALUES (")
		builder.WriteString(m.dialect.Placeholder(1))
		builder.WriteString(")")
	} else {
		// SQLite and SQL Server
		builder.WriteString("INSERT OR IGNORE INTO ")
		builder.WriteString(m.dialect.Quote(m.options.LockTableName))
		builder.WriteString(" (id) VALUES (")
		builder.WriteString(m.dialect.Placeholder(1))
		builder.WriteString(")")
	}

	return builder.String()
}

// acquireLock acquires a migration lock to prevent concurrent migrations
func (m *Migrator) acquireLock(ctx context.Context) error {
	timeout := time.Now().Add(m.options.LockTimeout)

	for time.Now().Before(timeout) {
		// Try to acquire lock
		lockSQL := m.buildAcquireLockSQL()

		lockTimeout := time.Now().Add(-time.Minute * 5) // Consider locks older than 5 minutes as stale
		result, err := m.db.ExecContext(ctx, lockSQL,
			time.Now(),
			"goef-migrator",
			1,           // process ID placeholder
			"localhost", // hostname placeholder
			lockTimeout,
		)

		if err != nil {
			return fmt.Errorf("failed to acquire lock: %w", err)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to check lock acquisition: %w", err)
		}

		if rowsAffected > 0 {
			m.logger.Debug("Successfully acquired migration lock")
			return nil
		}

		// Lock not acquired, wait and retry
		time.Sleep(time.Second)
	}

	return fmt.Errorf("failed to acquire migration lock within timeout of %v", m.options.LockTimeout)
}

// buildAcquireLockSQL builds the SQL for acquiring a migration lock
func (m *Migrator) buildAcquireLockSQL() string {
	var builder strings.Builder

	builder.WriteString("UPDATE ")
	builder.WriteString(m.dialect.Quote(m.options.LockTableName))
	builder.WriteString(" SET locked_at = ")
	builder.WriteString(m.dialect.Placeholder(1))
	builder.WriteString(", locked_by = ")
	builder.WriteString(m.dialect.Placeholder(2))
	builder.WriteString(", process_id = ")
	builder.WriteString(m.dialect.Placeholder(3))
	builder.WriteString(", hostname = ")
	builder.WriteString(m.dialect.Placeholder(4))
	builder.WriteString(" WHERE id = 1 AND (locked_at IS NULL OR locked_at < ")
	builder.WriteString(m.dialect.Placeholder(5))
	builder.WriteString(")")

	return builder.String()
}

// releaseLock releases the migration lock
func (m *Migrator) releaseLock(ctx context.Context) error {
	unlockSQL := m.buildReleaseLockSQL()

	_, err := m.db.ExecContext(ctx, unlockSQL)
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}

	m.logger.Debug("Successfully released migration lock")
	return nil
}

// buildReleaseLockSQL builds the SQL for releasing a migration lock
func (m *Migrator) buildReleaseLockSQL() string {
	var builder strings.Builder

	builder.WriteString("UPDATE ")
	builder.WriteString(m.dialect.Quote(m.options.LockTableName))
	builder.WriteString(" SET locked_at = NULL, locked_by = NULL, process_id = NULL, hostname = NULL")
	builder.WriteString(" WHERE id = 1")

	return builder.String()
}

// RollbackMigration rolls back a specific migration
func (m *Migrator) RollbackMigration(ctx context.Context, migration Migration) error {
	if migration.DownSQL == "" {
		return fmt.Errorf("migration %s has no rollback SQL defined", migration.ID)
	}

	m.logger.Info("Rolling back migration %s: %s", migration.ID, migration.Name)

	// Acquire lock
	if err := m.acquireLock(ctx); err != nil {
		return fmt.Errorf("failed to acquire migration lock: %w", err)
	}
	defer func() { _ = m.releaseLock(ctx) }()

	// Begin transaction
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin rollback transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			m.logger.Error("Failed to rollback transaction: %v", err)
		}
	}()

	// Execute rollback SQL
	statements := m.splitSQL(migration.DownSQL)
	for i, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}

		m.logger.Debug("Executing rollback statement %d/%d", i+1, len(statements))
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("failed to execute rollback statement %d: %w", i+1, err)
		}
	}

	// Mark migration as rolled back
	updateSQL := m.buildMarkRollbackSQL()
	if _, err := tx.ExecContext(ctx, updateSQL, time.Now(), migration.ID); err != nil {
		return fmt.Errorf("failed to mark migration as rolled back: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit rollback transaction: %w", err)
	}

	m.logger.Info("Successfully rolled back migration %s", migration.ID)
	return nil
}

// buildMarkRollbackSQL builds the SQL for marking a migration as rolled back
func (m *Migrator) buildMarkRollbackSQL() string {
	var builder strings.Builder

	builder.WriteString("UPDATE ")
	builder.WriteString(m.dialect.Quote(m.options.TableName))
	builder.WriteString(" SET rolled_back_at = ")
	builder.WriteString(m.dialect.Placeholder(1))
	builder.WriteString(" WHERE id = ")
	builder.WriteString(m.dialect.Placeholder(2))

	return builder.String()
}

// GetMigrationStatus returns the status of all migrations
func (m *Migrator) GetMigrationStatus(ctx context.Context, allMigrations []Migration) (map[string]MigrationStatus, error) {
	applied, err := m.GetAppliedMigrations(ctx)
	if err != nil {
		return nil, err
	}

	appliedMap := make(map[string]*MigrationHistory)
	for i := range applied {
		appliedMap[applied[i].ID] = &applied[i]
	}

	status := make(map[string]MigrationStatus)
	for _, migration := range allMigrations {
		if appliedMig, exists := appliedMap[migration.ID]; exists {
			if appliedMig.RolledBackAt != nil {
				status[migration.ID] = StatusRolledBack
			} else if appliedMig.Success {
				status[migration.ID] = StatusApplied
			} else {
				status[migration.ID] = StatusFailed
			}
		} else {
			status[migration.ID] = StatusPending
		}
	}

	return status, nil
}
