package migration

import (
	"fmt"
	"strings"
	"time"

	"github.com/goef-team/goef/dialects"
	"github.com/goef-team/goef/metadata"
)

// SchemaDiff represents the differences between two schemas
type SchemaDiff struct {
	AddedTables    []TableDiff
	RemovedTables  []TableDiff
	ModifiedTables []TableDiff
}

// TableDiff represents changes to a table
type TableDiff struct {
	Name            string
	AddedColumns    []ColumnDiff
	RemovedColumns  []ColumnDiff
	ModifiedColumns []ColumnDiff
	AddedIndexes    []IndexDiff
	RemovedIndexes  []IndexDiff
}

// ColumnDiff represents changes to a column
type ColumnDiff struct {
	Name    string
	OldType string
	NewType string
	Changes []string
}

// IndexDiff represents changes to an index
type IndexDiff struct {
	Name     string
	Columns  []string
	IsUnique bool
	Action   string // ADD, DROP, MODIFY
}

// SchemaDiffer generates schema differences
type SchemaDiffer struct {
	dialect  dialects.Dialect
	metadata *metadata.Registry
}

// NewSchemaDiffer creates a new schema differ
func NewSchemaDiffer(dialect dialects.Dialect, metadata *metadata.Registry) *SchemaDiffer {
	return &SchemaDiffer{
		dialect:  dialect,
		metadata: metadata,
	}
}

// GenerateCreateMigration generates a migration to create all tables for the given entities
func (d *SchemaDiffer) GenerateCreateMigration(entities []interface{}) (Migration, error) {
	var upSQL strings.Builder
	var downSQL strings.Builder

	// Generate CREATE TABLE statements
	for _, entity := range entities {
		meta := d.metadata.GetOrCreate(entity)

		// Convert metadata to column definitions
		columns := make([]dialects.ColumnDefinition, len(meta.Fields))
		for i, field := range meta.Fields {
			columns[i] = dialects.ColumnDefinition{
				Name:            field.ColumnName,
				Type:            d.dialect.MapGoTypeToSQL(field.Type, field.MaxLength, field.IsAutoIncrement),
				IsPrimaryKey:    field.IsPrimaryKey,
				IsAutoIncrement: field.IsAutoIncrement,
				IsRequired:      field.IsRequired,
				IsUnique:        field.IsUnique,
				DefaultValue:    field.DefaultValue,
				MaxLength:       field.MaxLength,
			}
		}

		// Create table
		createSQL, err := d.dialect.BuildCreateTable(meta.TableName, columns)
		if err != nil {
			return Migration{}, fmt.Errorf("failed to build CREATE TABLE for %s: %w", meta.TableName, err)
		}

		upSQL.WriteString(createSQL)
		upSQL.WriteString(";\n\n")

		// Drop table (for rollback)
		dropSQL, err := d.dialect.BuildDropTable(meta.TableName)
		if err != nil {
			return Migration{}, fmt.Errorf("failed to build DROP TABLE for %s: %w", meta.TableName, err)
		}

		downSQL.WriteString(dropSQL)
		downSQL.WriteString(";\n\n")
	}

	migration := Migration{
		ID:          generateMigrationID(),
		Name:        "InitialCreate",
		Version:     generateVersion(),
		UpSQL:       upSQL.String(),
		DownSQL:     downSQL.String(),
		Description: "Initial database creation",
	}

	return migration, nil
}

// GenerateUpdateMigration generates a migration to update schema based on entity changes
func (d *SchemaDiffer) GenerateUpdateMigration(currentEntities []interface{}, name string) (Migration, error) {
	var upSQL strings.Builder
	var downSQL strings.Builder

	for _, entity := range currentEntities {
		meta := d.metadata.GetOrCreate(entity)

		// Add new columns
		for _, field := range meta.Fields {
			if d.isNewField(field) {
				column := dialects.ColumnDefinition{
					Name:            field.ColumnName,
					Type:            d.dialect.MapGoTypeToSQL(field.Type, field.MaxLength, field.IsAutoIncrement),
					IsPrimaryKey:    field.IsPrimaryKey,
					IsAutoIncrement: field.IsAutoIncrement,
					IsRequired:      field.IsRequired,
					IsUnique:        field.IsUnique,
					DefaultValue:    field.DefaultValue,
					MaxLength:       field.MaxLength,
				}

				addColumnSQL, err := d.dialect.BuildAddColumn(meta.TableName, column)
				if err != nil {
					return Migration{}, fmt.Errorf("failed to build ADD COLUMN: %w", err)
				}

				upSQL.WriteString(addColumnSQL)
				upSQL.WriteString(";\n")

				dropColumnSQL, err := d.dialect.BuildDropColumn(meta.TableName, field.ColumnName)
				if err != nil {
					return Migration{}, fmt.Errorf("failed to build DROP COLUMN: %w", err)
				}

				downSQL.WriteString(dropColumnSQL)
				downSQL.WriteString(";\n")
			}
		}

		for _, index := range meta.Indexes {
			if d.isNewIndex(index) {
				createIndexSQL := d.buildCreateIndex(meta.TableName, index)
				upSQL.WriteString(createIndexSQL)
				upSQL.WriteString(";\n")

				dropIndexSQL := d.buildDropIndex(index.Name)
				downSQL.WriteString(dropIndexSQL)
				downSQL.WriteString(";\n")
			}
		}
	}

	migration := Migration{
		ID:          generateMigrationID(),
		Name:        name,
		Version:     generateVersion(),
		UpSQL:       upSQL.String(),
		DownSQL:     downSQL.String(),
		Description: fmt.Sprintf("Update schema: %s", name),
	}

	return migration, nil
}

func (d *SchemaDiffer) isNewIndex(index metadata.IndexMetadata) bool {
	// This would be based on actual database comparison
	// For now, return false as a placeholder - implement actual logic based on database introspection
	return false
}

// buildCreateIndex builds a CREATE INDEX statement
func (d *SchemaDiffer) buildCreateIndex(tableName string, index metadata.IndexMetadata) string {
	var sql strings.Builder

	sql.WriteString("CREATE ")
	if index.IsUnique {
		sql.WriteString("UNIQUE ")
	}
	sql.WriteString("INDEX ")
	sql.WriteString(d.dialect.Quote(index.Name))
	sql.WriteString(" ON ")
	sql.WriteString(d.dialect.Quote(tableName))
	sql.WriteString(" (")

	for i, field := range index.Fields {
		if i > 0 {
			sql.WriteString(", ")
		}
		sql.WriteString(d.dialect.Quote(field))
	}

	sql.WriteString(")")

	return sql.String()
}

// buildDropIndex builds a DROP INDEX statement
func (d *SchemaDiffer) buildDropIndex(indexName string) string {
	return fmt.Sprintf("DROP INDEX %s", d.dialect.Quote(indexName))
}

// isNewField determines if a field is new (simplified implementation)
func (d *SchemaDiffer) isNewField(field metadata.FieldMetadata) bool {
	// This would be based on actual database comparison
	// For now, return false as a placeholder
	return false
}

// generateMigrationID generates a unique migration ID
func generateMigrationID() string {
	return fmt.Sprintf("migration_%d", generateVersion())
}

// generateVersion generates a version number based on timestamp
func generateVersion() int64 {
	return time.Now().Unix()
}
