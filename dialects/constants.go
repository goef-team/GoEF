package dialects

import (
	"fmt"
	"strings"
)

// dialects/constants.go (continued)
const (
	// Database driver names
	DriverPostgres  = "postgres"
	DriverMySQL     = "mysql"
	DriverSQLite3   = "sqlite3"
	DriverSQLite    = "sqlite"
	DriverSQLServer = "sqlserver"

	// Common SQL keywords
	SQLTrue = "true"

	// Table name for tests
	TestEntitiesTable = "test_entities"

	// Common column types
	MySQLBoolean         = "BOOLEAN"
	MySQLText            = "TEXT"
	PostgreSQLInteger    = "INTEGER"
	SQLServerTinyInt     = "TINYINT"
	SQLServerSmallInt    = "SMALLINT"
	SQLServerBigInt      = "BIGINT"
	SQLServerReal        = "REAL"
	SQLServerNVarCharMax = "NVARCHAR(MAX)"
)

// SQL type constants to reduce duplication
const (
	// Common SQL types
	TypeBoolean  = "BOOLEAN"
	TypeTinyInt  = "TINYINT"
	TypeSmallInt = "SMALLINT"
	TypeInt      = "INT"
	TypeBigInt   = "BIGINT"
	TypeReal     = "REAL"
	TypeFloat    = "FLOAT"
	TypeDouble   = "DOUBLE"
	TypeText     = "TEXT"
	TypeBlob     = "BLOB"
	TypeJSON     = "JSON"
	TypeInteger  = "INTEGER"

	// SQL Server specific
	TypeBit          = "BIT"
	TypeNVarCharMax  = "NVARCHAR(MAX)"
	TypeVarBinaryMax = "VARBINARY(MAX)"
	TypeDateTime2    = "DATETIME2"

	// PostgreSQL specific
	TypeBigSerial       = "BIGSERIAL"
	TypeSerial          = "SERIAL"
	TypeTimestampWithTZ = "TIMESTAMP WITH TIME ZONE"
	TypeBytea           = "BYTEA"

	// MySQL specific
	TypeDateTime      = "DATETIME"
	TypeAutoIncrement = "AUTO_INCREMENT"

	// Common string literals
	StringTrue      = "true"
	StringPostgres  = "postgres"
	StringMySQL     = "mysql"
	StringSQLite    = "sqlite"
	StringSQLServer = "sqlserver"
)

// buildCreateTableSQL provides a shared implementation for CREATE TABLE
func buildCreateTableSQL(dialect Dialect, tableName string, columns []ColumnDefinition) (string, error) {
	var sql strings.Builder

	sql.WriteString(fmt.Sprintf("CREATE TABLE %s (\n", dialect.Quote(tableName)))

	columnDefs := make([]string, 0, len(columns))
	var primaryKeys []string

	for _, col := range columns {
		colDef := buildColumnDefinitionSQL(dialect, col)
		columnDefs = append(columnDefs, "  "+colDef)

		if col.IsPrimaryKey {
			primaryKeys = append(primaryKeys, dialect.Quote(col.Name))
		}
	}

	sql.WriteString(strings.Join(columnDefs, ",\n"))

	if len(primaryKeys) > 0 {
		sql.WriteString(",\n")
		sql.WriteString(fmt.Sprintf("  CONSTRAINT %s PRIMARY KEY (%s)",
			dialect.Quote(fmt.Sprintf("PK_%s", tableName)),
			strings.Join(primaryKeys, ", ")))
	}

	sql.WriteString("\n)")
	return sql.String(), nil
}

// buildColumnDefinitionSQL builds column definition for dialects
func buildColumnDefinitionSQL(dialect Dialect, col ColumnDefinition) string {
	var parts []string

	parts = append(parts, dialect.Quote(col.Name))
	parts = append(parts, col.Type)

	if col.IsRequired {
		parts = append(parts, "NOT NULL")
	}

	if col.IsUnique {
		parts = append(parts, "UNIQUE")
	}

	if col.DefaultValue != nil {
		parts = append(parts, fmt.Sprintf("DEFAULT %v", col.DefaultValue))
	}

	return strings.Join(parts, " ")
}
