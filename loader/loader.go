// Package loader provides relationship loading capabilities for GoEF
package loader

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"

	"github.com/goef-team/goef/dialects"
	"github.com/goef-team/goef/metadata"
)

// Loader manages relationship loading for entities
type Loader struct {
	db       *sql.DB
	dialect  dialects.Dialect
	metadata *metadata.Registry
}

// New creates a new loader instance
func New(db *sql.DB, dialect dialects.Dialect, metadata *metadata.Registry) *Loader {
	return &Loader{
		db:       db,
		dialect:  dialect,
		metadata: metadata,
	}
}

// LoadRelation loads a specific relationship for an entity
func (l *Loader) LoadRelation(ctx context.Context, entity interface{}, relationName string) error {
	meta := l.metadata.GetOrCreate(entity)

	// Find the relationship metadata
	var relationship *metadata.RelationshipMetadata
	for _, rel := range meta.Relationships {
		if rel.Name == relationName {
			relationship = &rel
			break
		}
	}

	if relationship == nil {
		return fmt.Errorf("relationship '%s' not found on entity %s", relationName, meta.Type.Name())
	}

	return l.loadRelationship(ctx, entity, relationship)
}

// LoadAllRelations loads all relationships for an entity
func (l *Loader) LoadAllRelations(ctx context.Context, entity interface{}) error {
	meta := l.metadata.GetOrCreate(entity)

	for _, relationship := range meta.Relationships {
		if err := l.loadRelationship(ctx, entity, &relationship); err != nil {
			return fmt.Errorf("failed to load relationship %s: %w", relationship.Name, err)
		}
	}

	return nil
}

// EagerLoad loads relationships during the initial query
func (l *Loader) EagerLoad(ctx context.Context, query *dialects.SelectQuery, includes []string) error {
	// This would modify the query to include JOINs for the specified relationships
	// For now, we'll implement a simplified version

	for _, include := range includes {
		// Parse the include path (e.g., "User.Orders.OrderItems")
		parts := strings.Split(include, ".")
		if len(parts) > 1 {
			// Add JOIN clauses for the relationship path
			if err := l.addJoinForRelationship(query, parts); err != nil {
				return fmt.Errorf("failed to add JOIN for %s: %w", include, err)
			}
		}
	}

	return nil
}

// loadRelationship loads a specific relationship - IMPROVED VERSION
func (l *Loader) loadRelationship(ctx context.Context, entity interface{}, relationship *metadata.RelationshipMetadata) error {
	entityValue := reflect.ValueOf(entity)
	if entityValue.Kind() == reflect.Ptr {
		entityValue = entityValue.Elem()
	}

	// Get the foreign key value - IMPROVED LOGIC
	var foreignKeyValue interface{}
	meta := l.metadata.GetOrCreate(entity)

	// For one-to-many relationships, we need the primary key of the parent entity
	// For many-to-one relationships, we need the foreign key field value
	switch relationship.Type {
	case metadata.OneToMany:
		// For one-to-many, use the primary key of the current entity
		if len(meta.PrimaryKeys) > 0 {
			pkField := meta.PrimaryKeys[0]
			fieldValue := entityValue.FieldByName(pkField.Name)
			if fieldValue.IsValid() {
				foreignKeyValue = fieldValue.Interface()
			}
		}
	case metadata.ManyToOne, metadata.OneToOne:
		// FIXED: Improved foreign key field lookup
		foreignKeyValue = l.findForeignKeyValue(entityValue, meta, relationship)
	}

	if foreignKeyValue == nil || isZeroValue(foreignKeyValue) {
		return fmt.Errorf("foreign key value not found for relationship %s", relationship.Name)
	}

	// Load the related entities
	switch relationship.Type {
	case metadata.OneToOne, metadata.ManyToOne:
		return l.loadSingleRelation(ctx, entity, relationship, foreignKeyValue)
	case metadata.OneToMany:
		return l.loadCollectionRelation(ctx, entity, relationship, foreignKeyValue)
	case metadata.ManyToMany:
		return l.loadManyToManyRelation(ctx, entity, relationship, foreignKeyValue)
	default:
		return fmt.Errorf("unsupported relationship type: %v", relationship.Type)
	}
}

