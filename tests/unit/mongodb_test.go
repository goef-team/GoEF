package unit

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/goef-team/goef/dialects"
	"github.com/goef-team/goef/metadata"
)

func TestMongoDBDialect_Name(t *testing.T) {
	options := map[string]interface{}{
		"connection_string": "mongodb://localhost:27017",
		"database":          "test_db",
	}

	dialect, err := dialects.NewMongoDBDialect(options)
	if err != nil {
		t.Fatalf("Failed to create MongoDB dialect: %v", err)
	}

	if dialect.Name() != "mongodb" {
		t.Errorf("Expected name 'mongodb', got %s", dialect.Name())
	}
}

func TestMongoDBDialect_Quote(t *testing.T) {
	options := map[string]interface{}{
		"connection_string": "mongodb://localhost:27017",
		"database":          "test_db",
	}

	dialect, err := dialects.NewMongoDBDialect(options)
	if err != nil {
		t.Fatalf("Failed to create MongoDB dialect: %v", err)
	}

	// MongoDB doesn't require quoting
	result := dialect.Quote("field_name")
	if result != "field_name" {
		t.Errorf("Expected 'field_name', got %s", result)
	}
}

func TestMongoDBDialect_MapGoTypeToSQL(t *testing.T) {
	options := map[string]interface{}{
		"connection_string": "mongodb://localhost:27017",
		"database":          "test_db",
	}

	dialect, err := dialects.NewMongoDBDialect(options)
	if err != nil {
		t.Fatalf("Failed to create MongoDB dialect: %v", err)
	}

	tests := []struct {
		goType      reflect.Type
		expectedSQL string
	}{
		{reflect.TypeOf(true), "boolean"},
		{reflect.TypeOf(int(0)), "int"},
		{reflect.TypeOf(int64(0)), "int"},
		{reflect.TypeOf(float64(0)), "double"},
		{reflect.TypeOf(""), "string"},
		{reflect.TypeOf([]string{}), "array"},
		{reflect.TypeOf(map[string]interface{}{}), "object"},
		{reflect.TypeOf(time.Time{}), "date"},
	}

	for _, test := range tests {
		result := dialect.MapGoTypeToSQL(test.goType, 100, true)
		if result != test.expectedSQL {
			t.Errorf("Expected %s for type %v, got %s", test.expectedSQL, test.goType, result)
		}
	}
}

func TestMongoDBDialect_BuildSelect(t *testing.T) {
	options := map[string]interface{}{
		"connection_string": "mongodb://localhost:27017",
		"database":          "test_db",
	}

	dialect, err := dialects.NewMongoDBDialect(options)
	if err != nil {
		t.Fatalf("Failed to create MongoDB dialect: %v", err)
	}

	meta := &metadata.EntityMetadata{
		TableName: "users",
	}

	query := &dialects.SelectQuery{
		Table: "users",
		Where: []dialects.WhereClause{
			{Column: "age", Operator: ">", Value: 18},
			{Column: "active", Operator: "=", Value: true},
		},
	}

	sql, args, err := dialect.BuildSelect(meta.TableName, query)
	if err != nil {
		t.Fatalf("Failed to build select: %v", err)
	}

	if sql == "" {
		t.Error("Expected non-empty SQL")
	}

	if len(args) != 0 {
		t.Errorf("Expected 0 args for MongoDB, got %d", len(args))
	}

	// Should contain MongoDB query structure
	if !strings.Contains(sql, "age") {
		t.Error("Expected SQL to contain 'age' field")
	}
}

func TestMongoDBDialect_SupportsFeatures(t *testing.T) {
	options := map[string]interface{}{
		"connection_string": "mongodb://localhost:27017",
		"database":          "test_db",
	}

	dialect, err := dialects.NewMongoDBDialect(options)
	if err != nil {
		t.Fatalf("Failed to create MongoDB dialect: %v", err)
	}

	// Test feature support
	if dialect.SupportsReturning() {
		t.Error("MongoDB should not support RETURNING")
	}

	if !dialect.SupportsTransactions() {
		t.Error("MongoDB should support transactions")
	}

	if dialect.SupportsCTE() {
		t.Error("MongoDB should not support CTE")
	}
}
