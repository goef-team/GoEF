package relationship

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"reflect"

	"github.com/goef-team/goef/dialects"
	"github.com/goef-team/goef/metadata"
)

// SelfReferencingManager handles self-referencing relationships
type SelfReferencingManager struct {
	dialect  dialects.Dialect
	metadata *metadata.Registry
}

// NewSelfReferencingManager creates a new self-referencing relationship manager
func NewSelfReferencingManager(dialect dialects.Dialect, metadata *metadata.Registry) *SelfReferencingManager {
	return &SelfReferencingManager{
		dialect:  dialect,
		metadata: metadata,
	}
}

// BuildHierarchicalQuery builds queries for hierarchical data structures
func (srm *SelfReferencingManager) BuildHierarchicalQuery(entityType reflect.Type, rootCondition string, maxDepth int) (*dialects.SelectQuery, error) {
	meta := srm.metadata.GetOrCreate(reflect.New(entityType).Interface())

	// Build CTE (Common Table Expression) for hierarchical queries
	if !srm.dialect.SupportsCTE() {
		return srm.buildRecursiveWithoutCTE(meta, rootCondition, maxDepth)
	}

	return srm.buildRecursiveWithCTE(meta, rootCondition, maxDepth)
}

// buildRecursiveWithCTE builds a recursive CTE query
func (srm *SelfReferencingManager) buildRecursiveWithCTE(meta *metadata.EntityMetadata, rootCondition string, maxDepth int) (*dialects.SelectQuery, error) {
	withClause := fmt.Sprintf(`WITH RECURSIVE hierarchy AS (
               SELECT *, 0 as level, CAST(id AS TEXT) as path
               FROM %s
               WHERE %s

               UNION ALL

               SELECT t.*, h.level + 1, h.path || ',' || CAST(t.id AS TEXT)
               FROM %s t
               INNER JOIN hierarchy h ON t.parent_id = h.id
               WHERE h.level < %d
)`, srm.dialect.Quote(meta.TableName), rootCondition, srm.dialect.Quote(meta.TableName), maxDepth)

	return &dialects.SelectQuery{
		With:    withClause,
		Table:   "hierarchy",
		OrderBy: []dialects.OrderByClause{{Column: "path", Direction: "ASC"}},
	}, nil
}

// buildRecursiveWithoutCTE builds hierarchical queries for databases without CTE support
func (srm *SelfReferencingManager) buildRecursiveWithoutCTE(meta *metadata.EntityMetadata, rootCondition string, maxDepth int) (*dialects.SelectQuery, error) {
	// For databases without CTE, we need to use multiple queries or self-joins
	// This is a simplified approach using self-joins for limited depth

	query := &dialects.SelectQuery{
		Table:   meta.TableName,
		Columns: []string{},
	}

	// Add all columns for the base table
	for _, field := range meta.Fields {
		if !field.IsRelationship {
			query.Columns = append(query.Columns, fmt.Sprintf("t0.%s", field.ColumnName))
		}
	}

	// Add self-joins for each level
	for i := 1; i <= maxDepth; i++ {
		alias := fmt.Sprintf("t%d", i)
		if i == 1 {
			query.Joins = append(query.Joins, dialects.JoinClause{
				Type:      "LEFT",
				Table:     fmt.Sprintf("%s %s", srm.dialect.Quote(meta.TableName), alias),
				Condition: fmt.Sprintf("t0.id = %s.parent_id", alias),
			})
		} else {
			prevAlias := fmt.Sprintf("t%d", i-1)
			query.Joins = append(query.Joins, dialects.JoinClause{
				Type:      "LEFT",
				Table:     fmt.Sprintf("%s %s", srm.dialect.Quote(meta.TableName), alias),
				Condition: fmt.Sprintf("%s.id = %s.parent_id", prevAlias, alias),
			})
		}
	}

	return query, nil
}

// PolymorphicManager handles polymorphic relationships
type PolymorphicManager struct {
	dialect  dialects.Dialect
	metadata *metadata.Registry
}

