// Package dialects provides database-specific SQL generation and operations
// for different database backends supported by GoEF.
package dialects

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
)

// Dialect defines the interface for database-specific operations and SQL generation
type Dialect interface {
	// Name returns the name of the dialect
	Name() string

	// Quote quotes an identifier (table name, column name, etc.)
	Quote(identifier string) string

	// Placeholder returns the placeholder for the nth parameter (1-indexed)
	Placeholder(n int) string

	// BuildSelect builds a SELECT statement
	BuildSelect(tableName string, query *SelectQuery) (string, []interface{}, error)

	// BuildInsert builds an INSERT statement
	BuildInsert(tableName string, fields []string, values []interface{}) (string, []interface{}, error)

	// BuildBatchInsert builds a batch INSERT statement for multiple rows.
	BuildBatchInsert(tableName string, fields []string, allValues [][]interface{}) (string, []interface{}, error)

	// BuildUpdate builds an UPDATE statement
	BuildUpdate(tableName string, fields []string, values []interface{}, whereClause string, whereArgs []interface{}) (string, []interface{}, error)

	// BuildDelete builds a DELETE statement
	BuildDelete(tableName string, whereClause string, whereArgs []interface{}) (string, []interface{}, error)

	// BuildBatchDelete builds a batch DELETE statement using a list of primary keys.
	BuildBatchDelete(tableName string, pkColumn string, pkValues []interface{}) (string, []interface{}, error)

	// BuildCreateTable builds a CREATE TABLE statement
	BuildCreateTable(tableName string, columns []ColumnDefinition) (string, error)

	// BuildDropTable builds a DROP TABLE statement
	BuildDropTable(tableName string) (string, error)

	// BuildAddColumn builds an ADD COLUMN statement
	BuildAddColumn(tableName string, column ColumnDefinition) (string, error)

	// BuildDropColumn builds a DROP COLUMN statement
	BuildDropColumn(tableName string, columnName string) (string, error)

	// MapGoTypeToSQL maps a Go type to a SQL type
	MapGoTypeToSQL(goType reflect.Type, maxLength int, isAutoIncrement bool) string

	// SupportsReturning indicates if the database supports RETURNING clause
	SupportsReturning() bool

	// SupportsTransactions indicates if the database supports transactions
	SupportsTransactions() bool

	// SupportsCTE indicates if the database supports Common Table Expressions
	SupportsCTE() bool

	// GetLastInsertID gets the last inserted ID for auto-increment fields
	GetLastInsertID(ctx context.Context, tx *sql.Tx, tableName string) (int64, error)
}

// SelectQuery represents a SELECT query structure
type SelectQuery struct {
	// With allows specifying a CTE/with clause prefix for the query.
	With      string
	Table     string
	Columns   []string
	Where     []WhereClause
	OrderBy   []OrderByClause
	GroupBy   []string
	Having    []WhereClause
	Joins     []JoinClause
	Limit     *int
	Offset    *int
	Distinct  bool
	ForUpdate bool
}

// WhereClause represents a WHERE condition
type WhereClause struct {
	Column   string
	Operator string
	Value    interface{}
	Logic    string // AND, OR
}

// OrderByClause represents an ORDER BY clause
type OrderByClause struct {
	Column    string
	Direction string // ASC, DESC
}

// JoinClause represents a JOIN clause
type JoinClause struct {
	Type      string // INNER, LEFT, RIGHT, FULL
	Table     string
	Condition string
}

// ColumnDefinition represents a column definition for DDL
type ColumnDefinition struct {
	Name            string
	Type            string
	IsPrimaryKey    bool
	IsAutoIncrement bool
	IsRequired      bool
	IsUnique        bool
	DefaultValue    interface{}
	MaxLength       int
}

// New creates a new dialect instance based on the driver name
func New(driverName string, options map[string]interface{}) (Dialect, error) {
	switch driverName {
	case "postgres":
		return NewPostgreSQLDialect(options), nil
	case "mysql":
		return NewMySQLDialect(options), nil
	case "sqlite3":
		return NewSQLiteDialect(options), nil
	case "sqlserver":
		return NewSQLServerDialect(options), nil
	case "mongodb":
		return NewMongoDBDialect(options)
	default:
		return nil, fmt.Errorf("unsupported dialect: %s", driverName)
	}
}

