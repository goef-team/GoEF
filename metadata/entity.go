// Package metadata provides entity introspection, relationship discovery,
// and metadata caching for the GoEF ORM.
package metadata

import (
	"fmt"
	"log"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

const tagValueTrue = "true"

// EntityMetadata contains comprehensive metadata about an entity type
type EntityMetadata struct {
	Type          reflect.Type
	TableName     string
	PrimaryKeys   []FieldMetadata
	Fields        []FieldMetadata
	Relationships []RelationshipMetadata
	Indexes       []IndexMetadata
	Constraints   []ConstraintMetadata

	// Caching for performance
	FieldsByName   map[string]*FieldMetadata
	FieldsByColumn map[string]*FieldMetadata
	pkFields       []*FieldMetadata
}

// FieldMetadata contains metadata about a single entity field
type FieldMetadata struct {
	Name            string
	Type            reflect.Type
	ColumnName      string
	IsPrimaryKey    bool
	IsAutoIncrement bool
	IsRequired      bool
	IsUnique        bool
	IsSoftDelete    bool
	IsCreatedAt     bool
	IsUpdatedAt     bool
	MaxLength       int
	MinValue        interface{}
	MaxValue        interface{}
	DefaultValue    interface{}
	Tags            map[string]string

	// For relationship fields
	IsRelationship   bool
	RelationshipType RelationshipType
	ForeignKey       string
	RelatedEntity    reflect.Type
}

// RelationshipMetadata describes relationships between entities
type RelationshipMetadata struct {
	Name            string
	Type            RelationshipType
	SourceField     string
	TargetEntity    reflect.Type
	TargetField     string
	ForeignKey      string
	InverseProperty string
	CascadeDelete   bool
	IsRequired      bool
}

// RelationshipType defines the type of relationship between entities
type RelationshipType int

const (
	OneToOne RelationshipType = iota
	OneToMany
	ManyToOne
	ManyToMany
)

// IndexMetadata describes database indexes
type IndexMetadata struct {
	Name     string
	Fields   []string
	IsUnique bool
	Type     IndexType
}

// IndexType defines the type of database index
type IndexType int

const (
	BTreeIndex IndexType = iota
	HashIndex
	GinIndex
	GistIndex
)

// ConstraintMetadata describes database constraints
type ConstraintMetadata struct {
	Name   string
	Type   ConstraintType
	Fields []string
}

// ConstraintType defines the type of database constraint
type ConstraintType int

const (
	PrimaryKeyConstraint ConstraintType = iota
	ForeignKeyConstraint
	UniqueConstraint
	CheckConstraint
)

// Registry manages entity metadata with thread-safe caching
type Registry struct {
	entities map[reflect.Type]*EntityMetadata
	mu       sync.RWMutex
}

// NewRegistry creates a new metadata registry
func NewRegistry() *Registry {
	return &Registry{
		entities: make(map[reflect.Type]*EntityMetadata),
	}
}

// GetOrCreate returns cached metadata or creates new metadata for the given entity type
func (r *Registry) GetOrCreate(entity interface{}) *EntityMetadata {
	entityType := getEntityType(entity)

	r.mu.RLock()
	if meta, exists := r.entities[entityType]; exists {
		r.mu.RUnlock()
		return meta
	}
	r.mu.RUnlock()

	// Create metadata outside of lock to avoid long lock times
	meta, err := r.extractMetadata(entityType)
	if err != nil {
		// In production, we might want to handle this differently
		panic(fmt.Sprintf("failed to extract metadata for %s: %v", entityType.Name(), err))
	}

	r.mu.Lock()
	// Double-check pattern - another goroutine might have added it
	if existing, exists := r.entities[entityType]; exists {
		r.mu.Unlock()
		return existing
	}
	r.entities[entityType] = meta
	r.mu.Unlock()

	return meta
}

// GetAll returns all registered metadata
func (r *Registry) GetAll() []*EntityMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*EntityMetadata, 0, len(r.entities))
	for _, meta := range r.entities {
		result = append(result, meta)
	}
	return result
}

// Clear removes all cached metadata
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entities = make(map[reflect.Type]*EntityMetadata)
}