// NEW: findForeignKeyValue method with improved field lookup
func (l *Loader) findForeignKeyValue(entityValue reflect.Value, meta *metadata.EntityMetadata, relationship *metadata.RelationshipMetadata) interface{} {
	foreignKeyName := relationship.ForeignKey

	// Strategy 1: Look for field by column name match
	for _, field := range meta.Fields {
		if field.IsRelationship {
			continue // Skip relationship fields
		}

		// Check direct column name match (most reliable)
		if field.ColumnName == foreignKeyName {
			fieldValue := entityValue.FieldByName(field.Name)
			if fieldValue.IsValid() {
				return fieldValue.Interface()
			}
		}
	}

	// Strategy 2: Look for field by name variations
	for _, field := range meta.Fields {
		if field.IsRelationship {
			continue
		}

		// Check various name patterns
		if strings.EqualFold(field.Name, foreignKeyName) ||
			strings.EqualFold(field.Name, toCamelCase(foreignKeyName)) ||
			strings.EqualFold(field.ColumnName, foreignKeyName) {
			fieldValue := entityValue.FieldByName(field.Name)
			if fieldValue.IsValid() {
				return fieldValue.Interface()
			}
		}
	}

	// Strategy 3: Try common patterns
	possibleFieldNames := []string{
		foreignKeyName,
		toCamelCase(foreignKeyName),                       // blog_id -> BlogId
		toPascalCase(foreignKeyName),                      // blog_id -> BlogID
		relationship.Name + "ID",                          // Blog -> BlogID
		relationship.Name + "Id",                          // Blog -> BlogId
		strings.TrimSuffix(relationship.Name, "s") + "ID", // Blogs -> BlogID
	}

	for _, fieldName := range possibleFieldNames {
		fieldValue := entityValue.FieldByName(fieldName)
		if fieldValue.IsValid() {
			return fieldValue.Interface()
		}
	}

	return nil
}

func toCamelCase(s string) string {
	parts := strings.Split(s, "_")
	if len(parts) == 0 {
		return s
	}

	result := parts[0]
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			result += strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return result
}

// Helper function to convert snake_case to PascalCase
func toPascalCase(s string) string {
	parts := strings.Split(s, "_")
	var result strings.Builder

	for _, part := range parts {
		if len(part) > 0 {
			result.WriteString(strings.ToUpper(part[:1]))
			if len(part) > 1 {
				result.WriteString(part[1:])
			}
		}
	}
	return result.String()
}

// isZeroValue checks if a value is zero/nil
func isZeroValue(value interface{}) bool {
	if value == nil {
		return true
	}

	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.String:
		return v.String() == ""
	case reflect.Bool:
		return !v.Bool()
	case reflect.Ptr, reflect.Interface:
		return v.IsNil()
	default:
		return false
	}
}

// loadSingleRelation loads a single related entity - FIXED VERSION
func (l *Loader) loadSingleRelation(ctx context.Context, entity interface{}, relationship *metadata.RelationshipMetadata, foreignKeyValue interface{}) error {
	// Create an instance of the target entity type
	targetEntity := reflect.New(relationship.TargetEntity).Interface()
	targetMeta := l.metadata.GetOrCreate(targetEntity)

	// For many-to-one relationships, we query by primary key of target entity
	// For one-to-one relationships, we might query by foreign key or primary key depending on direction
	var queryColumn string
	if relationship.Type == metadata.ManyToOne {
		// Query target entity by its primary key
		if len(targetMeta.PrimaryKeys) > 0 {
			queryColumn = targetMeta.PrimaryKeys[0].ColumnName
		}
	} else {
		// For one-to-one, use the target field specified in relationship
		queryColumn = relationship.TargetField
		if queryColumn == "" && len(targetMeta.PrimaryKeys) > 0 {
			queryColumn = targetMeta.PrimaryKeys[0].ColumnName
		}
	}

	if queryColumn == "" {
		return fmt.Errorf("cannot determine query column for relationship %s", relationship.Name)
	}

	// Build query to load the related entity
	query := &dialects.SelectQuery{
		Table:   targetMeta.TableName,
		Columns: []string{},
		Where: []dialects.WhereClause{
			{
				Column:   queryColumn,
				Operator: "=",
				Value:    foreignKeyValue,
				Logic:    "AND",
			},
		},
	}

	// FIXED: Only add non-relationship fields (actual database columns)
	for _, field := range targetMeta.Fields {
		if !field.IsRelationship { // Skip relationship fields
			query.Columns = append(query.Columns, field.ColumnName)
		}
	}

	// Execute query
	builtSql, args, err := l.dialect.BuildSelect(targetMeta.TableName, query)
	if err != nil {
		return fmt.Errorf("failed to build select query: %w", err)
	}

	row := l.db.QueryRowContext(ctx, builtSql, args...)

	// Scan the result
	relatedEntity, err := l.scanSingleEntity(row, targetMeta)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil // No related entity found
		}
		return fmt.Errorf("failed to scan related entity: %w", err)
	}

	// Set the relationship property
	entityValue := reflect.ValueOf(entity)
	if entityValue.Kind() == reflect.Ptr {
		entityValue = entityValue.Elem()
	}

	relationField := entityValue.FieldByName(relationship.Name)
	if !relationField.IsValid() {
		return fmt.Errorf("relationship field %s not found", relationship.Name)
	}

	if relationField.CanSet() {
		relationField.Set(reflect.ValueOf(relatedEntity))
	}

	return nil
}

