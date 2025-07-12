package migration

import (
	"context"
	"database/sql"
	"log"
	"strings"

	"github.com/goef-team/goef/dialects"
)

// HistoryRepository provides basic access to the migration history table.
type HistoryRepository struct {
	db      *sql.DB
	dialect dialects.Dialect
	table   string
}

// NewHistoryRepository creates a new repository instance.
func NewHistoryRepository(db *sql.DB, dialect dialects.Dialect, table string) *HistoryRepository {
	if table == "" {
		table = "__goef_migrations"
	}
	return &HistoryRepository{db: db, dialect: dialect, table: table}
}

// buildSelectAllQuery constructs the SQL query for getting all migration history records
// This method avoids fmt.Sprintf to satisfy gosec requirements while maintaining security
func (r *HistoryRepository) buildSelectAllQuery() string {
	var query strings.Builder

	// Build SELECT clause
	query.WriteString("SELECT id, name, version, applied_at, execution_ms, checksum, success, error, rolled_back_at")
	query.WriteString(" FROM ")

	// Use dialect.Quote to safely handle table name (prevents injection)
	query.WriteString(r.dialect.Quote(r.table))

	// Add ORDER BY clause
	query.WriteString(" ORDER BY version")

	return query.String()
}

// buildSelectByIDQuery constructs the SQL query for getting a specific migration by ID
// This method avoids fmt.Sprintf to satisfy gosec requirements while maintaining security
func (r *HistoryRepository) buildSelectByIDQuery() string {
	var query strings.Builder

	// Build SELECT clause
	query.WriteString("SELECT id, name, version, applied_at, execution_ms, checksum, success, error, rolled_back_at")
	query.WriteString(" FROM ")

	// Use dialect.Quote to safely handle table name (prevents injection)
	query.WriteString(r.dialect.Quote(r.table))

	// Add WHERE clause with parameterized placeholder
	query.WriteString(" WHERE id = ")
	query.WriteString(r.dialect.Placeholder(1))

	return query.String()
}

// GetAll returns all migration history records ordered by version.
func (r *HistoryRepository) GetAll(ctx context.Context) ([]MigrationHistory, error) {
	// Use secure query builder instead of fmt.Sprintf (fixes G201)
	query := r.buildSelectAllQuery()

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var results []MigrationHistory
	for rows.Next() {
		var mh MigrationHistory
		var rolled sql.NullTime

		err := rows.Scan(
			&mh.ID,
			&mh.Name,
			&mh.Version,
			&mh.AppliedAt,
			&mh.ExecutionMS,
			&mh.Checksum,
			&mh.Success,
			&mh.Error,
			&rolled,
		)
		if err != nil {
			return nil, err
		}

		if rolled.Valid {
			mh.RolledBackAt = &rolled.Time
		}
		results = append(results, mh)
	}

	return results, rows.Err()
}