// extractMetadata extracts metadata from an entity type
func (r *Registry) extractMetadata(entityType reflect.Type) (*EntityMetadata, error) {
	if entityType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("entity must be a struct, got %s", entityType.Kind())
	}

	meta := &EntityMetadata{
		Type:           entityType,
		TableName:      extractTableName(entityType),
		Fields:         make([]FieldMetadata, 0),
		FieldsByName:   make(map[string]*FieldMetadata),
		FieldsByColumn: make(map[string]*FieldMetadata),
	}

	// Collect fields using a recursive function to handle embedded structs
	var collectStructFields func(reflect.Type, bool) error

	collectStructFields = func(t reflect.Type, isEmbedded bool) error {
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)

			if !field.IsExported() {
				continue
			}

			// Handle embedded structs
			if field.Anonymous && field.Type.Kind() == reflect.Struct {
				err := collectStructFields(field.Type, true)
				if err != nil {
					return err
				}
				continue
			}

			fieldMeta, err := r.extractFieldMetadata(field)
			if err != nil {
				return fmt.Errorf("failed to extract field metadata for %s: %w", field.Name, err)
			}

			if fieldMeta == nil {
				continue // Field was skipped
			}

			// Handle field name conflicts (embedded structs can cause this)
			if existing, exists := meta.FieldsByName[fieldMeta.Name]; exists {
				if isEmbedded && !existing.IsPrimaryKey {
					// If current field is from embedded struct and existing isn't a PK, prefer the new one
					// Remove old field from meta.Fields
					for j, f := range meta.Fields {
						if f.Name == existing.Name {
							meta.Fields = append(meta.Fields[:j], meta.Fields[j+1:]...)
							break
						}
					}
					// Remove from FieldsByColumn map as well
					delete(meta.FieldsByColumn, existing.ColumnName)
				} else {
					// Both are embedded. The first one encountered (less nested) wins due to the check above.
					// This path should ideally not be hit if logic is correct.
					continue
				}
			}

			meta.Fields = append(meta.Fields, *fieldMeta)
			meta.FieldsByName[fieldMeta.Name] = fieldMeta
			// Only add to FieldsByColumn if it's not already there or if this one is a PK.
			// PK column names must be unique. Non-PK can theoretically clash if not careful with db design.
			if _, exists := meta.FieldsByColumn[fieldMeta.ColumnName]; !exists || fieldMeta.IsPrimaryKey {
				meta.FieldsByColumn[fieldMeta.ColumnName] = fieldMeta
			}
		}
		return nil
	}

	if err := collectStructFields(entityType, false); err != nil {
		return nil, fmt.Errorf("failed to collect struct fields: %w", err)
	}

	// Populate PrimaryKeys from the collected and resolved Fields
	// This ensures that overriding logic has been applied before deciding PKs.
	for _, fieldMeta := range meta.Fields {
		if fieldMeta.IsPrimaryKey {
			// Avoid adding duplicate PKs by column name.
			// This can happen if two fields in final struct have `primary_key` and same column name.
			isDuplicateColumn := false
			for _, pk := range meta.PrimaryKeys {
				if pk.ColumnName == fieldMeta.ColumnName {
					isDuplicateColumn = true
					break
				}
			}
			if !isDuplicateColumn {
				meta.PrimaryKeys = append(meta.PrimaryKeys, fieldMeta) // Store copy
				// pkFields stores pointers, find the one in meta.Fields
				for i := range meta.Fields {
					if meta.Fields[i].Name == fieldMeta.Name && meta.Fields[i].ColumnName == fieldMeta.ColumnName {
						meta.pkFields = append(meta.pkFields, &meta.Fields[i])
						break
					}
				}
			} else if fieldMeta.Name != meta.FieldsByColumn[fieldMeta.ColumnName].Name {
				// FIXED: Handle empty branch - Log warning for configuration issue
				log.Printf("Warning: Field %s with column %s conflicts with existing primary key field %s",
					fieldMeta.Name, fieldMeta.ColumnName, meta.FieldsByColumn[fieldMeta.ColumnName].Name)
			}
		}
	}

	// Extract relationships
	relationships, err := r.extractRelationships(entityType)
	if err != nil {
		return nil, fmt.Errorf("failed to extract relationships: %w", err)
	}
	meta.Relationships = relationships

	// Extract indexes and constraints
	meta.Indexes = r.extractIndexes(entityType)
	meta.Constraints = r.extractConstraints(entityType)

	// Validate metadata
	if err = r.validateMetadata(meta); err != nil {
		return nil, fmt.Errorf("metadata validation failed: %w", err)
	}

	return meta, nil
}

