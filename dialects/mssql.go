// Package dialects provides SQL Server-specific dialect implementation for GoEF.
package dialects

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"time"

	_ "github.com/denisenkom/go-mssqldb" // SQL Server driver
)

// SQLServerDialect implements the Dialect interface for SQL Server
type SQLServerDialect struct {
	*BaseDialect
}

// NewSQLServerDialect creates a new SQL Server dialect
func NewSQLServerDialect(options map[string]interface{}) *SQLServerDialect {
	return &SQLServerDialect{
		BaseDialect: NewBaseDialect("sqlserver", options),
	}
}

// Quote quotes SQL Server identifiers using square brackets
func (d *SQLServerDialect) Quote(identifier string) string {
	return fmt.Sprintf("[%s]", identifier)
}

// Placeholder returns SQL Server-style placeholders (@p1, @p2, etc.)
func (d *SQLServerDialect) Placeholder(n int) string {
	return fmt.Sprintf("@p%d", n)
}

// MapGoTypeToSQL maps Go types to SQL Server types
func (d *SQLServerDialect) MapGoTypeToSQL(goType reflect.Type, maxLength int, isAutoIncrement bool) string {
	if goType.Kind() == reflect.Ptr {
		goType = goType.Elem()
	}

	switch goType.Kind() {
	case reflect.Bool:
		return TypeBit
	case reflect.Int8:
		return TypeTinyInt
	case reflect.Int16:
		return TypeSmallInt
	case reflect.Int32, reflect.Int:
		if isAutoIncrement {
			return "INT IDENTITY(1,1)"
		}
		return TypeInt
	case reflect.Int64:
		if isAutoIncrement {
			return "BIGINT IDENTITY(1,1)"
		}
		return TypeBigInt
	case reflect.Uint8:
		return TypeTinyInt
	case reflect.Uint16:
		return TypeSmallInt
	case reflect.Uint32, reflect.Uint:
		return TypeBigInt
	case reflect.Uint64:
		return "DECIMAL(20,0)"
	case reflect.Float32:
		return TypeReal
	case reflect.Float64:
		return TypeFloat
	case reflect.String:
		if maxLength > 0 {
			if maxLength <= 4000 {
				return fmt.Sprintf("NVARCHAR(%d)", maxLength)
			}
			return TypeNVarCharMax
		}
		return TypeNVarCharMax
	case reflect.Slice:
		if goType.Elem().Kind() == reflect.Uint8 {
			return TypeVarBinaryMax // []byte
		}
		return TypeNVarCharMax // Store as JSON
	default:
		if goType == reflect.TypeOf(time.Time{}) {
			return TypeDateTime2
		}
		return TypeNVarCharMax
	}
}

// BuildCreateTable builds a SQL Server CREATE TABLE statement
func (d *SQLServerDialect) BuildCreateTable(tableName string, columns []ColumnDefinition) (string, error) {
	var sql strings.Builder

	sql.WriteString(fmt.Sprintf("CREATE TABLE %s (\n", d.Quote(tableName)))

	columnDefs := make([]string, 0, len(columns))
	var primaryKeys []string

	for _, col := range columns {
		colDef := d.buildColumnDefinition(col)
		columnDefs = append(columnDefs, "  "+colDef)

		if col.IsPrimaryKey {
			primaryKeys = append(primaryKeys, d.Quote(col.Name))
		}
	}

	sql.WriteString(strings.Join(columnDefs, ",\n"))

	if len(primaryKeys) > 0 {
		sql.WriteString(",\n")
		sql.WriteString(fmt.Sprintf("  CONSTRAINT %s PRIMARY KEY (%s)",
			d.Quote(fmt.Sprintf("PK_%s", tableName)),
			strings.Join(primaryKeys, ", ")))
	}

	sql.WriteString("\n)")
	return sql.String(), nil
}