// loadCollectionRelation loads a collection of related entities - FIXED VERSION
func (l *Loader) loadCollectionRelation(ctx context.Context, entity interface{}, relationship *metadata.RelationshipMetadata, foreignKeyValue interface{}) error {
	// Create an instance of the target entity type
	targetEntity := reflect.New(relationship.TargetEntity).Interface()
	targetMeta := l.metadata.GetOrCreate(targetEntity)

	// For one-to-many relationships, query target entities by their foreign key pointing back to this entity
	queryColumn := relationship.ForeignKey
	if queryColumn == "" {
		// Try to infer the foreign key column name
		entityMeta := l.metadata.GetOrCreate(entity)
		queryColumn = strings.ToLower(entityMeta.Type.Name()) + "_id"
	}

	// Build query to load the related entities
	query := &dialects.SelectQuery{
		Table:   targetMeta.TableName,
		Columns: []string{},
		Where: []dialects.WhereClause{
			{
				Column:   queryColumn,
				Operator: "=",
				Value:    foreignKeyValue,
				Logic:    "AND",
			},
		},
	}

	// FIXED: Only add non-relationship fields (actual database columns)
	for _, field := range targetMeta.Fields {
		if !field.IsRelationship { // Skip relationship fields
			query.Columns = append(query.Columns, field.ColumnName)
		}
	}

	// Execute query
	sql, args, err := l.dialect.BuildSelect(targetMeta.TableName, query)
	if err != nil {
		return fmt.Errorf("failed to build select query: %w", err)
	}

	rows, err := l.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to execute query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Scan all results
	var relatedEntities []interface{}
	for rows.Next() {
		relatedEntity, err := l.scanSingleEntityFromRows(rows, targetMeta)
		if err != nil {
			return fmt.Errorf("failed to scan related entity: %w", err)
		}
		relatedEntities = append(relatedEntities, relatedEntity)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error reading rows: %w", err)
	}

	// Set the relationship property
	entityValue := reflect.ValueOf(entity)
	if entityValue.Kind() == reflect.Ptr {
		entityValue = entityValue.Elem()
	}

	relationField := entityValue.FieldByName(relationship.Name)
	if !relationField.IsValid() {
		return fmt.Errorf("relationship field %s not found", relationship.Name)
	}

	// Create slice of the appropriate type
	if len(relatedEntities) > 0 {
		sliceType := reflect.SliceOf(relationship.TargetEntity)
		sliceValue := reflect.MakeSlice(sliceType, len(relatedEntities), len(relatedEntities))

		for i, relatedEntity := range relatedEntities {
			entityValue := reflect.ValueOf(relatedEntity)
			if entityValue.Kind() == reflect.Ptr {
				entityValue = entityValue.Elem()
			}
			sliceValue.Index(i).Set(entityValue)
		}

		if relationField.CanSet() {
			relationField.Set(sliceValue)
		}
	} else {
		// Set empty slice
		sliceType := reflect.SliceOf(relationship.TargetEntity)
		emptySlice := reflect.MakeSlice(sliceType, 0, 0)
		if relationField.CanSet() {
			relationField.Set(emptySlice)
		}
	}

	return nil
}

// loadManyToManyRelation loads many-to-many relationships
func (l *Loader) loadManyToManyRelation(ctx context.Context, entity interface{}, relationship *metadata.RelationshipMetadata, foreignKeyValue interface{}) error {
	// Many-to-many relationships require a join table
	// This is a simplified implementation
	return fmt.Errorf("many-to-many relationships not yet implemented")
}

// addJoinForRelationship adds JOIN clauses for a relationship path
func (l *Loader) addJoinForRelationship(query *dialects.SelectQuery, relationshipPath []string) error {
	// This would analyze the relationship path and add appropriate JOINs
	// For now, return nil as a placeholder
	return nil
}

// LazyLoader provides lazy loading capabilities
type LazyLoader struct {
	*Loader
	loadedRelations map[string]bool
}

// NewLazyLoader creates a new lazy loader
func NewLazyLoader(db *sql.DB, dialect dialects.Dialect, metadata *metadata.Registry) *LazyLoader {
	return &LazyLoader{
		Loader:          New(db, dialect, metadata),
		loadedRelations: make(map[string]bool),
	}
}

// Load loads a relationship if not already loaded
func (ll *LazyLoader) Load(ctx context.Context, entity interface{}, relationName string) error {
	entityKey := fmt.Sprintf("%p_%s", entity, relationName)

	if ll.loadedRelations[entityKey] {
		return nil // Already loaded
	}

	err := ll.LoadRelation(ctx, entity, relationName)
	if err != nil {
		return err
	}

	ll.loadedRelations[entityKey] = true
	return nil
}

// IsLoaded checks if a relationship is already loaded
func (ll *LazyLoader) IsLoaded(entity interface{}, relationName string) bool {
	entityKey := fmt.Sprintf("%p_%s", entity, relationName)
	return ll.loadedRelations[entityKey]
}