// extractFieldMetadata extracts metadata for a single field
func (r *Registry) extractFieldMetadata(field reflect.StructField) (*FieldMetadata, error) {
	// Skip non-exported fields
	if !field.IsExported() {
		return nil, nil
	}

	// Skip embedded/anonymous fields (like BaseEntity)
	if field.Anonymous {
		return nil, nil
	}

	goefTag := field.Tag.Get("goef")
	if goefTag == "-" {
		return nil, nil // Skip this field
	}

	fieldMeta := &FieldMetadata{
		Name:       field.Name,
		Type:       field.Type,
		ColumnName: extractColumnName(field),
		Tags:       ParseGoEFTags(goefTag),
	}

	// Check if this is a relationship field FIRST
	if isRelationshipField(field) {
		fieldMeta.IsRelationship = true
		// Set relationship-specific metadata
		if relationshipType := fieldMeta.Tags["relationship"]; relationshipType != "" {
			switch relationshipType {
			case "one_to_one":
				fieldMeta.RelationshipType = OneToOne
			case "many_to_one":
				fieldMeta.RelationshipType = ManyToOne
			case "one_to_many":
				fieldMeta.RelationshipType = OneToMany
			case "many_to_many":
				fieldMeta.RelationshipType = ManyToMany
			}
		}
		fieldMeta.ForeignKey = fieldMeta.Tags["foreign_key"]

		// For relationship fields, extract the related entity type
		relatedType := field.Type
		if relatedType.Kind() == reflect.Ptr {
			relatedType = relatedType.Elem()
		}
		if relatedType.Kind() == reflect.Slice {
			relatedType = relatedType.Elem()
			if relatedType.Kind() == reflect.Ptr {
				relatedType = relatedType.Elem()
			}
		}
		fieldMeta.RelatedEntity = relatedType

		return fieldMeta, nil
	}

	// Parse tag attributes for non-relationship fields
	for key, value := range fieldMeta.Tags {
		switch key {
		case "primary_key":
			fieldMeta.IsPrimaryKey = true
		case "auto_increment":
			fieldMeta.IsAutoIncrement = true
		case "required":
			fieldMeta.IsRequired = true
		case "unique":
			fieldMeta.IsUnique = true
		case "soft_delete":
			fieldMeta.IsSoftDelete = true
		case "created_at":
			fieldMeta.IsCreatedAt = true
		case "updated_at":
			fieldMeta.IsUpdatedAt = true
		case "max_length":
			if length, err := strconv.Atoi(value); err == nil {
				fieldMeta.MaxLength = length
			}
		case "min":
			fieldMeta.MinValue = parseValue(value, field.Type)
		case "max":
			fieldMeta.MaxValue = parseValue(value, field.Type)
		case "default":
			fieldMeta.DefaultValue = parseValue(value, field.Type)
		}
	}

	return fieldMeta, nil
}

// isRelationshipField determines if a field represents a relationship
func isRelationshipField(field reflect.StructField) bool {
	// Skip non-exported fields
	if !field.IsExported() {
		return false
	}

	// Skip embedded fields (Anonymous fields)
	if field.Anonymous {
		return false
	}

	fieldType := field.Type

	// Handle pointers
	if fieldType.Kind() == reflect.Ptr {
		fieldType = fieldType.Elem()
	}

	// Handle slices (for one-to-many and many-to-many)
	if fieldType.Kind() == reflect.Slice {
		fieldType = fieldType.Elem()
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}
	}

	// Must be a struct to be a relationship
	if fieldType.Kind() != reflect.Struct {
		return false
	}

	// Skip common embedded types and built-in types
	typeName := fieldType.Name()
	packagePath := fieldType.PkgPath()

	// Skip embedded base entity types
	if typeName == "BaseEntity" || strings.HasSuffix(typeName, "BaseEntity") {
		return false
	}

	// Skip time types and other built-in types
	if packagePath == "time" || packagePath == "" {
		return false
	}

	// Check if it explicitly has a relationship tag
	goefTag := field.Tag.Get("goef")
	tags := ParseGoEFTags(goefTag)

	if relationshipType := tags["relationship"]; relationshipType != "" {
		return true
	}

	// Default to false unless explicitly marked as relationship
	return false
}

// extractRelationships extracts relationship metadata from struct fields
func (r *Registry) extractRelationships(entityType reflect.Type) ([]RelationshipMetadata, error) {
	var relationships []RelationshipMetadata

	for i := 0; i < entityType.NumField(); i++ {
		field := entityType.Field(i)

		if !isRelationshipField(field) {
			continue
		}

		relationship, err := r.extractRelationshipFromField(field)
		if err != nil {
			return nil, fmt.Errorf("failed to extract relationship from field %s: %w", field.Name, err)
		}

		if relationship != nil {
			relationships = append(relationships, *relationship)
		}
	}

	return relationships, nil
}

