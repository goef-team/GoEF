// Package query provides a type-safe query builder for GoEF.
package query

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/goef-team/goef/dialects"
	"github.com/goef-team/goef/metadata"
)

// Builder provides a query builder for entities
type Builder struct {
	dialect  dialects.Dialect
	metadata *metadata.Registry
	db       *sql.DB
}

// NewBuilder creates a new query builder
func NewBuilder(dialect dialects.Dialect, metadata *metadata.Registry, db *sql.DB) *Builder {
	return &Builder{
		dialect:  dialect,
		metadata: metadata,
		db:       db,
	}
}

// Where adds a WHERE condition
func (b *Builder) Where(column string, operator string, value interface{}) *QueryBuilder {
	query := &dialects.SelectQuery{
		Where: []dialects.WhereClause{
			{
				Column:   column,
				Operator: operator,
				Value:    value,
				Logic:    "AND",
			},
		},
	}

	return &QueryBuilder{
		Builder: b,
		query:   query,
	}
}

// QueryBuilder provides fluent interface for building queries
type QueryBuilder struct {
	*Builder
	query *dialects.SelectQuery
}

// Where adds another WHERE condition
func (qb *QueryBuilder) Where(column string, operator string, value interface{}) *QueryBuilder {
	qb.query.Where = append(qb.query.Where, dialects.WhereClause{
		Column:   column,
		Operator: operator,
		Value:    value,
		Logic:    "AND",
	})
	return qb
}

// Or adds an OR condition
func (qb *QueryBuilder) Or(column string, operator string, value interface{}) *QueryBuilder {
	qb.query.Where = append(qb.query.Where, dialects.WhereClause{
		Column:   column,
		Operator: operator,
		Value:    value,
		Logic:    "OR",
	})
	return qb
}

// OrderBy adds an ORDER BY clause
func (qb *QueryBuilder) OrderBy(column string) *QueryBuilder {
	qb.query.OrderBy = append(qb.query.OrderBy, dialects.OrderByClause{
		Column:    column,
		Direction: "ASC",
	})
	return qb
}

// OrderByDescending adds a descending ORDER BY clause
func (qb *QueryBuilder) OrderByDescending(column string) *QueryBuilder {
	qb.query.OrderBy = append(qb.query.OrderBy, dialects.OrderByClause{
		Column:    column,
		Direction: "DESC",
	})
	return qb
}

// Take limits the number of results
func (qb *QueryBuilder) Take(count int) *QueryBuilder {
	qb.query.Limit = &count
	return qb
}

// Skip skips the specified number of results
func (qb *QueryBuilder) Skip(count int) *QueryBuilder {
	qb.query.Offset = &count
	return qb
}

// Distinct ensures unique results
func (qb *QueryBuilder) Distinct() *QueryBuilder {
	qb.query.Distinct = true
	return qb
}

// FirstOrDefault returns the first result or nil
func (qb *QueryBuilder) FirstOrDefault() (interface{}, error) {
	return qb.FirstOrDefaultWithContext(context.Background())
}

// FirstOrDefaultWithContext returns the first result with context
func (qb *QueryBuilder) FirstOrDefaultWithContext(ctx context.Context) (interface{}, error) {
	originalLimit := qb.query.Limit
	qb.query.Limit = intPtr(1)
	defer func() {
		qb.query.Limit = originalLimit
	}()

	results, err := qb.ToListWithContext(ctx)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, nil
	}

	return results[0], nil
}

// ToListWithContext executes the query and returns results
func (qb *QueryBuilder) ToListWithContext(ctx context.Context) ([]interface{}, error) {
	if qb.query.Table == "" {
		return nil, fmt.Errorf("table name is required")
	}

	sqlStr, args, err := qb.dialect.BuildSelect(qb.query.Table, qb.query)
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	rows, err := qb.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer func() {
		if err = rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	var results []interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		scanArgs := make([]interface{}, len(columns))
		for i := range values {
			scanArgs[i] = &values[i]
		}

		if err = rows.Scan(scanArgs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		rowMap := make(map[string]interface{})
		for i, col := range columns {
			rowMap[col] = values[i]
		}
		results = append(results, rowMap)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("row error: %w", err)
	}

	return results, nil
}

// Count returns the number of entities matching the query
func (qb *QueryBuilder) Count() (int64, error) {
	return qb.CountWithContext(context.Background())
}

// CountWithContext returns the count with context
func (qb *QueryBuilder) CountWithContext(ctx context.Context) (int64, error) {
	// Create a count query
	countQuery := &dialects.SelectQuery{
		Table:   qb.query.Table,
		Columns: []string{"COUNT(*)"},
		Where:   qb.query.Where,
		Joins:   qb.query.Joins,
	}

	buildSelect, args, err := qb.dialect.BuildSelect(qb.query.Table, countQuery)
	if err != nil {
		return 0, fmt.Errorf("failed to build count query: %w", err)
	}

	var count int64
	err = qb.db.QueryRowContext(ctx, buildSelect, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to execute count query: %w", err)
	}

	return count, nil
}

// Any returns true if any entities match the query
func (qb *QueryBuilder) Any() (bool, error) {
	return qb.AnyWithContext(context.Background())
}

// AnyWithContext returns true if any entities match the query with context
func (qb *QueryBuilder) AnyWithContext(ctx context.Context) (bool, error) {
	count, err := qb.CountWithContext(ctx)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// intPtr returns a pointer to an int
func intPtr(i int) *int {
	return &i
}
