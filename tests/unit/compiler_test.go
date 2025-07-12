package unit

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/goef-team/goef/dialects"
	"github.com/goef-team/goef/metadata"
	"github.com/goef-team/goef/query"
)

func TestQueryCompiler_Compile(t *testing.T) {
	dialect := dialects.NewPostgreSQLDialect(nil)
	metadataRegistry := metadata.NewRegistry()

	compiler := query.NewQueryCompiler(dialect, metadataRegistry, 100)

	// Create a test query
	queryObj := &dialects.SelectQuery{
		Table:   "users",
		Columns: []string{"id", "name", "email"},
		Where: []dialects.WhereClause{
			{Column: "age", Operator: ">", Value: 18, Logic: "AND"},
		},
	}

	// Compile the query
	compiled, err := compiler.Compile(&testEntity{}, queryObj)
	if err != nil {
		t.Fatalf("Failed to compile query: %v", err)
	}

	if compiled.SQL == "" {
		t.Error("Expected non-empty SQL")
	}

	if compiled.ParamCount != 1 {
		t.Errorf("Expected 1 parameter, got %d", compiled.ParamCount)
	}

	if compiled.AccessCount != 1 {
		t.Errorf("Expected access count 1, got %d", compiled.AccessCount)
	}
}

func TestQueryCompiler_CacheHit(t *testing.T) {
	dialect := dialects.NewPostgreSQLDialect(nil)
	metadataRegistry := metadata.NewRegistry()

	compiler := query.NewQueryCompiler(dialect, metadataRegistry, 100)

	// Create a test query
	queryObj := &dialects.SelectQuery{
		Table:   "users",
		Columns: []string{"id", "name"},
		Where: []dialects.WhereClause{
			{Column: "active", Operator: "=", Value: true, Logic: "AND"},
		},
	}

	// Compile the query first time
	compiled1, err := compiler.Compile(&testEntity{}, queryObj)
	if err != nil {
		t.Fatalf("Failed to compile query first time: %v", err)
	}

	// Compile the same query again
	compiled2, err := compiler.Compile(&testEntity{}, queryObj)
	if err != nil {
		t.Fatalf("Failed to compile query second time: %v", err)
	}

	// Should be the same instance (cache hit)
	if compiled1 != compiled2 {
		t.Error("Expected same compiled query instance from cache")
	}

	if compiled2.AccessCount != 2 {
		t.Errorf("Expected access count 2, got %d", compiled2.AccessCount)
	}
}

func TestQueryCompiler_CompileInsert(t *testing.T) {
	dialect := dialects.NewPostgreSQLDialect(nil)
	metadataRegistry := metadata.NewRegistry()

	compiler := query.NewQueryCompiler(dialect, metadataRegistry, 100)

	// Compile insert query
	compiled, err := compiler.CompileInsert(&testEntity{})
	if err != nil {
		t.Fatalf("Failed to compile insert query: %v", err)
	}

	if compiled.SQL == "" {
		t.Error("Expected non-empty SQL")
	}

	// Should contain INSERT
	if !strings.Contains(compiled.SQL, "INSERT") {
		t.Error("Expected SQL to contain INSERT")
	}
}

func TestQueryCompiler_CompileUpdate(t *testing.T) {
	dialect := dialects.NewPostgreSQLDialect(nil)
	metadataRegistry := metadata.NewRegistry()

	compiler := query.NewQueryCompiler(dialect, metadataRegistry, 100)

	// Compile update query
	modifiedFields := []string{"Name", "Email"}
	compiled, err := compiler.CompileUpdate(&testEntity{}, modifiedFields)
	if err != nil {
		t.Fatalf("Failed to compile update query: %v", err)
	}

	if compiled.SQL == "" {
		t.Error("Expected non-empty SQL")
	}

	// Should contain UPDATE
	if !strings.Contains(compiled.SQL, "UPDATE") {
		t.Error("Expected SQL to contain UPDATE")
	}
}

func TestQueryCompiler_CompileDelete(t *testing.T) {
	dialect := dialects.NewPostgreSQLDialect(nil)
	metadataRegistry := metadata.NewRegistry()

	compiler := query.NewQueryCompiler(dialect, metadataRegistry, 100)

	// Compile delete query
	compiled, err := compiler.CompileDelete(&testEntity{})
	if err != nil {
		t.Fatalf("Failed to compile delete query: %v", err)
	}

	if compiled.SQL == "" {
		t.Error("Expected non-empty SQL")
	}

	// Should contain UPDATE (for soft delete) or DELETE
	if !strings.Contains(compiled.SQL, "UPDATE") && !strings.Contains(compiled.SQL, "DELETE") {
		t.Error("Expected SQL to contain UPDATE or DELETE")
	}
}