// extractRelationshipFromField extracts relationship metadata from a single field
func (r *Registry) extractRelationshipFromField(field reflect.StructField) (*RelationshipMetadata, error) {
	goefTag := field.Tag.Get("goef")
	tags := ParseGoEFTags(goefTag)

	fieldType := field.Type
	isCollection := false

	// Handle pointers
	if fieldType.Kind() == reflect.Ptr {
		fieldType = fieldType.Elem()
	}

	// Handle slices (collections)
	if fieldType.Kind() == reflect.Slice {
		isCollection = true
		fieldType = fieldType.Elem()
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}
	}

	if fieldType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("relationship field %s must point to a struct type", field.Name)
	}

	relationshipTag := tags["relationship"]
	foreignKey := tags["foreign_key"]
	inverseProperty := tags["inverse_property"]

	// FIXED: Use inferForeignKey if not explicitly specified
	if foreignKey == "" {
		foreignKey = inferForeignKey(field.Name, fieldType)
	}

	relationship := &RelationshipMetadata{
		Name:            field.Name,
		SourceField:     field.Name,
		TargetEntity:    fieldType,
		ForeignKey:      foreignKey,
		InverseProperty: inverseProperty,
		CascadeDelete:   tags["cascade_delete"] == tagValueTrue,
		IsRequired:      tags["required"] == tagValueTrue,
	}

	// Rest of the function remains the same...
	// Determine relationship type based on tag and collection status
	switch relationshipTag {
	case "one_to_one":
		relationship.Type = OneToOne
	case "many_to_one":
		relationship.Type = ManyToOne
	case "one_to_many":
		relationship.Type = OneToMany
	case "many_to_many":
		relationship.Type = ManyToMany
	default:
		// Infer from collection status if not explicitly specified
		if isCollection {
			relationship.Type = OneToMany
		} else {
			relationship.Type = ManyToOne
		}
	}

	return relationship, nil
}

// extractIndexes extracts index metadata from struct tags
func (r *Registry) extractIndexes(entityType reflect.Type) []IndexMetadata {
	var indexes []IndexMetadata

	// Extract from field-level tags
	indexMap := make(map[string][]string)
	uniqueIndexMap := make(map[string][]string)

	for i := 0; i < entityType.NumField(); i++ {
		field := entityType.Field(i)
		goefTag := field.Tag.Get("goef")
		tags := ParseGoEFTags(goefTag)

		if indexName := tags["index"]; indexName != "" {
			indexMap[indexName] = append(indexMap[indexName], extractColumnName(field))
		}

		if indexName := tags["unique_index"]; indexName != "" {
			uniqueIndexMap[indexName] = append(uniqueIndexMap[indexName], extractColumnName(field))
		}

		// Single field unique constraint becomes unique index
		if tags["unique"] == tagValueTrue {
			indexName := fmt.Sprintf("uidx_%s_%s", strings.ToLower(entityType.Name()), extractColumnName(field))
			indexes = append(indexes, IndexMetadata{
				Name:     indexName,
				Fields:   []string{extractColumnName(field)},
				IsUnique: true,
				Type:     BTreeIndex,
			})
		}
	}

	// Convert maps to index metadata
	for name, fields := range indexMap {
		indexes = append(indexes, IndexMetadata{
			Name:     name,
			Fields:   fields,
			IsUnique: false,
			Type:     BTreeIndex,
		})
	}

	for name, fields := range uniqueIndexMap {
		indexes = append(indexes, IndexMetadata{
			Name:     name,
			Fields:   fields,
			IsUnique: true,
			Type:     BTreeIndex,
		})
	}

	return indexes
}

// extractConstraints extracts constraint metadata
func (r *Registry) extractConstraints(entityType reflect.Type) []ConstraintMetadata {
	var constraints []ConstraintMetadata

	// Primary key constraint
	var pkFields []string
	for i := 0; i < entityType.NumField(); i++ {
		field := entityType.Field(i)
		goefTag := field.Tag.Get("goef")
		tags := ParseGoEFTags(goefTag)

		if tags["primary_key"] == tagValueTrue {
			pkFields = append(pkFields, extractColumnName(field))
		}
	}

	if len(pkFields) > 0 {
		constraints = append(constraints, ConstraintMetadata{
			Name:   fmt.Sprintf("pk_%s", strings.ToLower(entityType.Name())),
			Type:   PrimaryKeyConstraint,
			Fields: pkFields,
		})
	}

	return constraints
}

