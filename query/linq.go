// query/linq.go with proper Builder integration
package query

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"

	"github.com/goef-team/goef/dialects"
	"github.com/goef-team/goef/metadata"
)

// Queryable provides LINQ-style querying capabilities
type Queryable[T any] struct {
	provider *QueryProvider
	query    *dialects.SelectQuery
}

// QueryProvider handles query execution
type QueryProvider struct {
	dialect  dialects.Dialect
	metadata *metadata.Registry
	db       *sql.DB
}

// NewQueryable creates a new queryable instance
func NewQueryable[T any](dialect dialects.Dialect, metadata *metadata.Registry, db *sql.DB) *Queryable[T] {
	var zero T
	meta := metadata.GetOrCreate(&zero)

	provider := &QueryProvider{
		dialect:  dialect,
		metadata: metadata,
		db:       db,
	}

	baseQuery := &dialects.SelectQuery{
		Table:   meta.TableName,
		Columns: []string{},
	}

	// Add all non-relationship columns by default
	for _, field := range meta.Fields {
		if !field.IsRelationship {
			baseQuery.Columns = append(baseQuery.Columns, field.ColumnName)
		}
	}

	return &Queryable[T]{
		provider: provider,
		query:    baseQuery,
	}
}

// Where adds a WHERE clause with column name and value
func (q *Queryable[T]) Where(column string, operator string, value interface{}) *Queryable[T] {
	newQuery := q.copyQuery()
	newQuery.Where = append(newQuery.Where, dialects.WhereClause{
		Column:   column,
		Operator: operator,
		Value:    value,
		Logic:    "AND",
	})

	return &Queryable[T]{
		provider: q.provider,
		query:    newQuery,
	}
}

// WhereExpr adds a WHERE clause using an expression -  VERSION
func (q *Queryable[T]) WhereExpr(expr Expression) *Queryable[T] {
	newQuery := q.copyQuery()

	// Try to extract a simple binary expression
	if binExpr, ok := expr.(*BinaryExpression); ok {
		if memberExpr, ok := binExpr.Left.(*MemberExpression); ok {
			if constExpr, ok := binExpr.Right.(*ConstantExpression); ok {
				// Convert member name to column name using metadata
				var zero T
				meta := q.provider.metadata.GetOrCreate(&zero)

				columnName := memberExpr.Member
				// Try to find the field by name and get its column name
				for _, field := range meta.Fields {
					if !field.IsRelationship && strings.EqualFold(field.Name, memberExpr.Member) {
						columnName = field.ColumnName
						break
					}
				}

				operator := binExpr.Operator.String()
				newQuery.Where = append(newQuery.Where, dialects.WhereClause{
					Column:   columnName,
					Operator: operator,
					Value:    constExpr.Value,
					Logic:    "AND",
				})
			}
		}
	}

	return &Queryable[T]{
		provider: q.provider,
		query:    newQuery,
	}
}

// OrderBy adds an ORDER BY clause
func (q *Queryable[T]) OrderBy(column string) *Queryable[T] {
	newQuery := q.copyQuery()
	newQuery.OrderBy = append(newQuery.OrderBy, dialects.OrderByClause{
		Column:    column,
		Direction: "ASC",
	})

	return &Queryable[T]{
		provider: q.provider,
		query:    newQuery,
	}
}

// OrderByDesc adds an ORDER BY DESC clause
func (q *Queryable[T]) OrderByDesc(column string) *Queryable[T] {
	newQuery := q.copyQuery()
	newQuery.OrderBy = append(newQuery.OrderBy, dialects.OrderByClause{
		Column:    column,
		Direction: "DESC",
	})

	return &Queryable[T]{
		provider: q.provider,
		query:    newQuery,
	}
}

// Take limits the number of results
func (q *Queryable[T]) Take(count int) *Queryable[T] {
	newQuery := q.copyQuery()
	newQuery.Limit = &count

	return &Queryable[T]{
		provider: q.provider,
		query:    newQuery,
	}
}

// Skip skips the specified number of results
func (q *Queryable[T]) Skip(count int) *Queryable[T] {
	newQuery := q.copyQuery()
	newQuery.Offset = &count

	return &Queryable[T]{
		provider: q.provider,
		query:    newQuery,
	}
}

func (q *Queryable[T]) ToList() ([]*T, error) {
	sql, args, err := q.provider.dialect.BuildSelect(q.query.Table, q.query)
	if err != nil {
		return nil, err
	}

	rows, err := q.provider.db.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var results []*T
	meta := q.provider.metadata.GetOrCreate(new(T))

	for rows.Next() {
		entity, err := q.scanSingleEntityFromRows(rows, meta)
		if err != nil {
			return nil, err
		}
		results = append(results, entity.(*T))
	}

	return results, rows.Err()
}

// : scanSingleEntityFromRows method
func (q *Queryable[T]) scanSingleEntityFromRows(rows *sql.Rows, meta *metadata.EntityMetadata) (interface{}, error) {
	entity := reflect.New(meta.Type).Interface()
	entityValue := reflect.ValueOf(entity).Elem()

	// : Only create scan destinations for non-relationship fields
	var scanDests []interface{}
	var scanFields []metadata.FieldMetadata

	for _, field := range meta.Fields {
		if !field.IsRelationship { // : Skip relationship fields
			scanFields = append(scanFields, field)
		}
	}

	scanDests = make([]interface{}, len(scanFields))
	for i, field := range scanFields {
		fieldValue := entityValue.FieldByName(field.Name)
		if !fieldValue.IsValid() {
			return nil, fmt.Errorf("field %s not found in entity", field.Name)
		}
		scanDests[i] = fieldValue.Addr().Interface()
	}

	if err := rows.Scan(scanDests...); err != nil {
		return nil, err
	}

	return entity, nil
}