func TestQueryCompiler_CacheEviction(t *testing.T) {
	dialect := dialects.NewPostgreSQLDialect(nil)
	metadataRegistry := metadata.NewRegistry()

	// Create compiler with small cache size
	compiler := query.NewQueryCompiler(dialect, metadataRegistry, 2)

	// Compile more queries than cache size
	for i := 0; i < 5; i++ {
		queryObj := &dialects.SelectQuery{
			Table:   "users",
			Columns: []string{"id"},
			Where: []dialects.WhereClause{
				{Column: "id", Operator: "=", Value: i, Logic: "AND"},
			},
		}

		_, err := compiler.Compile(&testEntity{}, queryObj)
		if err != nil {
			t.Fatalf("Failed to compile query %d: %v", i, err)
		}
	}

	// Check cache stats
	stats := compiler.GetCacheStats()
	cacheSize := stats["cache_size"].(int)

	if cacheSize > 2 {
		t.Errorf("Expected cache size to be <= 2, got %d", cacheSize)
	}
}

func TestQueryCompiler_GetCacheStats(t *testing.T) {
	dialect := dialects.NewPostgreSQLDialect(nil)
	metadataRegistry := metadata.NewRegistry()

	compiler := query.NewQueryCompiler(dialect, metadataRegistry, 100)

	// Initially empty cache
	stats := compiler.GetCacheStats()
	if stats["cache_size"].(int) != 0 {
		t.Error("Expected empty cache initially")
	}

	// Compile a query
	queryObj := &dialects.SelectQuery{
		Table:   "users",
		Columns: []string{"id"},
	}

	_, err := compiler.Compile(&testEntity{}, queryObj)
	if err != nil {
		t.Fatalf("Failed to compile query: %v", err)
	}

	// Check stats after compilation
	stats = compiler.GetCacheStats()
	if stats["cache_size"].(int) != 1 {
		t.Error("Expected cache size of 1 after compilation")
	}

	if stats["max_size"].(int) != 100 {
		t.Error("Expected max size of 100")
	}
}

func TestBatchCompiler_AddToBatch(t *testing.T) {
	dialect := dialects.NewPostgreSQLDialect(nil)
	metadataRegistry := metadata.NewRegistry()
	compiler := query.NewQueryCompiler(dialect, metadataRegistry, 100)

	batchCompiler := query.NewBatchCompiler(compiler, 2)

	var compiledQueries []*query.CompiledQuery
	var compilationErrors []error
	var mu sync.Mutex     // ✅ CRITICAL: Mutex to protect shared slices
	var wg sync.WaitGroup // ✅ CRITICAL: WaitGroup to wait for completion

	// Create callback to collect results with proper synchronization
	callback := func(compiled *query.CompiledQuery, err error) {
		defer wg.Done() // Signal completion of this callback

		mu.Lock() // ✅ CRITICAL: Lock before accessing shared slices
		defer mu.Unlock()

		if err != nil {
			compilationErrors = append(compilationErrors, err)
		} else {
			compiledQueries = append(compiledQueries, compiled)
		}
	}

	// Add queries to batch
	for i := 0; i < 3; i++ {
		queryObj := &dialects.SelectQuery{
			Table:   "users",
			Columns: []string{"id"},
			Where: []dialects.WhereClause{
				{Column: "id", Operator: "=", Value: i, Logic: "AND"},
			},
		}

		wg.Add(1) // ✅ CRITICAL: Expect one callback per request

		request := query.QueryCompilationRequest{
			EntityType: &testEntity{},
			Query:      queryObj,
			QueryType:  "select", // ✅ Fixed: Added missing QueryType
			Callback:   callback,
		}

		batchCompiler.AddToBatch(request)
	}

	// Process any remaining queries
	batchCompiler.ProcessBatch() // or ProcessBatchSync() if you have it

	// ✅ CRITICAL: Wait for all processing to complete with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All processing completed
	case <-time.After(2 * time.Second):
		t.Fatal("Test timeout: batch processing did not complete within 2 seconds")
	}

	// ✅ CRITICAL: Lock before checking results
	mu.Lock()
	defer mu.Unlock()

	// Check results
	if len(compilationErrors) > 0 {
		t.Errorf("Expected no compilation errors, got %d", len(compilationErrors))
		for i, err := range compilationErrors {
			t.Errorf("Error %d: %v", i, err)
		}
	}

	if len(compiledQueries) != 3 {
		t.Errorf("Expected 3 compiled queries, got %d", len(compiledQueries))
	}

	// Verify that each compiled query has valid SQL
	for i, compiled := range compiledQueries {
		if compiled.SQL == "" {
			t.Errorf("Compiled query %d has empty SQL", i)
		}
	}
}
