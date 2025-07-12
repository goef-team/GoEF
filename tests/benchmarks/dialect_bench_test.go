package benchmarks

import (
	"reflect"
	"testing"
	"time"

	"github.com/goef-team/goef/dialects"
)

func BenchmarkPostgreSQLDialect_BuildSelect(b *testing.B) {
	dialect := dialects.NewPostgreSQLDialect(nil)

	query := &dialects.SelectQuery{
		Table:   "users",
		Columns: []string{"id", "name", "email", "age"},
		Where: []dialects.WhereClause{
			{Column: "age", Operator: ">", Value: 18, Logic: "AND"},
			{Column: "is_active", Operator: "=", Value: true, Logic: "AND"},
		},
		OrderBy: []dialects.OrderByClause{
			{Column: "name", Direction: "ASC"},
		},
		Limit:  intPtr(100),
		Offset: intPtr(20),
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, err := dialect.BuildSelect("users", query)
		if err != nil {
			b.Fatalf("Failed to build select: %v", err)
		}
	}
}

func BenchmarkMySQLDialect_BuildSelect(b *testing.B) {
	dialect := dialects.NewMySQLDialect(nil)

	query := &dialects.SelectQuery{
		Table:   "users",
		Columns: []string{"id", "name", "email", "age"},
		Where: []dialects.WhereClause{
			{Column: "age", Operator: ">", Value: 18, Logic: "AND"},
			{Column: "is_active", Operator: "=", Value: true, Logic: "AND"},
		},
		OrderBy: []dialects.OrderByClause{
			{Column: "name", Direction: "ASC"},
		},
		Limit:  intPtr(100),
		Offset: intPtr(20),
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, err := dialect.BuildSelect("users", query)
		if err != nil {
			b.Fatalf("Failed to build select: %v", err)
		}
	}
}

func BenchmarkSQLiteDialect_BuildSelect(b *testing.B) {
	dialect := dialects.NewSQLiteDialect(nil)

	query := &dialects.SelectQuery{
		Table:   "users",
		Columns: []string{"id", "name", "email", "age"},
		Where: []dialects.WhereClause{
			{Column: "age", Operator: ">", Value: 18, Logic: "AND"},
			{Column: "is_active", Operator: "=", Value: true, Logic: "AND"},
		},
		OrderBy: []dialects.OrderByClause{
			{Column: "name", Direction: "ASC"},
		},
		Limit:  intPtr(100),
		Offset: intPtr(20),
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, err := dialect.BuildSelect("users", query)
		if err != nil {
			b.Fatalf("Failed to build select: %v", err)
		}
	}
}

func BenchmarkDialect_BuildInsert(b *testing.B) {
	dialects := []struct {
		name    string
		dialect dialects.Dialect
	}{
		{"PostgreSQL", dialects.NewPostgreSQLDialect(nil)},
		{"MySQL", dialects.NewMySQLDialect(nil)},
		{"SQLite", dialects.NewSQLiteDialect(nil)},
	}

	fields := []string{"name", "email", "age", "is_active"}
	values := []interface{}{"John Doe", "john@example.com", 30, true}

	for _, d := range dialects {
		b.Run(d.name, func(b *testing.B) {
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, _, err := d.dialect.BuildInsert("users", fields, values)
				if err != nil {
					b.Fatalf("Failed to build insert: %v", err)
				}
			}
		})
	}
}

func BenchmarkDialect_BuildUpdate(b *testing.B) {
	dialects := []struct {
		name    string
		dialect dialects.Dialect
	}{
		{"PostgreSQL", dialects.NewPostgreSQLDialect(nil)},
		{"MySQL", dialects.NewMySQLDialect(nil)},
		{"SQLite", dialects.NewSQLiteDialect(nil)},
	}

	fields := []string{"name", "email", "age"}
	values := []interface{}{"Jane Doe", "jane@example.com", 25}
	whereClause := "id = ?"
	whereArgs := []interface{}{1}

	for _, d := range dialects {
		b.Run(d.name, func(b *testing.B) {
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, _, err := d.dialect.BuildUpdate("users", fields, values, whereClause, whereArgs)
				if err != nil {
					b.Fatalf("Failed to build update: %v", err)
				}
			}
		})
	}
}

func BenchmarkDialect_MapGoTypeToSQL(b *testing.B) {
	dialect := dialects.NewPostgreSQLDialect(nil)

	types := []reflect.Type{
		reflect.TypeOf(int(0)),
		reflect.TypeOf(int64(0)),
		reflect.TypeOf(string("")),
		reflect.TypeOf(bool(false)),
		reflect.TypeOf(time.Time{}),
		reflect.TypeOf([]byte{}),
		reflect.TypeOf(float64(0)),
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, t := range types {
			dialect.MapGoTypeToSQL(t, 255, false)
		}
	}
}

func intPtr(i int) *int {
	return &i
}
