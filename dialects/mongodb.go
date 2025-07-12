// Package dialects provides MongoDB-specific dialect implementation for GoEF.
package dialects

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	mongoOptions "go.mongodb.org/mongo-driver/mongo/options"
)

// MongoDBDialect implements the Dialect interface for MongoDB
type MongoDBDialect struct {
	*BaseDialect
	client   *mongo.Client
	database string
}

func (d *MongoDBDialect) BuildBatchInsert(tableName string, fields []string, allValues [][]interface{}) (string, []interface{}, error) {
	docs := make([]bson.M, 0, len(allValues))
	for _, values := range allValues {
		doc := bson.M{}
		for i, field := range fields {
			if i < len(values) {
				doc[field] = values[i]
			}
		}
		docs = append(docs, doc)
	}

	data, err := bson.MarshalExtJSON(docs, false, false)
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal documents: %w", err)
	}

	return string(data), nil, nil
}

func (d *MongoDBDialect) BuildBatchDelete(tableName string, pkColumn string, pkValues []interface{}) (string, []interface{}, error) {
	filter := bson.M{pkColumn: bson.M{"$in": pkValues}}
	data, err := bson.MarshalExtJSON(filter, false, false)
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal filter: %w", err)
	}
	return string(data), nil, nil
}

// NewMongoDBDialect creates a new MongoDB dialect
func NewMongoDBDialect(options map[string]interface{}) (*MongoDBDialect, error) {
	connectionString, ok := options["connection_string"].(string)
	if !ok {
		return nil, fmt.Errorf("connection_string required for MongoDB")
	}

	databaseName, ok := options["database"].(string)
	if !ok {
		return nil, fmt.Errorf("database name required for MongoDB")
	}

	// Use Background context for the initial connection. In production
	// code the caller should provide a cancellable context, but for this
	// simplified example a background context avoids potential leaks.
	client, err := mongo.Connect(context.Background(), mongoOptions.Client().ApplyURI(connectionString))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	return &MongoDBDialect{
		BaseDialect: NewBaseDialect("mongodb", options),
		client:      client,
		database:    databaseName,
	}, nil
}

// Quote quotes MongoDB identifiers (MongoDB doesn't require quoting)
func (d *MongoDBDialect) Quote(identifier string) string {
	return identifier
}

// Placeholder returns MongoDB-style placeholders (not applicable)
func (d *MongoDBDialect) Placeholder(n int) string {
	return fmt.Sprintf("$%d", n)
}

// MapGoTypeToSQL maps Go types to MongoDB types
func (d *MongoDBDialect) MapGoTypeToSQL(goType reflect.Type, maxLength int, isAutoIncrement bool) string {
	if goType.Kind() == reflect.Ptr {
		goType = goType.Elem()
	}

	switch goType.Kind() {
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "int"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "int"
	case reflect.Float32, reflect.Float64:
		return "double"
	case reflect.String:
		return "string"
	case reflect.Slice:
		return "array"
	case reflect.Map:
		return "object"
	default:
		if goType == reflect.TypeOf(time.Time{}) {
			return "date"
		}
		return "object"
	}
}

// BuildSelect builds a MongoDB find query
func (d *MongoDBDialect) BuildSelect(tableName string, query *SelectQuery) (string, []interface{}, error) {
	filter := bson.M{}

	for _, where := range query.Where {
		switch where.Operator {
		case "=":
			filter[where.Column] = where.Value
		case ">":
			filter[where.Column] = bson.M{"$gt": where.Value}
		case "<":
			filter[where.Column] = bson.M{"$lt": where.Value}
		case ">=":
			filter[where.Column] = bson.M{"$gte": where.Value}
		case "<=":
			filter[where.Column] = bson.M{"$lte": where.Value}
		case "!=", "<>":
			filter[where.Column] = bson.M{"$ne": where.Value}
		case "IN":
			filter[where.Column] = bson.M{"$in": where.Value}
		case "NOT IN":
			filter[where.Column] = bson.M{"$nin": where.Value}
		}
	}

	filterJSON, err := bson.MarshalExtJSON(filter, false, false)
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal filter: %w", err)
	}

	return string(filterJSON), []interface{}{}, nil
}