// BaseDialect provides common functionality for SQL dialects
type BaseDialect struct {
	name    string
	options map[string]interface{}
}

// NewBaseDialect creates a new base dialect
func NewBaseDialect(name string, options map[string]interface{}) *BaseDialect {
	return &BaseDialect{
		name:    name,
		options: options,
	}
}

// Name returns the dialect name
func (d *BaseDialect) Name() string {
	return d.name
}

// buildSelectSQL is a helper function to build a basic SELECT statement
func buildSelectSQL(dialect Dialect, tableName string, query *SelectQuery) (string, []interface{}, error) {
	var sql strings.Builder
	var args []interface{}
	argIndex := 1

	// Prepend CTE/with clause if present
	if query.With != "" {
		sql.WriteString(query.With)
		if query.With[len(query.With)-1] != ' ' {
			sql.WriteString(" ")
		}
	}

	// SELECT clause
	sql.WriteString("SELECT ")
	if query.Distinct {
		sql.WriteString("DISTINCT ")
	}

	if len(query.Columns) == 0 {
		sql.WriteString("*")
	} else {
		for i, col := range query.Columns {
			if i > 0 {
				sql.WriteString(", ")
			}
			sql.WriteString(dialect.Quote(col))
		}
	}

	// FROM clause
	sql.WriteString(" FROM ")
	sql.WriteString(dialect.Quote(tableName))

	// JOIN clauses
	for _, join := range query.Joins {
		sql.WriteString(fmt.Sprintf(" %s JOIN %s ON %s", join.Type, dialect.Quote(join.Table), join.Condition))
	}

	// WHERE clause
	if len(query.Where) > 0 {
		sql.WriteString(" WHERE ")
		for i, where := range query.Where {
			if i > 0 {
				sql.WriteString(fmt.Sprintf(" %s ", where.Logic))
			}
			sql.WriteString(fmt.Sprintf("%s %s %s", dialect.Quote(where.Column), where.Operator, dialect.Placeholder(argIndex)))
			args = append(args, where.Value)
			argIndex++
		}
	}

	// GROUP BY clause
	if len(query.GroupBy) > 0 {
		sql.WriteString(" GROUP BY ")
		for i, col := range query.GroupBy {
			if i > 0 {
				sql.WriteString(", ")
			}
			sql.WriteString(dialect.Quote(col))
		}
	}

	// HAVING clause
	if len(query.Having) > 0 {
		sql.WriteString(" HAVING ")
		for i, having := range query.Having {
			if i > 0 {
				sql.WriteString(fmt.Sprintf(" %s ", having.Logic))
			}
			sql.WriteString(fmt.Sprintf("%s %s %s", dialect.Quote(having.Column), having.Operator, dialect.Placeholder(argIndex)))
			args = append(args, having.Value)
			argIndex++
		}
	}

	// ORDER BY clause
	if len(query.OrderBy) > 0 {
		sql.WriteString(" ORDER BY ")
		for i, order := range query.OrderBy {
			if i > 0 {
				sql.WriteString(", ")
			}
			sql.WriteString(fmt.Sprintf("%s %s", dialect.Quote(order.Column), order.Direction))
		}
	}

	// LIMIT/OFFSET
	if query.Limit != nil {
		sql.WriteString(fmt.Sprintf(" LIMIT %d", *query.Limit))
	}
	if query.Offset != nil {
		sql.WriteString(fmt.Sprintf(" OFFSET %d", *query.Offset))
	}

	// FOR UPDATE
	if query.ForUpdate {
		sql.WriteString(" FOR UPDATE")
	}

	return sql.String(), args, nil
}

// buildInsertSQL is a helper function to build a basic INSERT statement
func buildInsertSQL(dialect Dialect, tableName string, fields []string, values []interface{}) (string, []interface{}, error) {
	var columns []string
	var placeholders []string

	for i, field := range fields {
		columns = append(columns, dialect.Quote(field))
		placeholders = append(placeholders, dialect.Placeholder(i+1))
	}

	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		dialect.Quote(tableName),
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "))

	return sql, values, nil
}

