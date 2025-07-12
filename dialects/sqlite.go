// Package dialects provides SQLite-specific dialect implementation for GoEF.
package dialects

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// SQLiteDialect implements the Dialect interface for SQLite
type SQLiteDialect struct {
	*BaseDialect
}

// NewSQLiteDialect creates a new SQLite dialect
func NewSQLiteDialect(options map[string]interface{}) *SQLiteDialect {
	return &SQLiteDialect{
		BaseDialect: NewBaseDialect("sqlite", options),
	}
}

// Quote quotes SQLite identifiers using double quotes
func (d *SQLiteDialect) Quote(identifier string) string {
	return fmt.Sprintf(`"%s"`, identifier)
}

// BuildSelect builds a SELECT statement for SQLite
func (d *SQLiteDialect) BuildSelect(tableName string, query *SelectQuery) (string, []interface{}, error) {
	var builder strings.Builder
	var args []interface{}
	argIndex := 1

	// SELECT clause
	builder.WriteString("SELECT ")
	if query.Distinct {
		builder.WriteString("DISTINCT ")
	}

	if len(query.Columns) == 0 {
		builder.WriteString("*")
	} else {
		for i, col := range query.Columns {
			if i > 0 {
				builder.WriteString(", ")
			}
			// FIXED: Don't quote aggregate functions
			if isAggregateFunction(col) {
				builder.WriteString(col) // Don't quote COUNT(*), SUM(), etc.
			} else {
				builder.WriteString(d.Quote(col))
			}
		}
	}

	// FROM clause
	builder.WriteString(" FROM ")
	builder.WriteString(d.Quote(tableName))

	// JOIN clauses
	for _, join := range query.Joins {
		builder.WriteString(fmt.Sprintf(" %s JOIN %s ON %s", join.Type, d.Quote(join.Table), join.Condition))
	}

	// WHERE clause
	if len(query.Where) > 0 {
		builder.WriteString(" WHERE ")
		for i, where := range query.Where {
			if i > 0 {
				builder.WriteString(fmt.Sprintf(" %s ", where.Logic))
			}
			builder.WriteString(d.Quote(where.Column))
			builder.WriteString(fmt.Sprintf(" %s ", where.Operator))
			builder.WriteString(d.Placeholder(argIndex))
			args = append(args, where.Value)
			argIndex++
		}
	}

	// GROUP BY clause
	if len(query.GroupBy) > 0 {
		builder.WriteString(" GROUP BY ")
		for i, col := range query.GroupBy {
			if i > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString(d.Quote(col))
		}
	}

	// HAVING clause
	if len(query.Having) > 0 {
		builder.WriteString(" HAVING ")
		for i, having := range query.Having {
			if i > 0 {
				builder.WriteString(fmt.Sprintf(" %s ", having.Logic))
			}
			builder.WriteString(d.Quote(having.Column))
			builder.WriteString(fmt.Sprintf(" %s ", having.Operator))
			builder.WriteString(d.Placeholder(argIndex))
			args = append(args, having.Value)
			argIndex++
		}
	}

	// ORDER BY clause
	if len(query.OrderBy) > 0 {
		builder.WriteString(" ORDER BY ")
		for i, order := range query.OrderBy {
			if i > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString(d.Quote(order.Column))
			builder.WriteString(" " + order.Direction)
		}
	}

	// LIMIT clause
	if query.Limit != nil {
		builder.WriteString(fmt.Sprintf(" LIMIT %d", *query.Limit))
	}

	// OFFSET clause
	if query.Offset != nil {
		builder.WriteString(fmt.Sprintf(" OFFSET %d", *query.Offset))
	}

	return builder.String(), args, nil
}

// isAggregateFunction checks if a column specification is an aggregate function
func isAggregateFunction(col string) bool {
	col = strings.ToUpper(strings.TrimSpace(col))
	aggregateFunctions := []string{
		"COUNT(", "SUM(", "AVG(", "MIN(", "MAX(",
		"GROUP_CONCAT(", "TOTAL(", "COUNT(*)",
	}

	for _, fn := range aggregateFunctions {
		if strings.HasPrefix(col, fn) || col == strings.TrimSuffix(fn, "(") {
			return true
		}
	}
	return false
}

// BuildInsert builds an INSERT statement for SQLite
func (d *SQLiteDialect) BuildInsert(tableName string, fields []string, values []interface{}) (string, []interface{}, error) {
	return buildInsertSQL(d, tableName, fields, values)
}

// BuildBatchInsert builds a batch INSERT statement for PostgreSQL using a single multi-row VALUES clause.
func (d *SQLiteDialect) BuildBatchInsert(tableName string, fields []string, allValues [][]interface{}) (string, []interface{}, error) {
	return buildBatchInsertSQL(d, tableName, fields, allValues)
}

// BuildUpdate builds an UPDATE statement for SQLite
func (d *SQLiteDialect) BuildUpdate(tableName string, fields []string, values []interface{}, whereClause string, whereArgs []interface{}) (string, []interface{}, error) {
	return buildUpdateSQL(d, tableName, fields, values, whereClause, whereArgs)
}

