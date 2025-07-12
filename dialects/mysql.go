// Package dialects provides MySQL-specific dialect implementation for GoEF.
package dialects

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"time"

	_ "github.com/go-sql-driver/mysql" // MySQL driver
)

// MySQLDialect implements the Dialect interface for MySQL
type MySQLDialect struct {
	*BaseDialect
}

// NewMySQLDialect creates a new MySQL dialect
func NewMySQLDialect(options map[string]interface{}) *MySQLDialect {
	return &MySQLDialect{
		BaseDialect: NewBaseDialect("mysql", options),
	}
}

// Quote quotes MySQL identifiers using backticks
func (d *MySQLDialect) Quote(identifier string) string {
	return fmt.Sprintf("`%s`", identifier)
}

// BuildSelect builds a SELECT statement for MySQL
func (d *MySQLDialect) BuildSelect(tableName string, query *SelectQuery) (string, []interface{}, error) {
	// MySQL uses '?' for placeholders, BaseDialect needs to handle this.
	// For now, let's assume buildSelectSQL can be adapted or Placeholder() is used correctly.
	return buildSelectSQL(d, tableName, query)
}

// BuildInsert builds an INSERT statement for MySQL
func (d *MySQLDialect) BuildInsert(tableName string, fields []string, values []interface{}) (string, []interface{}, error) {
	return buildInsertSQL(d, tableName, fields, values)
}

// BuildBatchInsert builds a batch INSERT statement for PostgreSQL using a single multi-row VALUES clause.
func (d *MySQLDialect) BuildBatchInsert(tableName string, fields []string, allValues [][]interface{}) (string, []interface{}, error) {
	return buildBatchInsertSQL(d, tableName, fields, allValues)
}

// BuildUpdate builds an UPDATE statement for MySQL
func (d *MySQLDialect) BuildUpdate(tableName string, fields []string, values []interface{}, whereClause string, whereArgs []interface{}) (string, []interface{}, error) {
	return buildUpdateSQL(d, tableName, fields, values, whereClause, whereArgs)
}

// BuildDelete builds a DELETE statement for MySQL
func (d *MySQLDialect) BuildDelete(tableName string, whereClause string, whereArgs []interface{}) (string, []interface{}, error) {
	return buildDeleteSQL(d, tableName, whereClause, whereArgs)
}

// BuildBatchDelete builds a batch DELETE statement for PostgreSQL using 'IN'.
func (d *MySQLDialect) BuildBatchDelete(tableName string, pkColumn string, pkValues []interface{}) (string, []interface{}, error) {
	return buildBatchDeleteSQL(d, tableName, pkColumn, pkValues)
}

// Placeholder returns MySQL-style placeholders (?)
func (d *MySQLDialect) Placeholder(n int) string {
	return "?"
}

// MapGoTypeToSQL maps Go types to MySQL types
func (d *MySQLDialect) MapGoTypeToSQL(goType reflect.Type, maxLength int, isAutoIncrement bool) string {
	if goType.Kind() == reflect.Ptr {
		goType = goType.Elem()
	}

	switch goType.Kind() {
	case reflect.Bool:
		return TypeBoolean
	case reflect.Int8:
		return TypeTinyInt
	case reflect.Int16:
		return TypeSmallInt
	case reflect.Int32, reflect.Int:
		if isAutoIncrement {
			return fmt.Sprintf("%s %s", TypeInt, TypeAutoIncrement)
		}
		return TypeInt
	case reflect.Int64:
		if isAutoIncrement {
			return fmt.Sprintf("%s %s", TypeBigInt, TypeAutoIncrement)
		}
		return TypeBigInt
	case reflect.Uint8:
		return TypeTinyInt + " UNSIGNED"
	case reflect.Uint16:
		return TypeSmallInt + " UNSIGNED"
	case reflect.Uint32, reflect.Uint:
		return TypeInt + " UNSIGNED"
	case reflect.Uint64:
		return TypeBigInt + " UNSIGNED"
	case reflect.Float32:
		return TypeFloat
	case reflect.Float64:
		return TypeDouble
	case reflect.String:
		if maxLength > 0 {
			if maxLength <= 255 {
				return fmt.Sprintf("VARCHAR(%d)", maxLength)
			}
			return TypeText
		}
		return TypeText
	case reflect.Slice:
		if goType.Elem().Kind() == reflect.Uint8 {
			return TypeBlob
		}
		return TypeJSON
	default:
		if goType == reflect.TypeOf(time.Time{}) {
			return TypeDateTime
		}
		return TypeJSON
	}
}

// BuildCreateTable builds a MySQL CREATE TABLE statement
func (d *MySQLDialect) BuildCreateTable(tableName string, columns []ColumnDefinition) (string, error) {
	return buildCreateTableSQL(d, tableName, columns)
}

func (d *MySQLDialect) buildColumnDefinition(col ColumnDefinition) string {
	return buildColumnDefinitionSQL(d, col)
}

// BuildDropTable builds a DROP TABLE statement
func (d *MySQLDialect) BuildDropTable(tableName string) (string, error) {
	return fmt.Sprintf("DROP TABLE IF EXISTS %s", d.Quote(tableName)), nil
}

// BuildAddColumn builds an ADD COLUMN statement
func (d *MySQLDialect) BuildAddColumn(tableName string, column ColumnDefinition) (string, error) {
	colDef := d.buildColumnDefinition(column)
	return fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", d.Quote(tableName), colDef), nil
}

// BuildDropColumn builds a DROP COLUMN statement
func (d *MySQLDialect) BuildDropColumn(tableName string, columnName string) (string, error) {
	return fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", d.Quote(tableName), d.Quote(columnName)), nil
}

// SupportsReturning returns false for MySQL
func (d *MySQLDialect) SupportsReturning() bool {
	return false
}

// SupportsTransactions returns true for MySQL
func (d *MySQLDialect) SupportsTransactions() bool {
	return true
}

// SupportsCTE returns true for MySQL 8.0+
func (d *MySQLDialect) SupportsCTE() bool {
	return true
}

// GetLastInsertID gets the last inserted ID using LAST_INSERT_ID()
func (d *MySQLDialect) GetLastInsertID(ctx context.Context, tx *sql.Tx, tableName string) (int64, error) {
	var lastID int64
	err := tx.QueryRowContext(ctx, "SELECT LAST_INSERT_ID()").Scan(&lastID)
	return lastID, err
}