// NewPolymorphicManager creates a new polymorphic relationship manager
func NewPolymorphicManager(dialect dialects.Dialect, metadata *metadata.Registry) *PolymorphicManager {
	return &PolymorphicManager{
		dialect:  dialect,
		metadata: metadata,
	}
}

// PolymorphicRelationship represents a polymorphic relationship configuration
type PolymorphicRelationship struct {
	ForeignKey       string
	TypeColumn       string
	EntityTypes      map[string]reflect.Type
	DiscriminatorMap map[reflect.Type]string
}

// LoadPolymorphicRelation loads entities from a polymorphic relationship
func (pm *PolymorphicManager) LoadPolymorphicRelation(ctx context.Context, db *sql.DB, entity interface{}, config *PolymorphicRelationship) ([]interface{}, error) {
	entityValue := reflect.ValueOf(entity)
	if entityValue.Kind() == reflect.Ptr {
		entityValue = entityValue.Elem()
	}

	// Get the foreign key value
	var foreignKeyValue interface{}
	meta := pm.metadata.GetOrCreate(entity)
	for _, field := range meta.Fields {
		if field.ColumnName == config.ForeignKey {
			foreignKeyValue = entityValue.FieldByName(field.Name).Interface()
			break
		}
	}

	if foreignKeyValue == nil {
		return nil, fmt.Errorf("foreign key value not found")
	}

	var results []interface{}

	// Load entities for each type in the polymorphic relationship
	for discriminator, entityType := range config.EntityTypes {
		targetMeta := pm.metadata.GetOrCreate(reflect.New(entityType).Interface())

		// Fix: Assign the query to a variable to avoid unused write
		query := &dialects.SelectQuery{
			Table: targetMeta.TableName,
			Where: []dialects.WhereClause{
				{
					Column:   config.ForeignKey,
					Operator: "=",
					Value:    foreignKeyValue,
					Logic:    "AND",
				},
				{
					Column:   config.TypeColumn,
					Operator: "=",
					Value:    discriminator,
					Logic:    "AND",
				},
			},
		}

		// Add all non-relationship columns
		for _, field := range targetMeta.Fields {
			if !field.IsRelationship {
				query.Columns = append(query.Columns, field.ColumnName)
			}
		}

		// Execute query and scan results
		buildSelect, args, err := pm.dialect.BuildSelect(targetMeta.TableName, query)
		if err != nil {
			return nil, fmt.Errorf("failed to build query for type %s: %w", discriminator, err)
		}

		rows, err := db.QueryContext(ctx, buildSelect, args...)
		if err != nil {
			return nil, fmt.Errorf("failed to execute query for type %s: %w", discriminator, err)
		}

		// Scan results
		entities, err := pm.scanPolymorphicEntities(rows, targetMeta, entityType)
		if err != nil {
			err = rows.Close()
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("failed to scan entities for type %s: %w", discriminator, err)
		}
		err = rows.Close()
		if err != nil {
			return nil, err
		}

		results = append(results, entities...)
	}

	return results, nil
}