// BuildDelete builds a DELETE statement for SQLite
func (d *SQLiteDialect) BuildDelete(tableName string, whereClause string, whereArgs []interface{}) (string, []interface{}, error) {
	return buildDeleteSQL(d, tableName, whereClause, whereArgs)
}

// BuildBatchDelete builds a batch DELETE statement for PostgreSQL using 'IN'.
func (d *SQLiteDialect) BuildBatchDelete(tableName string, pkColumn string, pkValues []interface{}) (string, []interface{}, error) {
	return buildBatchDeleteSQL(d, tableName, pkColumn, pkValues)
}

// Placeholder returns SQLite-style placeholders (?)
func (d *SQLiteDialect) Placeholder(n int) string {
	return "?"
}

// MapGoTypeToSQL maps Go types to SQLite types
func (d *SQLiteDialect) MapGoTypeToSQL(goType reflect.Type, maxLength int, isAutoIncrement bool) string {
	if goType.Kind() == reflect.Ptr {
		goType = goType.Elem()
	}

	switch goType.Kind() {
	case reflect.Bool:
		return TypeBoolean
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int, reflect.Int64:
		if isAutoIncrement {
			return "INTEGER PRIMARY KEY AUTOINCREMENT"
		}
		return TypeInteger
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint, reflect.Uint64:
		if isAutoIncrement {
			return "INTEGER PRIMARY KEY AUTOINCREMENT"
		}
		return TypeInteger
	case reflect.Float32, reflect.Float64:
		return TypeReal
	case reflect.String:
		return TypeText
	case reflect.Slice:
		if goType.Elem().Kind() == reflect.Uint8 {
			return "BLOB"
		}
		return TypeText
	default:
		if goType == reflect.TypeOf(time.Time{}) {
			return "DATETIME"
		}
		return TypeText
	}
}

// BuildCreateTable builds a SQLite CREATE TABLE statement
func (d *SQLiteDialect) BuildCreateTable(tableName string, columns []ColumnDefinition) (string, error) {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("CREATE TABLE %s (\n", d.Quote(tableName)))

	columnDefs := make([]string, 0, len(columns))
	var primaryKeys []string

	for _, col := range columns {
		colDef := d.buildColumnDefinition(col)
		columnDefs = append(columnDefs, "  "+colDef)

		if col.IsPrimaryKey && !col.IsAutoIncrement {
			primaryKeys = append(primaryKeys, d.Quote(col.Name))
		}
	}

	builder.WriteString(strings.Join(columnDefs, ",\n"))

	if len(primaryKeys) > 1 {
		builder.WriteString(",\n")
		builder.WriteString(fmt.Sprintf("  PRIMARY KEY (%s)", strings.Join(primaryKeys, ", ")))
	}

	builder.WriteString("\n)")
	return builder.String(), nil
}

func (d *SQLiteDialect) buildColumnDefinition(col ColumnDefinition) string {
	var def strings.Builder

	def.WriteString(d.Quote(col.Name))
	def.WriteString(" ")

	if col.IsAutoIncrement && col.IsPrimaryKey {
		def.WriteString("INTEGER PRIMARY KEY AUTOINCREMENT")
	} else {
		def.WriteString(col.Type)

		if col.IsPrimaryKey && !col.IsAutoIncrement {
			def.WriteString(" PRIMARY KEY")
		}
	}

	if (col.IsRequired || col.IsPrimaryKey) && (!col.IsAutoIncrement || !col.IsPrimaryKey) {
		def.WriteString(" NOT NULL")
	}

	if col.DefaultValue != nil {
		def.WriteString(fmt.Sprintf(" DEFAULT %v", col.DefaultValue))
	}

	if col.IsUnique && !col.IsPrimaryKey {
		def.WriteString(" UNIQUE")
	}

	return def.String()
}

// BuildDropTable builds a DROP TABLE statement
func (d *SQLiteDialect) BuildDropTable(tableName string) (string, error) {
	return fmt.Sprintf("DROP TABLE IF EXISTS %s", d.Quote(tableName)), nil
}

// BuildAddColumn builds an ADD COLUMN statement
func (d *SQLiteDialect) BuildAddColumn(tableName string, column ColumnDefinition) (string, error) {
	colDef := d.buildColumnDefinition(column)
	return fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", d.Quote(tableName), colDef), nil
}

// BuildDropColumn builds a DROP COLUMN statement
func (d *SQLiteDialect) BuildDropColumn(tableName string, columnName string) (string, error) {
	return fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", d.Quote(tableName), d.Quote(columnName)), nil
}

// SupportsReturning returns true for SQLite 3.35.0+
func (d *SQLiteDialect) SupportsReturning() bool {
	return true
}

// SupportsTransactions returns true for SQLite
func (d *SQLiteDialect) SupportsTransactions() bool {
	return true
}

// SupportsCTE returns true for SQLite
func (d *SQLiteDialect) SupportsCTE() bool {
	return true
}

// GetLastInsertID gets the last inserted ID using last_insert_rowid()
func (d *SQLiteDialect) GetLastInsertID(ctx context.Context, tx *sql.Tx, tableName string) (int64, error) {
	var lastID int64
	err := tx.QueryRowContext(ctx, "SELECT last_insert_rowid()").Scan(&lastID)
	return lastID, err
}
