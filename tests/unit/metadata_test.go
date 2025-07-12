package unit

import (
	"reflect"
	"testing"
	"time"

	"github.com/goef-team/goef/dialects"
	"github.com/goef-team/goef/metadata"
)

// testEntity for testing metadata extraction
type testEntity struct {
	ID        int64      `goef:"primary_key,auto_increment"`
	Name      string     `goef:"required,max_length:100"`
	Email     string     `goef:"required,unique,max_length:255"`
	Age       int        `goef:"min:0,max:150"`
	IsActive  bool       `goef:"default:true"`
	CreatedAt time.Time  `goef:"created_at"`
	UpdatedAt time.Time  `goef:"updated_at"`
	DeletedAt *time.Time `goef:"soft_delete"`
}

func (e *testEntity) GetTableName() string {
	return dialects.TestEntitiesTable
}

func (e *testEntity) GetID() interface{} {
	return e.ID
}

func (e *testEntity) SetID(id interface{}) {
	if v, ok := id.(int64); ok {
		e.ID = v
	}
}

func TestMetadataRegistry_ExtractMetadata(t *testing.T) {
	registry := metadata.NewRegistry()

	entity := &testEntity{}
	meta := registry.GetOrCreate(entity)

	// Test basic metadata
	if meta.Type != reflect.TypeOf(testEntity{}) {
		t.Errorf("Expected type testEntity, got %v", meta.Type)
	}

	if meta.TableName != dialects.TestEntitiesTable {
		t.Errorf("Expected table name '%s', got %s", dialects.TestEntitiesTable, meta.TableName)
	}

	// Test field count
	expectedFieldCount := 8 // ID, Name, Email, Age, IsActive, CreatedAt, UpdatedAt, DeletedAt
	if len(meta.Fields) != expectedFieldCount {
		t.Errorf("Expected %d fields, got %d", expectedFieldCount, len(meta.Fields))
	}

	// Test primary key
	if len(meta.PrimaryKeys) != 1 {
		t.Fatalf("Expected 1 primary key, got %d", len(meta.PrimaryKeys))
	}

	pkField := meta.PrimaryKeys[0]
	if pkField.Name != "ID" {
		t.Errorf("Expected primary key field 'ID', got %s", pkField.Name)
	}

	if !pkField.IsPrimaryKey {
		t.Error("Expected ID field to be marked as primary key")
	}

	if !pkField.IsAutoIncrement {
		t.Error("Expected ID field to be marked as auto increment")
	}

	// Test required fields
	nameField := meta.FieldsByName["Name"]
	if nameField == nil {
		t.Fatal("Name field not found")
	}

	if !nameField.IsRequired {
		t.Error("Expected Name field to be required")
	}

	if nameField.MaxLength != 100 {
		t.Errorf("Expected Name field max length 100, got %d", nameField.MaxLength)
	}

	// Test unique fields
	emailField := meta.FieldsByName["Email"]
	if emailField == nil {
		t.Fatal("Email field not found")
	}

	if !emailField.IsUnique {
		t.Error("Expected Email field to be unique")
	}

	// Test timestamp fields
	createdAtField := meta.FieldsByName["CreatedAt"]
	if createdAtField == nil {
		t.Fatal("CreatedAt field not found")
	}

	if !createdAtField.IsCreatedAt {
		t.Error("Expected CreatedAt field to be marked as created_at")
	}

	deletedAtField := meta.FieldsByName["DeletedAt"]
	if deletedAtField == nil {
		t.Fatal("DeletedAt field not found")
	}

	if !deletedAtField.IsSoftDelete {
		t.Error("Expected DeletedAt field to be marked as soft delete")
	}
}

func TestMetadataRegistry_Caching(t *testing.T) {
	registry := metadata.NewRegistry()

	entity1 := &testEntity{}
	entity2 := &testEntity{}

	meta1 := registry.GetOrCreate(entity1)
	meta2 := registry.GetOrCreate(entity2)

	// Should return the same metadata instance due to caching
	if meta1 != meta2 {
		t.Error("Expected metadata to be cached and return same instance")
	}
}

func TestMetadataRegistry_InvalidEntity(t *testing.T) {
	registry := metadata.NewRegistry()

	// Test with non-struct type
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for non-struct entity")
		}
	}()

	registry.GetOrCreate("not a struct")
}

type EntityWithoutPK struct {
	Name string
}

func (e *EntityWithoutPK) GetTableName() string {
	return "no_pk_entity"
}

func (e *EntityWithoutPK) GetID() interface{} {
	return nil
}

func (e *EntityWithoutPK) SetID(id interface{}) {
	// no-op
}

func TestMetadataRegistry_ValidationNoPrimaryKey(t *testing.T) {
	registry := metadata.NewRegistry()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for entity without primary key")
		}
	}()

	entity := &EntityWithoutPK{}
	registry.GetOrCreate(entity)
}

func TestParseGoEFTags(t *testing.T) {
	tests := []struct {
		tag      string
		expected map[string]string
	}{
		{
			tag: "primary_key,auto_increment",
			expected: map[string]string{
				"primary_key":    "true",
				"auto_increment": "true",
			},
		},
		{
			tag: "required,max_length:100,min:1",
			expected: map[string]string{
				"required":   "true",
				"max_length": "100",
				"min":        "1",
			},
		},
		{
			tag:      "",
			expected: map[string]string{},
		},
		{
			tag: "column:custom_name,unique",
			expected: map[string]string{
				"column": "custom_name",
				"unique": "true",
			},
		},
	}

	for _, test := range tests {
		result := metadata.ParseGoEFTags(test.tag)

		if len(result) != len(test.expected) {
			t.Errorf("Tag %s: expected %d tags, got %d", test.tag, len(test.expected), len(result))
			continue
		}

		for key, expectedValue := range test.expected {
			if actualValue, exists := result[key]; !exists || actualValue != expectedValue {
				t.Errorf("Tag %s: expected %s=%s, got %s=%s", test.tag, key, expectedValue, key, actualValue)
			}
		}
	}
}

func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"testEntity", "test_entity"},
		{"UserProfile", "user_profile"},
		{"ID", "id"},
		{"XMLParser", "xmlparser"},
		{"HTTPResponse", "httpresponse"},
		{"", ""},
		{"A", "a"},
	}

	for _, test := range tests {
		result := metadata.ToSnakeCase(test.input)
		if result != test.expected {
			t.Errorf("ToSnakeCase(%s): expected %s, got %s", test.input, test.expected, result)
		}
	}
}
