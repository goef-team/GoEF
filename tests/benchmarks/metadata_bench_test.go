package benchmarks

import (
	"testing"
	"time"

	"github.com/goef-team/goef/metadata"
)

func BenchmarkMetadataRegistry_GetOrCreate(b *testing.B) {
	registry := metadata.NewRegistry()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		entity := &TestEntityForBench{}
		registry.GetOrCreate(entity)
	}
}

func BenchmarkMetadataRegistry_GetOrCreateCached(b *testing.B) {
	registry := metadata.NewRegistry()

	// Warm up cache
	entity := &TestEntityForBench{}
	registry.GetOrCreate(entity)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		registry.GetOrCreate(entity)
	}
}

func BenchmarkMetadataExtraction_ComplexEntity(b *testing.B) {
	type ComplexEntity struct {
		ID          int64                  `goef:"primary_key,auto_increment"`
		Name        string                 `goef:"required,max_length:100"`
		Email       string                 `goef:"required,unique,max_length:255"`
		Age         int                    `goef:"min:0,max:150"`
		IsActive    bool                   `goef:"default:true"`
		Score       float64                `goef:"min:0.0,max:100.0"`
		Tags        []string               `goef:"json"`
		CreatedAt   time.Time              `goef:"created_at"`
		UpdatedAt   time.Time              `goef:"updated_at"`
		DeletedAt   *time.Time             `goef:"soft_delete"`
		Profile     string                 `goef:"text"`
		Bio         string                 `goef:"text,max_length:2000"`
		Website     string                 `goef:"url,max_length:500"`
		Phone       string                 `goef:"phone,max_length:20"`
		Address     string                 `goef:"address,max_length:500"`
		City        string                 `goef:"max_length:100"`
		State       string                 `goef:"max_length:50"`
		ZipCode     string                 `goef:"max_length:10"`
		Country     string                 `goef:"max_length:100"`
		Preferences map[string]interface{} `goef:"json"`
	}

	registry := metadata.NewRegistry()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		entity := &ComplexEntity{}
		registry.GetOrCreate(entity)
	}
}

func BenchmarkParseGoEFTags_Simple(b *testing.B) {
	tag := "primary_key,auto_increment"

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		metadata.ParseGoEFTags(tag)
	}
}

func BenchmarkParseGoEFTags_Complex(b *testing.B) {
	tag := "required,unique,max_length:255,min:1,index:idx_email,foreign_key:user_id"

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		metadata.ParseGoEFTags(tag)
	}
}

func BenchmarkToSnakeCase(b *testing.B) {
	testCases := []string{
		"SimpleCase",
		"ComplexCamelCase",
		"VeryLongCamelCaseStringWithManyWords",
		"HTTPSConnection",
		"XMLParser",
		"APIResponse",
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, testCase := range testCases {
			metadata.ToSnakeCase(testCase)
		}
	}
}

func BenchmarkMetadataRegistry_Memory(b *testing.B) {
	registry := metadata.NewRegistry()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		entity := &TestEntityForBench{}
		registry.GetOrCreate(entity)
	}
}