func buildBatchInsertSQL(dialect Dialect, tableName string, fields []string, allValues [][]interface{}) (string, []interface{}, error) {
	if len(allValues) == 0 {
		return "", nil, fmt.Errorf("no values to insert")
	}

	var columns []string
	for _, field := range fields {
		columns = append(columns, dialect.Quote(field))
	}

	var valueStrings []string
	var allArgs []interface{}
	paramIndex := 1

	for _, values := range allValues {
		var placeholders []string
		for _, val := range values {
			placeholders = append(placeholders, dialect.Placeholder(paramIndex))
			allArgs = append(allArgs, val)
			paramIndex++
		}
		valueStrings = append(valueStrings, "("+strings.Join(placeholders, ", ")+")")
	}

	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s",
		dialect.Quote(tableName),
		strings.Join(columns, ", "),
		strings.Join(valueStrings, ", "))

	return sql, allArgs, nil
}

// buildUpdateSQL is a helper function to build a basic UPDATE statement
func buildUpdateSQL(dialect Dialect, tableName string, fields []string, values []interface{}, whereClause string, whereArgs []interface{}) (string, []interface{}, error) {
	var setClauses []string
	argIndex := 1

	for _, field := range fields {
		setClauses = append(setClauses, fmt.Sprintf("%s = %s", dialect.Quote(field), dialect.Placeholder(argIndex)))
		argIndex++
	}

	sql := fmt.Sprintf("UPDATE %s SET %s WHERE %s",
		dialect.Quote(tableName),
		strings.Join(setClauses, ", "),
		whereClause)

	allArgs := append(values, whereArgs...)
	return sql, allArgs, nil
}

// buildDeleteSQL is a helper function to build a basic DELETE statement
func buildDeleteSQL(dialect Dialect, tableName string, whereClause string, whereArgs []interface{}) (string, []interface{}, error) {
	sql := fmt.Sprintf("DELETE FROM %s WHERE %s",
		dialect.Quote(tableName),
		whereClause)

	return sql, whereArgs, nil
}

func buildBatchDeleteSQL(dialect Dialect, tableName string, pkColumn string, pkValues []interface{}) (string, []interface{}, error) {
	var placeholders []string
	for i := range pkValues {
		placeholders = append(placeholders, dialect.Placeholder(i+1))
	}

	sql := fmt.Sprintf("DELETE FROM %s WHERE %s IN (%s)",
		dialect.Quote(tableName),
		dialect.Quote(pkColumn),
		strings.Join(placeholders, ", "),
	)

	return sql, pkValues, nil
}

// Must be implemented by concrete dialects
func (d *BaseDialect) Quote(identifier string) string {
	panic("Quote method must be implemented by concrete dialect")
}

func (d *BaseDialect) Placeholder(n int) string {
	panic("Placeholder method must be implemented by concrete dialect")
}

func (d *BaseDialect) MapGoTypeToSQL(goType reflect.Type, maxLength int, isAutoIncrement bool) string {
	panic("MapGoTypeToSQL method must be implemented by concrete dialect")
}

func (d *BaseDialect) BuildCreateTable(tableName string, columns []ColumnDefinition) (string, error) {
	panic("BuildCreateTable method must be implemented by concrete dialect")
}

func (d *BaseDialect) BuildDropTable(tableName string) (string, error) {
	panic("BuildDropTable method must be implemented by concrete dialect")
}

func (d *BaseDialect) BuildAddColumn(tableName string, column ColumnDefinition) (string, error) {
	panic("BuildAddColumn method must be implemented by concrete dialect")
}

func (d *BaseDialect) BuildDropColumn(tableName string, columnName string) (string, error) {
	panic("BuildDropColumn method must be implemented by concrete dialect")
}

func (d *BaseDialect) SupportsReturning() bool {
	return false
}

func (d *BaseDialect) SupportsTransactions() bool {
	return true
}

func (d *BaseDialect) SupportsCTE() bool {
	return false
}

func (d *BaseDialect) GetLastInsertID(ctx context.Context, tx *sql.Tx, tableName string) (int64, error) {
	return 0, fmt.Errorf("GetLastInsertID not implemented for this dialect")
}