// validateMetadata validates the extracted metadata for consistency
func (r *Registry) validateMetadata(meta *EntityMetadata) error {
	// Ensure at least one primary key
	if len(meta.PrimaryKeys) == 0 {
		return fmt.Errorf("entity %s must have at least one primary key field", meta.Type.Name())
	}

	// Validate field names are unique
	fieldNames := make(map[string]bool)
	for _, field := range meta.Fields {
		if fieldNames[field.Name] {
			return fmt.Errorf("duplicate field name: %s", field.Name)
		}
		fieldNames[field.Name] = true
	}

	// Validate column names are unique (only for non-relationship fields)
	columnNames := make(map[string]bool)
	for _, field := range meta.Fields {
		if !field.IsRelationship {
			if columnNames[field.ColumnName] {
				return fmt.Errorf("duplicate column name: %s", field.ColumnName)
			}
			columnNames[field.ColumnName] = true
		}
	}

	return nil
}

// Helper functions

// getEntityType returns the actual type of an entity, handling pointers and interfaces
func getEntityType(entity interface{}) reflect.Type {
	t := reflect.TypeOf(entity)

	// Handle pointers
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	return t
}

// extractTableName extracts the table name from the entity type
func extractTableName(entityType reflect.Type) string {
	// Try to call GetTableName method if it exists
	if method := reflect.New(entityType).MethodByName("GetTableName"); method.IsValid() {
		if results := method.Call(nil); len(results) > 0 {
			if tableName, ok := results[0].Interface().(string); ok && tableName != "" {
				return tableName
			}
		}
	}

	// Default to snake_case of type name
	return ToSnakeCase(entityType.Name())
}

// extractColumnName extracts the column name from a field
func extractColumnName(field reflect.StructField) string {
	// Check for explicit column name in tag
	goefTag := field.Tag.Get("goef")
	tags := ParseGoEFTags(goefTag)

	if columnName := tags["column"]; columnName != "" {
		return columnName
	}

	// Check for db tag (common convention)
	if dbTag := field.Tag.Get("db"); dbTag != "" {
		return dbTag
	}

	// Convert field name to snake_case
	return ToSnakeCase(field.Name)
}

// ParseGoEFTags parses GoEF struct tags into a map
func ParseGoEFTags(tag string) map[string]string {
	tags := make(map[string]string)
	if tag == "" {
		return tags
	}

	parts := strings.Split(tag, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if strings.Contains(part, ":") {
			kv := strings.SplitN(part, ":", 2)
			tags[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		} else {
			tags[part] = tagValueTrue
		}
	}

	return tags
}

// inferForeignKey infers the foreign key name based on the field and target type
func inferForeignKey(fieldName string, targetType reflect.Type) string {
	// FIXED: Replace inefficient string manipulation with TrimSuffix
	fieldName = strings.TrimSuffix(fieldName, "s")
	return ToSnakeCase(fieldName) + "_id"
}

// Utility functions for parsing values
func parseValue(s string, t reflect.Type) interface{} {
	switch t.Kind() {
	case reflect.String:
		return s
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			return v
		}
	case reflect.Float32, reflect.Float64:
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			return v
		}
	case reflect.Bool:
		if v, err := strconv.ParseBool(s); err == nil {
			return v
		}
	default:
		log.Fatalf("unhandled default case")
	}
	return nil
}

// ToSnakeCase converts PascalCase to snake_case, handling acronyms properly
func ToSnakeCase(s string) string {
	if s == "" {
		return ""
	}

	// Handle common special cases
	if s == "ID" {
		return "id"
	}

	var result strings.Builder
	runes := []rune(s)

	for i, r := range runes {
		if unicode.IsUpper(r) {
			// Add underscore before uppercase letters, except:
			// 1. It's the first character
			// 2. Previous character is also uppercase (part of acronym)
			if i > 0 {
				prevIsUpper := unicode.IsUpper(runes[i-1])

				// Only add underscore if previous character was lowercase
				// This handles acronyms by keeping them together: "XMLParser" -> "xmlparser"
				if !prevIsUpper {
					result.WriteRune('_')
				}
			}
			result.WriteRune(unicode.ToLower(r))
		} else {
			result.WriteRune(r)
		}
	}

	return result.String()
}