func (d *SQLServerDialect) buildColumnDefinition(col ColumnDefinition) string {
	var def strings.Builder

	def.WriteString(d.Quote(col.Name))
	def.WriteString(" ")
	def.WriteString(col.Type)

	if col.IsRequired || col.IsPrimaryKey {
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

// BuildSelect overrides base implementation to handle SQL Server-specific TOP syntax
func (d *SQLServerDialect) BuildSelect(tableName string, query *SelectQuery) (string, []interface{}, error) {
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

	// SELECT clause with TOP
	sql.WriteString("SELECT ")
	if query.Distinct {
		sql.WriteString("DISTINCT ")
	}

	// Handle LIMIT as TOP in SQL Server
	if query.Limit != nil {
		sql.WriteString(fmt.Sprintf("TOP %d ", *query.Limit))
	}

	if len(query.Columns) == 0 {
		sql.WriteString("*")
	} else {
		for i, col := range query.Columns {
			if i > 0 {
				sql.WriteString(", ")
			}
			sql.WriteString(d.Quote(col))
		}
	}

	// FROM clause
	sql.WriteString(" FROM ")
	sql.WriteString(d.Quote(tableName))

	// JOIN clauses
	for _, join := range query.Joins {
		sql.WriteString(fmt.Sprintf(" %s JOIN %s ON %s", join.Type, d.Quote(join.Table), join.Condition))
	}

	// WHERE clause
	if len(query.Where) > 0 {
		sql.WriteString(" WHERE ")
		for i, where := range query.Where {
			if i > 0 {
				sql.WriteString(fmt.Sprintf(" %s ", where.Logic))
			}
			sql.WriteString(fmt.Sprintf("%s %s %s", d.Quote(where.Column), where.Operator, d.Placeholder(argIndex)))
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
			sql.WriteString(d.Quote(col))
		}
	}

	// HAVING clause
	if len(query.Having) > 0 {
		sql.WriteString(" HAVING ")
		for i, having := range query.Having {
			if i > 0 {
				sql.WriteString(fmt.Sprintf(" %s ", having.Logic))
			}
			sql.WriteString(fmt.Sprintf("%s %s %s", d.Quote(having.Column), having.Operator, d.Placeholder(argIndex)))
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
			sql.WriteString(fmt.Sprintf("%s %s", d.Quote(order.Column), order.Direction))
		}
	}

	// SQL Server uses OFFSET/FETCH for pagination (requires ORDER BY)
	if query.Offset != nil {
		if len(query.OrderBy) == 0 {
			sql.WriteString(" ORDER BY (SELECT NULL)")
		}
		sql.WriteString(fmt.Sprintf(" OFFSET %d ROWS", *query.Offset))
		if query.Limit != nil {
			sql.WriteString(fmt.Sprintf(" FETCH NEXT %d ROWS ONLY", *query.Limit))
		}
	}

	return sql.String(), args, nil
}

// BuildInsert builds a SQL Server INSERT with OUTPUT support
func (d *SQLServerDialect) BuildInsert(tableName string, fields []string, values []interface{}) (string, []interface{}, error) {
	var columns []string
	var placeholders []string

	for i, field := range fields {
		columns = append(columns, d.Quote(field))
		placeholders = append(placeholders, d.Placeholder(i+1))
	}

	sql := fmt.Sprintf("INSERT INTO %s (%s) OUTPUT INSERTED.* VALUES (%s)",
		d.Quote(tableName),
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "))

	return sql, values, nil
}

func (d *SQLServerDialect) BuildUpdate(tableName string, fields []string, values []interface{}, whereClause string, whereArgs []interface{}) (string, []interface{}, error) {
	return buildUpdateSQL(d, tableName, fields, values, whereClause, whereArgs)
}

func (d *SQLServerDialect) BuildDelete(tableName string, whereClause string, whereArgs []interface{}) (string, []interface{}, error) {
	return buildDeleteSQL(d, tableName, whereClause, whereArgs)
}

// BuildBatchInsert builds a batch INSERT statement for PostgreSQL using a single multi-row VALUES clause.
func (d *SQLServerDialect) BuildBatchInsert(tableName string, fields []string, allValues [][]interface{}) (string, []interface{}, error) {
	if len(allValues) == 0 {
		return "", nil, fmt.Errorf("no values to insert")
	}

	var columns []string
	for _, field := range fields {
		columns = append(columns, d.Quote(field))
	}

	var valueStrings []string
	var allArgs []interface{}

	for _, values := range allValues {
		var placeholders []string
		for _, val := range values {
			placeholders = append(placeholders, d.Placeholder(0)) // '?' is stateless in MySQL driver
			allArgs = append(allArgs, val)
		}
		valueStrings = append(valueStrings, "("+strings.Join(placeholders, ", ")+")")
	}

	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s",
		d.Quote(tableName),
		strings.Join(columns, ", "),
		strings.Join(valueStrings, ", "))

	return sql, allArgs, nil
}

// BuildBatchDelete builds a batch DELETE statement for PostgreSQL using 'IN'.
func (d *SQLServerDialect) BuildBatchDelete(tableName string, pkColumn string, pkValues []interface{}) (string, []interface{}, error) {
	placeholders := strings.Repeat("?,", len(pkValues))
	placeholders = strings.TrimSuffix(placeholders, ",")

	sql := fmt.Sprintf("DELETE FROM %s WHERE %s IN (%s)",
		d.Quote(tableName),
		d.Quote(pkColumn),
		placeholders,
	)

	return sql, pkValues, nil
}

// BuildDropTable builds a DROP TABLE statement
func (d *SQLServerDialect) BuildDropTable(tableName string) (string, error) {
	return fmt.Sprintf("DROP TABLE IF EXISTS %s", d.Quote(tableName)), nil
}

// BuildAddColumn builds an ADD COLUMN statement
func (d *SQLServerDialect) BuildAddColumn(tableName string, column ColumnDefinition) (string, error) {
	colDef := d.buildColumnDefinition(column)
	return fmt.Sprintf("ALTER TABLE %s ADD %s", d.Quote(tableName), colDef), nil
}

// BuildDropColumn builds a DROP COLUMN statement
func (d *SQLServerDialect) BuildDropColumn(tableName string, columnName string) (string, error) {
	return fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", d.Quote(tableName), d.Quote(columnName)), nil
}

// SupportsReturning returns false for SQL Server (uses OUTPUT instead)
func (d *SQLServerDialect) SupportsReturning() bool {
	return false
}

// SupportsTransactions returns true for SQL Server
func (d *SQLServerDialect) SupportsTransactions() bool {
	return true
}

// SupportsCTE returns true for SQL Server
func (d *SQLServerDialect) SupportsCTE() bool {
	return true
}

// GetLastInsertID gets the last inserted ID using SCOPE_IDENTITY()
func (d *SQLServerDialect) GetLastInsertID(ctx context.Context, tx *sql.Tx, tableName string) (int64, error) {
	var lastID int64
	err := tx.QueryRowContext(ctx, "SELECT SCOPE_IDENTITY()").Scan(&lastID)
	return lastID, err
}
