package unit

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/goef-team/goef/dialects"
)

func TestPostgreSQLDialect_MapGoTypeToSQL(t *testing.T) {
	dialect := dialects.NewPostgreSQLDialect(nil)

	tests := []struct {
		goType        reflect.Type
		maxLength     int
		autoIncrement bool
		expectedSQL   string
	}{
		{
			goType:        reflect.TypeOf(int64(0)),
			maxLength:     0,
			autoIncrement: true,
			expectedSQL:   "BIGSERIAL",
		},
		{
			goType:        reflect.TypeOf(int64(0)),
			maxLength:     0,
			autoIncrement: false,
			expectedSQL:   "BIGINT",
		},
		{
			goType:        reflect.TypeOf(""),
			maxLength:     100,
			autoIncrement: false,
			expectedSQL:   "VARCHAR(100)",
		},
		{
			goType:        reflect.TypeOf(""),
			maxLength:     0,
			autoIncrement: false,
			expectedSQL:   "TEXT",
		},
		{
			goType:        reflect.TypeOf(true),
			maxLength:     0,
			autoIncrement: false,
			expectedSQL:   "BOOLEAN",
		},
		{
			goType:        reflect.TypeOf(time.Time{}),
			maxLength:     0,
			autoIncrement: false,
			expectedSQL:   "TIMESTAMP WITH TIME ZONE",
		},
		{
			goType:        reflect.TypeOf([]byte{}),
			maxLength:     0,
			autoIncrement: false,
			expectedSQL:   "BYTEA",
		},
	}

	for _, test := range tests {
		result := dialect.MapGoTypeToSQL(test.goType, test.maxLength, test.autoIncrement)
		if result != test.expectedSQL {
			t.Errorf("MapGoTypeToSQL(%v, %d, %v): expected %s, got %s",
				test.goType, test.maxLength, test.autoIncrement, test.expectedSQL, result)
		}
	}
}

func TestPostgreSQLDialect_BuildCreateTable(t *testing.T) {
	dialect := dialects.NewPostgreSQLDialect(nil)

	// Create test column definitions
	columns := []dialects.ColumnDefinition{
		{
			Name:            "id",
			Type:            "BIGSERIAL",
			IsPrimaryKey:    true,
			IsAutoIncrement: true,
			IsRequired:      true,
		},
		{
			Name:       "name",
			Type:       "VARCHAR(100)",
			IsRequired: true,
		},
		{
			Name:       "email",
			Type:       "VARCHAR(255)",
			IsRequired: true,
			IsUnique:   true,
		},
	}

	sql, err := dialect.BuildCreateTable("users", columns)
	if err != nil {
		t.Fatalf("BuildCreateTable failed: %v", err)
	}

	expectedSubstrings := []string{
		`CREATE TABLE "users"`,
		`"id" BIGSERIAL NOT NULL`,
		`"name" VARCHAR(100) NOT NULL`,
		`"email" VARCHAR(255) NOT NULL UNIQUE`,
		`CONSTRAINT "PK_users" PRIMARY KEY ("id")`,
	}

	for _, expected := range expectedSubstrings {
		if !strings.Contains(sql, expected) {
			t.Errorf("Expected SQL to contain '%s', but got:\n%s", expected, sql)
		}
	}
}

func TestPostgreSQLDialect_BuildInsert(t *testing.T) {
	dialect := dialects.NewPostgreSQLDialect(nil)

	fields := []string{"name", "email", "age"}
	values := []interface{}{"John Doe", "john@example.com", 30}

	sql, args, err := dialect.BuildInsert("test_entities", fields, values)
	if err != nil {
		t.Fatalf("BuildInsert failed: %v", err)
	}

	expectedSubstrings := []string{
		`INSERT INTO "test_entities"`,
		`"name"`,
		`"email"`,
		`"age"`,
		`VALUES`,
		`$1`,
		`$2`,
		`$3`,
	}

	for _, expected := range expectedSubstrings {
		if !strings.Contains(sql, expected) {
			t.Errorf("Expected SQL to contain '%s', but got:\n%s", expected, sql)
		}
	}

	if len(args) != 3 {
		t.Errorf("Expected 3 arguments, got %d", len(args))
	}

	expectedArgs := []interface{}{"John Doe", "john@example.com", 30}
	for i, expectedArg := range expectedArgs {
		if i < len(args) && args[i] != expectedArg {
			t.Errorf("Expected arg %d to be %v, got %v", i, expectedArg, args[i])
		}
	}
}