// ToListWithContext executes the query with context and returns all results
func (q *Queryable[T]) ToListWithContext(ctx context.Context) ([]T, error) {
	var zero T
	meta := q.provider.metadata.GetOrCreate(&zero)

	sql, args, err := q.provider.dialect.BuildSelect(meta.TableName, q.query)
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	rows, err := q.provider.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []T
	for rows.Next() {
		entity, err := q.scanEntity(rows, meta)
		if err != nil {
			return nil, fmt.Errorf("failed to scan entity: %w", err)
		}
		results = append(results, entity)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading rows: %w", err)
	}

	return results, nil
}

// First returns the first result or an error
func (q *Queryable[T]) First() (T, error) {
	return q.FirstWithContext(context.Background())
}

// FirstWithContext returns the first result with context
func (q *Queryable[T]) FirstWithContext(ctx context.Context) (T, error) {
	limitedQuery := q.Take(1)
	results, err := limitedQuery.ToListWithContext(ctx)
	if err != nil {
		var zero T
		return zero, err
	}

	if len(results) == 0 {
		var zero T
		return zero, fmt.Errorf("no results found")
	}

	return results[0], nil
}

// FirstOrDefault returns the first result or the zero value
func (q *Queryable[T]) FirstOrDefault() (T, error) {
	return q.FirstOrDefaultWithContext(context.Background())
}

// FirstOrDefaultWithContext returns the first result or zero value with context
func (q *Queryable[T]) FirstOrDefaultWithContext(ctx context.Context) (T, error) {
	results, err := q.Take(1).ToListWithContext(ctx)
	if err != nil {
		var zero T
		return zero, err
	}

	if len(results) == 0 {
		var zero T
		return zero, nil
	}

	return results[0], nil
}

// Count returns the number of entities matching the query
func (q *Queryable[T]) Count() (int64, error) {
	return q.CountWithContext(context.Background())
}

// CountWithContext returns the count with context -  VERSION
func (q *Queryable[T]) CountWithContext(ctx context.Context) (int64, error) {
	var zero T
	meta := q.provider.metadata.GetOrCreate(&zero)

	// Create a count query - : Use unquoted COUNT(*)
	countQuery := &dialects.SelectQuery{
		Table:   q.query.Table,
		Columns: []string{"COUNT(*)"}, // This will not be quoted by the  BuildSelect
		Where:   q.query.Where,
		Joins:   q.query.Joins,
	}

	sql, args, err := q.provider.dialect.BuildSelect(meta.TableName, countQuery)
	if err != nil {
		return 0, fmt.Errorf("failed to build count query: %w", err)
	}

	var count int64
	err = q.provider.db.QueryRowContext(ctx, sql, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to execute count query: %w", err)
	}

	return count, nil
}

// Any returns true if any entities match the query
func (q *Queryable[T]) Any() (bool, error) {
	return q.AnyWithContext(context.Background())
}

// AnyWithContext returns true if any entities match the query with context
func (q *Queryable[T]) AnyWithContext(ctx context.Context) (bool, error) {
	count, err := q.CountWithContext(ctx)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// copyQuery creates a copy of the current query
func (q *Queryable[T]) copyQuery() *dialects.SelectQuery {
	newQuery := &dialects.SelectQuery{
		Table:     q.query.Table,
		Columns:   make([]string, len(q.query.Columns)),
		Where:     make([]dialects.WhereClause, len(q.query.Where)),
		OrderBy:   make([]dialects.OrderByClause, len(q.query.OrderBy)),
		GroupBy:   make([]string, len(q.query.GroupBy)),
		Having:    make([]dialects.WhereClause, len(q.query.Having)),
		Joins:     make([]dialects.JoinClause, len(q.query.Joins)),
		Distinct:  q.query.Distinct,
		ForUpdate: q.query.ForUpdate,
	}

	copy(newQuery.Columns, q.query.Columns)
	copy(newQuery.Where, q.query.Where)
	copy(newQuery.OrderBy, q.query.OrderBy)
	copy(newQuery.GroupBy, q.query.GroupBy)
	copy(newQuery.Having, q.query.Having)
	copy(newQuery.Joins, q.query.Joins)

	if q.query.Limit != nil {
		limit := *q.query.Limit
		newQuery.Limit = &limit
	}

	if q.query.Offset != nil {
		offset := *q.query.Offset
		newQuery.Offset = &offset
	}

	return newQuery
}

// scanEntity scans a database row into an entity -  VERSION
func (q *Queryable[T]) scanEntity(rows *sql.Rows, meta *metadata.EntityMetadata) (T, error) {
	var zero T
	entity := reflect.New(reflect.TypeOf(zero)).Elem()

	// Create scan destinations only for non-relationship fields
	var scanDests []interface{}
	for _, field := range meta.Fields {
		if !field.IsRelationship {
			entityField := entity.FieldByName(field.Name)
			if !entityField.IsValid() {
				return zero, fmt.Errorf("field not found: %s", field.Name)
			}
			scanDests = append(scanDests, entityField.Addr().Interface())
		}
	}

	if err := rows.Scan(scanDests...); err != nil {
		return zero, fmt.Errorf("failed to scan row: %w", err)
	}

	return entity.Interface().(T), nil
}
