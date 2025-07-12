package dialects

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// PostgreSQLDialect implements the Dialect interface for PostgreSQL
type PostgreSQLDialect struct {
	*BaseDialect
}

// NewPostgreSQLDialect creates a new PostgreSQL dialect
func NewPostgreSQLDialect(options map[string]interface{}) *PostgreSQLDialect {
	return &PostgreSQLDialect{
		BaseDialect: NewBaseDialect("postgresql", options),
	}
}

// BuildSelect builds a SELECT statement for PostgreSQL
func (d *PostgreSQLDialect) BuildSelect(tableName string, query *SelectQuery) (string, []interface{}, error) {
	return buildSelectSQL(d, tableName, query)
}

// BuildInsert builds an INSERT statement for PostgreSQL
func (d *PostgreSQLDialect) BuildInsert(tableName string, fields []string, values []interface{}) (string, []interface{}, error) {
	return buildInsertSQL(d, tableName, fields, values)
}

// BuildBatchInsert builds a batch INSERT statement for PostgreSQL using a single multi-row VALUES clause.
func (d *PostgreSQLDialect) BuildBatchInsert(tableName string, fields []string, allValues [][]interface{}) (string, []interface{}, error) {
	return buildBatchInsertSQL(d, tableName, fields, allValues)
}

// BuildUpdate builds an UPDATE statement for PostgreSQL
func (d *PostgreSQLDialect) BuildUpdate(tableName string, fields []string, values []interface{}, whereClause string, whereArgs []interface{}) (string, []interface{}, error) {
	return buildUpdateSQL(d, tableName, fields, values, whereClause, whereArgs)
}

// BuildDelete builds a DELETE statement for PostgreSQL
func (d *PostgreSQLDialect) BuildDelete(tableName string, whereClause string, whereArgs []interface{}) (string, []interface{}, error) {
	return buildDeleteSQL(d, tableName, whereClause, whereArgs)
}

// BuildBatchDelete builds a batch DELETE statement for PostgreSQL using 'IN'.
func (d *PostgreSQLDialect) BuildBatchDelete(tableName string, pkColumn string, pkValues []interface{}) (string, []interface{}, error) {
	return buildBatchDeleteSQL(d, tableName, pkColumn, pkValues)
}

// Quote quotes PostgreSQL identifiers using double quotes
func (d *PostgreSQLDialect) Quote(identifier string) string {
	return fmt.Sprintf(`"%s"`, identifier)
}

// Placeholder returns PostgreSQL-style placeholders ($1, $2, etc.)
func (d *PostgreSQLDialect) Placeholder(n int) string {
	return fmt.Sprintf("$%d", n)
}

// MapGoTypeToSQL maps Go types to PostgreSQL types
func (d *PostgreSQLDialect) MapGoTypeToSQL(goType reflect.Type, maxLength int, isAutoIncrement bool) string {
	// Handle pointers
	if goType.Kind() == reflect.Ptr {
		goType = goType.Elem()
	}

	switch goType.Kind() {
	case reflect.Bool:
		return TypeBoolean
	case reflect.Int8:
		return TypeSmallInt
	case reflect.Int16:
		return TypeSmallInt
	case reflect.Int32, reflect.Int:
		if isAutoIncrement {
			return TypeSerial
		}
		return TypeInteger
	case reflect.Int64:
		if isAutoIncrement {
			return TypeBigSerial
		}
		return TypeBigInt
	case reflect.Uint8:
		return TypeSmallInt
	case reflect.Uint16:
		return TypeInteger
	case reflect.Uint32, reflect.Uint:
		return TypeBigInt
	case reflect.Uint64:
		return "NUMERIC(20,0)"
	case reflect.Float32:
		return TypeReal
	case reflect.Float64:
		return "DOUBLE PRECISION"
	case reflect.String:
		if maxLength > 0 {
			return fmt.Sprintf("VARCHAR(%d)", maxLength)
		}
		return TypeText
	case reflect.Slice:
		if goType.Elem().Kind() == reflect.Uint8 {
			return TypeBytea // []byte
		}
		return "JSONB"
	default:
		if goType == reflect.TypeOf(time.Time{}) {
			return TypeTimestampWithTZ
		}
		return "JSONB"
	}
}

// BuildCreateTable builds a PostgreSQL CREATE TABLE statement
func (d *PostgreSQLDialect) BuildCreateTable(tableName string, columns []ColumnDefinition) (string, error) {
	return buildCreateTableSQL(d, tableName, columns)
}

func (d *PostgreSQLDialect) buildColumnDefinition(col ColumnDefinition) string {
	return buildColumnDefinitionSQL(d, col)
}

// BuildDropTable builds a DROP TABLE statement
func (d *PostgreSQLDialect) BuildDropTable(tableName string) (string, error) {
	return fmt.Sprintf("DROP TABLE IF EXISTS %s", d.Quote(tableName)), nil
}

// BuildAddColumn builds an ADD COLUMN statement
func (d *PostgreSQLDialect) BuildAddColumn(tableName string, column ColumnDefinition) (string, error) {
	colDef := d.buildColumnDefinition(column)
	return fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", d.Quote(tableName), colDef), nil
}

// BuildDropColumn builds a DROP COLUMN statement
func (d *PostgreSQLDialect) BuildDropColumn(tableName string, columnName string) (string, error) {
	return fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", d.Quote(tableName), d.Quote(columnName)), nil
}

// SupportsReturning returns true for PostgreSQL
func (d *PostgreSQLDialect) SupportsReturning() bool {
	return true
}

// SupportsTransactions returns true for PostgreSQL
func (d *PostgreSQLDialect) SupportsTransactions() bool {
	return true
}

// SupportsCTE returns true for PostgreSQL
func (d *PostgreSQLDialect) SupportsCTE() bool {
	return true
}

// GetLastInsertID gets the last inserted ID using RETURNING clause
func (d *PostgreSQLDialect) GetLastInsertID(ctx context.Context, tx *sql.Tx, tableName string) (int64, error) {
	return 0, fmt.Errorf("PostgreSQL uses RETURNING clause for insert IDs")
}
