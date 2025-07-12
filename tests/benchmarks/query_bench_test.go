package benchmarks

import (
	"fmt"
	"testing"

	"github.com/goef-team/goef/dialects"
	"github.com/goef-team/goef/metadata"
	"github.com/goef-team/goef/query"
)

func BenchmarkQueryCompiler_Compile(b *testing.B) {
	dialect := dialects.NewPostgreSQLDialect(nil)
	metadataRegistry := metadata.NewRegistry()
	compiler := query.NewQueryCompiler(dialect, metadataRegistry, 1000)

	testQuery := &dialects.SelectQuery{
		Table:   "users",
		Columns: []string{"id", "name", "email"},
		Where: []dialects.WhereClause{
			{Column: "age", Operator: ">", Value: 18, Logic: "AND"},
		},
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := compiler.Compile(&TestEntityForBench{}, testQuery)
		if err != nil {
			b.Fatalf("Failed to compile query: %v", err)
		}
	}
}

func BenchmarkQueryCompiler_CompileWithCache(b *testing.B) {
	dialect := dialects.NewPostgreSQLDialect(nil)
	metadataRegistry := metadata.NewRegistry()
	compiler := query.NewQueryCompiler(dialect, metadataRegistry, 1000)

	testQuery := &dialects.SelectQuery{
		Table:   "users",
		Columns: []string{"id", "name", "email"},
		Where: []dialects.WhereClause{
			{Column: "age", Operator: ">", Value: 18, Logic: "AND"},
		},
	}

	// Warm up cache
	_, err := compiler.Compile(&TestEntityForBench{}, testQuery)
	if err != nil {
		b.Fatalf("error on compile, err: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := compiler.Compile(&TestEntityForBench{}, testQuery)
		if err != nil {
			b.Fatalf("Failed to compile query: %v", err)
		}
	}
}

func BenchmarkQueryCompiler_CompileInsert(b *testing.B) {
	dialect := dialects.NewPostgreSQLDialect(nil)
	metadataRegistry := metadata.NewRegistry()
	compiler := query.NewQueryCompiler(dialect, metadataRegistry, 1000)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := compiler.CompileInsert(&TestEntityForBench{})
		if err != nil {
			b.Fatalf("Failed to compile insert: %v", err)
		}
	}
}

func BenchmarkQueryCompiler_CompileUpdate(b *testing.B) {
	dialect := dialects.NewPostgreSQLDialect(nil)
	metadataRegistry := metadata.NewRegistry()
	compiler := query.NewQueryCompiler(dialect, metadataRegistry, 1000)

	modifiedFields := []string{"Name", "Email", "Age"}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := compiler.CompileUpdate(&TestEntityForBench{}, modifiedFields)
		if err != nil {
			b.Fatalf("Failed to compile update: %v", err)
		}
	}
}

func BenchmarkQueryCompiler_CompileDelete(b *testing.B) {
	dialect := dialects.NewPostgreSQLDialect(nil)
	metadataRegistry := metadata.NewRegistry()
	compiler := query.NewQueryCompiler(dialect, metadataRegistry, 1000)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := compiler.CompileDelete(&TestEntityForBench{})
		if err != nil {
			b.Fatalf("Failed to compile delete: %v", err)
		}
	}
}

func BenchmarkQueryCompiler_CachePerformance(b *testing.B) {
	cacheSizes := []int{10, 100, 1000, 10000}

	for _, cacheSize := range cacheSizes {
		b.Run(fmt.Sprintf("CacheSize_%d", cacheSize), func(b *testing.B) {
			dialect := dialects.NewPostgreSQLDialect(nil)
			metadataRegistry := metadata.NewRegistry()
			compiler := query.NewQueryCompiler(dialect, metadataRegistry, cacheSize)

			// Create different queries to fill cache
			for i := 0; i < cacheSize; i++ {
				testQuery := &dialects.SelectQuery{
					Table:   "users",
					Columns: []string{"id", "name", "email"},
					Where: []dialects.WhereClause{
						{Column: "age", Operator: ">", Value: i, Logic: "AND"},
					},
				}
				_, err := compiler.Compile(&TestEntityForBench{}, testQuery)
				if err != nil {
					b.Fatalf("error on compile, err: %v", err)
				}
			}

			// Benchmark cache hits
			testQuery := &dialects.SelectQuery{
				Table:   "users",
				Columns: []string{"id", "name", "email"},
				Where: []dialects.WhereClause{
					{Column: "age", Operator: ">", Value: 1, Logic: "AND"},
				},
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, err := compiler.Compile(&TestEntityForBench{}, testQuery)
				if err != nil {
					b.Fatalf("error on compile, err: %v", err)
				}
			}
		})
	}
}

func BenchmarkBatchCompiler_Performance(b *testing.B) {
	dialect := dialects.NewPostgreSQLDialect(nil)
	metadataRegistry := metadata.NewRegistry()
	compiler := query.NewQueryCompiler(dialect, metadataRegistry, 1000)
	batchCompiler := query.NewBatchCompiler(compiler, 10)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		testQuery := &dialects.SelectQuery{
			Table:   "users",
			Columns: []string{"id", "name", "email"},
			Where: []dialects.WhereClause{
				{Column: "id", Operator: "=", Value: i, Logic: "AND"},
			},
		}

		request := query.QueryCompilationRequest{
			EntityType: &TestEntityForBench{},
			Query:      testQuery,
			Callback:   func(*query.CompiledQuery, error) {},
		}

		batchCompiler.AddToBatch(request)
	}

	batchCompiler.ProcessBatch()
}