// scanPolymorphicEntities scans database rows into polymorphic entities
func (pm *PolymorphicManager) scanPolymorphicEntities(rows *sql.Rows, meta *metadata.EntityMetadata, entityType reflect.Type) ([]interface{}, error) {
	var entities []interface{}

	for rows.Next() {
		entity := reflect.New(entityType).Interface()
		entityValue := reflect.ValueOf(entity).Elem()

		// Create scan destinations for non-relationship fields
		var scanDests []interface{}
		for _, field := range meta.Fields {
			if !field.IsRelationship {
				fieldValue := entityValue.FieldByName(field.Name)
				if fieldValue.IsValid() && fieldValue.CanSet() {
					scanDests = append(scanDests, fieldValue.Addr().Interface())
				}
			}
		}

		if err := rows.Scan(scanDests...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		entities = append(entities, entity)
	}

	return entities, rows.Err()
}

// BuildPolymorphicQuery builds a query for polymorphic relationships
func (pm *PolymorphicManager) BuildPolymorphicQuery(entityType reflect.Type, config *PolymorphicRelationship) (*dialects.SelectQuery, error) {
	meta := pm.metadata.GetOrCreate(reflect.New(entityType).Interface())

	// Create the base query
	query := &dialects.SelectQuery{
		Table:   meta.TableName,
		Columns: []string{},
	}

	// Add all non-relationship columns
	for _, field := range meta.Fields {
		if !field.IsRelationship {
			query.Columns = append(query.Columns, field.ColumnName)
		}
	}

	// Add discriminator condition if specified
	if config.TypeColumn != "" {
		if discriminator, exists := config.DiscriminatorMap[entityType]; exists {
			query.Where = append(query.Where, dialects.WhereClause{
				Column:   config.TypeColumn,
				Operator: "=",
				Value:    discriminator,
				Logic:    "AND",
			})
		}
	}

	return query, nil
}

// ManyToManyManager handles complex many-to-many relationships
type ManyToManyManager struct {
	dialect  dialects.Dialect
	metadata *metadata.Registry
}

// NewManyToManyManager creates a new many-to-many relationship manager
func NewManyToManyManager(dialect dialects.Dialect, metadata *metadata.Registry) *ManyToManyManager {
	return &ManyToManyManager{
		dialect:  dialect,
		metadata: metadata,
	}
}

// ManyToManyConfig represents configuration for many-to-many relationships
type ManyToManyConfig struct {
	JoinTable       string
	LeftForeignKey  string
	RightForeignKey string
	ExtraColumns    []string // Additional columns in the join table
}

// BuildManyToManyQuery builds a query for many-to-many relationships
func (mm *ManyToManyManager) BuildManyToManyQuery(leftEntity interface{}, rightEntityType reflect.Type, config *ManyToManyConfig) (*dialects.SelectQuery, error) {
	leftMeta := mm.metadata.GetOrCreate(leftEntity)
	rightMeta := mm.metadata.GetOrCreate(reflect.New(rightEntityType).Interface())

	// Get the primary key value from the left entity
	leftValue := reflect.ValueOf(leftEntity)
	if leftValue.Kind() == reflect.Ptr {
		leftValue = leftValue.Elem()
	}

	var leftPKValue interface{}
	if len(leftMeta.PrimaryKeys) > 0 {
		pkField := leftMeta.PrimaryKeys[0]
		leftPKValue = leftValue.FieldByName(pkField.Name).Interface()
	}

	// Build the query with JOIN
	query := &dialects.SelectQuery{
		Table: rightMeta.TableName,
		Joins: []dialects.JoinClause{
			{
				Type:  "INNER",
				Table: mm.dialect.Quote(config.JoinTable),
				Condition: fmt.Sprintf("%s.%s = %s.%s",
					mm.dialect.Quote(rightMeta.TableName),
					mm.dialect.Quote(rightMeta.PrimaryKeys[0].ColumnName),
					mm.dialect.Quote(config.JoinTable),
					mm.dialect.Quote(config.RightForeignKey)),
			},
		},
		Where: []dialects.WhereClause{
			{
				Column:   fmt.Sprintf("%s.%s", mm.dialect.Quote(config.JoinTable), mm.dialect.Quote(config.LeftForeignKey)),
				Operator: "=",
				Value:    leftPKValue,
				Logic:    "AND",
			},
		},
	}

	// Add columns from the right entity
	for _, field := range rightMeta.Fields {
		if !field.IsRelationship {
			query.Columns = append(query.Columns, fmt.Sprintf("%s.%s",
				mm.dialect.Quote(rightMeta.TableName),
				mm.dialect.Quote(field.ColumnName)))
		}
	}

	// Add extra columns from join table if specified
	for _, extraCol := range config.ExtraColumns {
		query.Columns = append(query.Columns, fmt.Sprintf("%s.%s",
			mm.dialect.Quote(config.JoinTable),
			mm.dialect.Quote(extraCol)))
	}

	return query, nil
}

// LoadManyToManyRelation loads many-to-many related entities
func (mm *ManyToManyManager) LoadManyToManyRelation(ctx context.Context, db *sql.DB, leftEntity interface{}, rightEntityType reflect.Type, config *ManyToManyConfig) ([]interface{}, error) {
	query, err := mm.BuildManyToManyQuery(leftEntity, rightEntityType, config)
	if err != nil {
		return nil, fmt.Errorf("failed to build many-to-many query: %w", err)
	}

	rightMeta := mm.metadata.GetOrCreate(reflect.New(rightEntityType).Interface())

	// Execute the query
	buildSelect, args, err := mm.dialect.BuildSelect(rightMeta.TableName, query)
	if err != nil {
		return nil, fmt.Errorf("failed to build SQL: %w", err)
	}

	rows, err := db.QueryContext(ctx, buildSelect, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer func() {
		if err = rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	// Scan results
	var results []interface{}
	for rows.Next() {
		entity := reflect.New(rightEntityType).Interface()
		entityValue := reflect.ValueOf(entity).Elem()

		// Create scan destinations
		var scanDests []interface{}
		for _, field := range rightMeta.Fields {
			if !field.IsRelationship {
				fieldValue := entityValue.FieldByName(field.Name)
				if fieldValue.IsValid() && fieldValue.CanSet() {
					scanDests = append(scanDests, fieldValue.Addr().Interface())
				}
			}
		}

		// Add scan destinations for extra columns if any
		extraValues := make([]interface{}, len(config.ExtraColumns))
		for i := range config.ExtraColumns {
			extraValues[i] = new(interface{})
			scanDests = append(scanDests, extraValues[i])
		}

		if err = rows.Scan(scanDests...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		results = append(results, entity)
	}

	return results, rows.Err()
}

// AddManyToManyRelation adds a many-to-many relationship between two entities
func (mm *ManyToManyManager) AddManyToManyRelation(ctx context.Context, db *sql.DB, leftEntity, rightEntity interface{}, config *ManyToManyConfig) error {
	leftMeta := mm.metadata.GetOrCreate(leftEntity)
	rightMeta := mm.metadata.GetOrCreate(rightEntity)

	// Get primary key values
	leftValue := reflect.ValueOf(leftEntity)
	if leftValue.Kind() == reflect.Ptr {
		leftValue = leftValue.Elem()
	}

	rightValue := reflect.ValueOf(rightEntity)
	if rightValue.Kind() == reflect.Ptr {
		rightValue = rightValue.Elem()
	}

	var leftPK, rightPK interface{}
	if len(leftMeta.PrimaryKeys) > 0 {
		leftPK = leftValue.FieldByName(leftMeta.PrimaryKeys[0].Name).Interface()
	}
	if len(rightMeta.PrimaryKeys) > 0 {
		rightPK = rightValue.FieldByName(rightMeta.PrimaryKeys[0].Name).Interface()
	}

	// Insert into join table
	fields := []string{config.LeftForeignKey, config.RightForeignKey}
	values := []interface{}{leftPK, rightPK}

	// Add extra column values if specified
	for _, extraCol := range config.ExtraColumns {
		fields = append(fields, extraCol)
		values = append(values, nil) // Default value, could be customized
	}

	buildInsert, args, err := mm.dialect.BuildInsert(config.JoinTable, fields, values)
	if err != nil {
		return fmt.Errorf("failed to build insert SQL: %w", err)
	}

	_, err = db.ExecContext(ctx, buildInsert, args...)
	if err != nil {
		return fmt.Errorf("failed to insert many-to-many relation: %w", err)
	}

	return nil
}

// RemoveManyToManyRelation removes a many-to-many relationship between two entities
func (mm *ManyToManyManager) RemoveManyToManyRelation(ctx context.Context, db *sql.DB, leftEntity, rightEntity interface{}, config *ManyToManyConfig) error {
	leftMeta := mm.metadata.GetOrCreate(leftEntity)
	rightMeta := mm.metadata.GetOrCreate(rightEntity)

	// Get primary key values
	leftValue := reflect.ValueOf(leftEntity)
	if leftValue.Kind() == reflect.Ptr {
		leftValue = leftValue.Elem()
	}

	rightValue := reflect.ValueOf(rightEntity)
	if rightValue.Kind() == reflect.Ptr {
		rightValue = rightValue.Elem()
	}

	var leftPK, rightPK interface{}
	if len(leftMeta.PrimaryKeys) > 0 {
		leftPK = leftValue.FieldByName(leftMeta.PrimaryKeys[0].Name).Interface()
	}
	if len(rightMeta.PrimaryKeys) > 0 {
		rightPK = rightValue.FieldByName(rightMeta.PrimaryKeys[0].Name).Interface()
	}

	// Delete from join table
	whereClause := fmt.Sprintf("%s = %s AND %s = %s",
		mm.dialect.Quote(config.LeftForeignKey), mm.dialect.Placeholder(1),
		mm.dialect.Quote(config.RightForeignKey), mm.dialect.Placeholder(2))
	whereArgs := []interface{}{leftPK, rightPK}

	buildDelete, args, err := mm.dialect.BuildDelete(config.JoinTable, whereClause, whereArgs)
	if err != nil {
		return fmt.Errorf("failed to build delete SQL: %w", err)
	}

	_, err = db.ExecContext(ctx, buildDelete, args...)
	if err != nil {
		return fmt.Errorf("failed to delete many-to-many relation: %w", err)
	}

	return nil
}

// RelationshipCache provides caching for relationship queries
type RelationshipCache struct {
	cache map[string][]interface{}
}

// NewRelationshipCache creates a new relationship cache
func NewRelationshipCache() *RelationshipCache {
	return &RelationshipCache{
		cache: make(map[string][]interface{}),
	}
}

// Get retrieves cached relationship data
func (rc *RelationshipCache) Get(key string) ([]interface{}, bool) {
	data, exists := rc.cache[key]
	return data, exists
}

// Set stores relationship data in cache
func (rc *RelationshipCache) Set(key string, data []interface{}) {
	rc.cache[key] = data
}

// Clear clears all cached data
func (rc *RelationshipCache) Clear() {
	rc.cache = make(map[string][]interface{})
}

// GenerateCacheKey generates a cache key for relationship queries
func (rc *RelationshipCache) GenerateCacheKey(entityType reflect.Type, relationshipName string, foreignKeyValue interface{}) string {
	return fmt.Sprintf("%s_%s_%v", entityType.Name(), relationshipName, foreignKeyValue)
}

// RelationshipValidator validates relationship configurations
type RelationshipValidator struct {
	metadata *metadata.Registry
}

// NewRelationshipValidator creates a new relationship validator
func NewRelationshipValidator(metadata *metadata.Registry) *RelationshipValidator {
	return &RelationshipValidator{
		metadata: metadata,
	}
}

// ValidatePolymorphicConfig validates polymorphic relationship configuration
func (rv *RelationshipValidator) ValidatePolymorphicConfig(config *PolymorphicRelationship) error {
	if config.ForeignKey == "" {
		return fmt.Errorf("foreign key cannot be empty")
	}

	if config.TypeColumn == "" {
		return fmt.Errorf("type column cannot be empty")
	}

	if len(config.EntityTypes) == 0 {
		return fmt.Errorf("entity types cannot be empty")
	}

	// Validate that discriminator map is consistent with entity types
	for discriminator, entityType := range config.EntityTypes {
		if mappedType, exists := config.DiscriminatorMap[entityType]; exists {
			if mappedType != discriminator {
				return fmt.Errorf("inconsistent discriminator mapping for type %s", entityType.Name())
			}
		}
	}

	return nil
}

// ValidateManyToManyConfig validates many-to-many relationship configuration
func (rv *RelationshipValidator) ValidateManyToManyConfig(config *ManyToManyConfig) error {
	if config.JoinTable == "" {
		return fmt.Errorf("join table cannot be empty")
	}

	if config.LeftForeignKey == "" {
		return fmt.Errorf("left foreign key cannot be empty")
	}

	if config.RightForeignKey == "" {
		return fmt.Errorf("right foreign key cannot be empty")
	}

	if config.LeftForeignKey == config.RightForeignKey {
		return fmt.Errorf("left and right foreign keys cannot be the same")
	}

	return nil
}