// GetByID returns a single migration history record by ID.
func (r *HistoryRepository) GetByID(ctx context.Context, id string) (*MigrationHistory, error) {
	// Use secure query builder instead of fmt.Sprintf (fixes G201)
	query := r.buildSelectByIDQuery()

	// Use parameterized query to prevent SQL injection
	row := r.db.QueryRowContext(ctx, query, id)

	var mh MigrationHistory
	var rolled sql.NullTime

	err := row.Scan(
		&mh.ID,
		&mh.Name,
		&mh.Version,
		&mh.AppliedAt,
		&mh.ExecutionMS,
		&mh.Checksum,
		&mh.Success,
		&mh.Error,
		&rolled,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if rolled.Valid {
		mh.RolledBackAt = &rolled.Time
	}

	return &mh, nil
}

// GetByStatus returns migration history records filtered by success status
func (r *HistoryRepository) GetByStatus(ctx context.Context, success bool) ([]MigrationHistory, error) {
	query := r.buildSelectByStatusQuery()

	rows, err := r.db.QueryContext(ctx, query, success)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var results []MigrationHistory
	for rows.Next() {
		var mh MigrationHistory
		var rolled sql.NullTime

		err := rows.Scan(
			&mh.ID,
			&mh.Name,
			&mh.Version,
			&mh.AppliedAt,
			&mh.ExecutionMS,
			&mh.Checksum,
			&mh.Success,
			&mh.Error,
			&rolled,
		)
		if err != nil {
			return nil, err
		}

		if rolled.Valid {
			mh.RolledBackAt = &rolled.Time
		}
		results = append(results, mh)
	}

	return results, rows.Err()
}

// buildSelectByStatusQuery constructs the SQL query for getting migrations by status
func (r *HistoryRepository) buildSelectByStatusQuery() string {
	var query strings.Builder

	// Build SELECT clause
	query.WriteString("SELECT id, name, version, applied_at, execution_ms, checksum, success, error, rolled_back_at")
	query.WriteString(" FROM ")

	// Use dialect.Quote to safely handle table name
	query.WriteString(r.dialect.Quote(r.table))

	// Add WHERE clause with parameterized placeholder
	query.WriteString(" WHERE success = ")
	query.WriteString(r.dialect.Placeholder(1))
	query.WriteString(" ORDER BY version")

	return query.String()
}

// Insert adds a new migration history record
func (r *HistoryRepository) Insert(ctx context.Context, history *MigrationHistory) error {
	query := r.buildInsertQuery()

	_, err := r.db.ExecContext(ctx, query,
		history.ID,
		history.Name,
		history.Version,
		history.AppliedAt,
		history.ExecutionMS,
		history.Checksum,
		history.Success,
		history.Error,
	)

	return err
}

// buildInsertQuery constructs the SQL query for inserting migration history
func (r *HistoryRepository) buildInsertQuery() string {
	var query strings.Builder

	query.WriteString("INSERT INTO ")
	query.WriteString(r.dialect.Quote(r.table))
	query.WriteString(" (id, name, version, applied_at, execution_ms, checksum, success, error)")
	query.WriteString(" VALUES (")

	// Use dialect placeholders for parameterized query
	placeholders := make([]string, 8)
	for i := range placeholders {
		placeholders[i] = r.dialect.Placeholder(i + 1)
	}
	query.WriteString(strings.Join(placeholders, ", "))
	query.WriteString(")")

	return query.String()
}

// Update modifies an existing migration history record
func (r *HistoryRepository) Update(ctx context.Context, history *MigrationHistory) error {
	query := r.buildUpdateQuery()

	_, err := r.db.ExecContext(ctx, query,
		history.Name,
		history.ExecutionMS,
		history.Checksum,
		history.Success,
		history.Error,
		history.RolledBackAt,
		history.ID, // WHERE clause parameter
	)

	return err
}

// buildUpdateQuery constructs the SQL query for updating migration history
func (r *HistoryRepository) buildUpdateQuery() string {
	var query strings.Builder

	query.WriteString("UPDATE ")
	query.WriteString(r.dialect.Quote(r.table))
	query.WriteString(" SET ")
	query.WriteString("name = ")
	query.WriteString(r.dialect.Placeholder(1))
	query.WriteString(", execution_ms = ")
	query.WriteString(r.dialect.Placeholder(2))
	query.WriteString(", checksum = ")
	query.WriteString(r.dialect.Placeholder(3))
	query.WriteString(", success = ")
	query.WriteString(r.dialect.Placeholder(4))
	query.WriteString(", error = ")
	query.WriteString(r.dialect.Placeholder(5))
	query.WriteString(", rolled_back_at = ")
	query.WriteString(r.dialect.Placeholder(6))
	query.WriteString(" WHERE id = ")
	query.WriteString(r.dialect.Placeholder(7))

	return query.String()
}

// MarkAsRolledBack marks a migration as rolled back
func (r *HistoryRepository) MarkAsRolledBack(ctx context.Context, migrationID string, rolledBackAt sql.NullTime) error {
	query := r.buildMarkRolledBackQuery()

	_, err := r.db.ExecContext(ctx, query, rolledBackAt, migrationID)
	return err
}

// buildMarkRolledBackQuery constructs the SQL query for marking a migration as rolled back
func (r *HistoryRepository) buildMarkRolledBackQuery() string {
	var query strings.Builder

	query.WriteString("UPDATE ")
	query.WriteString(r.dialect.Quote(r.table))
	query.WriteString(" SET rolled_back_at = ")
	query.WriteString(r.dialect.Placeholder(1))
	query.WriteString(" WHERE id = ")
	query.WriteString(r.dialect.Placeholder(2))

	return query.String()
}

// Delete removes a migration history record (use with caution)
func (r *HistoryRepository) Delete(ctx context.Context, migrationID string) error {
	query := r.buildDeleteQuery()

	_, err := r.db.ExecContext(ctx, query, migrationID)
	return err
}

// buildDeleteQuery constructs the SQL query for deleting migration history
func (r *HistoryRepository) buildDeleteQuery() string {
	var query strings.Builder

	query.WriteString("DELETE FROM ")
	query.WriteString(r.dialect.Quote(r.table))
	query.WriteString(" WHERE id = ")
	query.WriteString(r.dialect.Placeholder(1))

	return query.String()
}

// GetAppliedCount returns the count of successfully applied migrations
func (r *HistoryRepository) GetAppliedCount(ctx context.Context) (int64, error) {
	query := r.buildCountAppliedQuery()

	var count int64
	err := r.db.QueryRowContext(ctx, query, true).Scan(&count)
	return count, err
}

// buildCountAppliedQuery constructs the SQL query for counting applied migrations
func (r *HistoryRepository) buildCountAppliedQuery() string {
	var query strings.Builder

	query.WriteString("SELECT COUNT(*) FROM ")
	query.WriteString(r.dialect.Quote(r.table))
	query.WriteString(" WHERE success = ")
	query.WriteString(r.dialect.Placeholder(1))
	query.WriteString(" AND rolled_back_at IS NULL")

	return query.String()
}
