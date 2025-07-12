package goef

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode"

	"github.com/goef-team/goef/dialects"
	"github.com/goef-team/goef/metadata"
	"github.com/goef-team/goef/query"
)

// Entity represents a database entity that can be tracked and persisted.
type Entity interface {
	GetID() interface{}
	SetID(id interface{})
	GetTableName() string
}

// BaseEntity provides a default implementation of common entity functionality.
type BaseEntity struct {
	ID        int64      `goef:"primary_key,auto_increment" json:"id"`
	CreatedAt time.Time  `goef:"created_at" json:"created_at"`
	UpdatedAt time.Time  `goef:"updated_at" json:"updated_at"`
	DeletedAt *time.Time `goef:"deleted_at,soft_delete" json:"deleted_at,omitempty"`
}

// GetID returns the primary key value
func (e *BaseEntity) GetID() interface{} {
	return e.ID
}

// SetID sets the primary key value
func (e *BaseEntity) SetID(id interface{}) {
	if v, ok := id.(int64); ok {
		e.ID = v
	}
}

// GetTableName returns the table name based on the struct type
func (e *BaseEntity) GetTableName() string {
	t := reflect.TypeOf(e).Elem()
	return ToSnakeCase(t.Name())
}

// EntityState represents the state of an entity in the context
type EntityState int

const (
	StateUnchanged EntityState = iota
	StateAdded
	StateModified
	StateDeleted
)

// String returns the string representation of the entity state
func (s EntityState) String() string {
	switch s {
	case StateUnchanged:
		return "Unchanged"
	case StateAdded:
		return "Added"
	case StateModified:
		return "Modified"
	case StateDeleted:
		return "Deleted"
	default:
		return "Unknown"
	}
}

// DbSet provides a strongly-typed interface for querying and manipulating entities
type DbSet struct {
	ctx      *DbContext
	metadata *metadata.EntityMetadata
	builder  *query.Builder
}

// Add adds an entity to the context for insertion
func (s *DbSet) Add(entity Entity) error {
	return s.ctx.Add(entity)
}

// Update marks an entity for update
func (s *DbSet) Update(entity Entity) error {
	return s.ctx.Update(entity)
}

// Remove marks an entity for deletion
func (s *DbSet) Remove(entity Entity) error {
	return s.ctx.Remove(entity)
}

// Find finds an entity by its primary key
func (s *DbSet) Find(id interface{}) (Entity, error) {
	if len(s.metadata.PrimaryKeys) == 0 {
		return nil, fmt.Errorf("no primary key defined for entity")
	}

	pkField := s.metadata.PrimaryKeys[0]

	// Build and execute q
	q := &dialects.SelectQuery{
		Table: s.metadata.TableName,
		Where: []dialects.WhereClause{
			{
				Column:   pkField.ColumnName,
				Operator: "=",
				Value:    id,
				Logic:    "AND",
			},
		},
		Limit: intPtr(1),
	}

	// FIXED: Add only non-relationship columns to select
	for _, field := range s.metadata.Fields {
		if !field.IsRelationship { // FIXED: Exclude relationship fields
			q.Columns = append(q.Columns, field.ColumnName)
		}
	}

	buildSelect, args, err := s.ctx.dialect.BuildSelect(s.metadata.TableName, q)
	if err != nil {
		return nil, fmt.Errorf("failed to build select q: %w", err)
	}

	row := s.ctx.db.QueryRow(buildSelect, args...)

	// Create new entity instance
	entity := reflect.New(s.metadata.Type).Interface()

	// FIXED: Scan only non-relationship fields
	err = s.scanEntity(row, entity)
	if err != nil {
		return nil, fmt.Errorf("failed to scan entity: %w", err)
	}

	return entity.(Entity), nil
}

// Where creates a filtered query
func (s *DbSet) Where(column string, operator string, value interface{}) *query.QueryBuilder {
	return s.builder.Where(column, operator, value)
}

// scanEntity scans a database row into an entity
func (s *DbSet) scanEntity(row *sql.Row, entity interface{}) error {
	entityValue := reflect.ValueOf(entity)
	if entityValue.Kind() == reflect.Ptr {
		entityValue = entityValue.Elem()
	}

	// FIXED: Create scan destinations only for non-relationship fields
	var scanDestinations []interface{}
	var scanFields []metadata.FieldMetadata

	for _, field := range s.metadata.Fields {
		if !field.IsRelationship { // FIXED: Only include non-relationship fields
			scanFields = append(scanFields, field)
		}
	}

	scanDestinations = make([]interface{}, len(scanFields))
	for i, field := range scanFields {
		fieldValue := entityValue.FieldByName(field.Name)
		if !fieldValue.IsValid() {
			return fmt.Errorf("field %s not found in entity", field.Name)
		}
		scanDestinations[i] = fieldValue.Addr().Interface()
	}

	return row.Scan(scanDestinations...)
}

// ToSnakeCase converts PascalCase to snake_case. It handles common acronyms like "ID".
func ToSnakeCase(s string) string {
	if s == "ID" {
		return "id"
	}
	var result strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				// Add underscore if previous char was not upper or if current is part of an acronym (e.g. UserID -> user_id)
				// This simplified logic might need refinement for complex cases like "HTTPRequest" -> "http_request"
				// For now, basic PascalCase to snake_case:
				// Add underscore if it's not the first char and ( (prev is lower) or (prev is upper and next is lower) )
				if !unicode.IsUpper(rune(s[i-1])) || (i+1 < len(s) && unicode.IsLower(rune(s[i+1])) && unicode.IsUpper(rune(s[i-1]))) {
					result.WriteRune('_')
				}
			}
			result.WriteRune(unicode.ToLower(r))
		} else {
			result.WriteRune(r)
		}
	}
	return strings.Trim(result.String(), "_") // Trim leading/trailing underscores if any
}

// intPtr returns a pointer to an int
func intPtr(i int) *int {
	return &i
}