// BuildInsert builds a MongoDB insert operation
func (d *MongoDBDialect) BuildInsert(tableName string, fields []string, values []interface{}) (string, []interface{}, error) {
	doc := bson.M{}
	for i, field := range fields {
		if i < len(values) {
			doc[field] = values[i]
		}
	}

	docJSON, err := bson.MarshalExtJSON(doc, false, false)
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal document: %w", err)
	}

	return string(docJSON), []interface{}{}, nil
}

// BuildUpdate builds a MongoDB update operation
func (d *MongoDBDialect) BuildUpdate(tableName string, fields []string, values []interface{}, whereClause string, whereArgs []interface{}) (string, []interface{}, error) {
	update := bson.M{"$set": bson.M{}}

	for i, field := range fields {
		if i < len(values) {
			update["$set"].(bson.M)[field] = values[i]
		}
	}

	updateJSON, err := bson.MarshalExtJSON(update, false, false)
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal update: %w", err)
	}

	return string(updateJSON), []interface{}{}, nil
}

// BuildDelete builds a MongoDB delete operation
func (d *MongoDBDialect) BuildDelete(tableName string, whereClause string, whereArgs []interface{}) (string, []interface{}, error) {
	// Simple implementation - in practice, you'd parse the whereClause
	filter := bson.M{}
	filterJSON, err := bson.MarshalExtJSON(filter, false, false)
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal filter: %w", err)
	}

	return string(filterJSON), []interface{}{}, nil
}

// BuildCreateTable creates a MongoDB collection (no-op)
func (d *MongoDBDialect) BuildCreateTable(tableName string, columns []ColumnDefinition) (string, error) {
	return "", nil
}

// BuildDropTable drops a MongoDB collection
func (d *MongoDBDialect) BuildDropTable(tableName string) (string, error) {
	return fmt.Sprintf("db.%s.drop()", tableName), nil
}

// BuildAddColumn adds a field to documents (no-op)
func (d *MongoDBDialect) BuildAddColumn(tableName string, column ColumnDefinition) (string, error) {
	return "", nil
}

// BuildDropColumn removes a field from documents
func (d *MongoDBDialect) BuildDropColumn(tableName string, columnName string) (string, error) {
	update := bson.M{"$unset": bson.M{columnName: ""}}
	updateJSON, err := bson.MarshalExtJSON(update, false, false)
	if err != nil {
		return "", fmt.Errorf("failed to marshal unset: %w", err)
	}
	return string(updateJSON), nil
}

// SupportsReturning returns false for MongoDB
func (d *MongoDBDialect) SupportsReturning() bool {
	return false
}

// SupportsTransactions returns true for MongoDB 4.0+
func (d *MongoDBDialect) SupportsTransactions() bool {
	return true
}

// SupportsCTE returns false for MongoDB
func (d *MongoDBDialect) SupportsCTE() bool {
	return false
}

// GetLastInsertID gets the last inserted ID (not applicable for MongoDB)
func (d *MongoDBDialect) GetLastInsertID(ctx context.Context, tx *sql.Tx, tableName string) (int64, error) {
	return 0, fmt.Errorf("GetLastInsertID not applicable for MongoDB")
}

// GetCollection returns a MongoDB collection
func (d *MongoDBDialect) GetCollection(collectionName string) *mongo.Collection {
	return d.client.Database(d.database).Collection(collectionName)
}

// Close closes the MongoDB connection
func (d *MongoDBDialect) Close() error {
	// Use Background context for disconnect; callers should provide their
	// own context in real applications.
	return d.client.Disconnect(context.Background())
}
