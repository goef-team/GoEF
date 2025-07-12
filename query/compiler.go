package query

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/goef-team/goef/dialects"
	"github.com/goef-team/goef/metadata"
)

// CompiledQuery represents a compiled query with cached SQL and metadata
type CompiledQuery struct {
	SQL           string
	ParamCount    int
	Metadata      *metadata.EntityMetadata
	CompiledAt    time.Time
	AccessCount   int64
	LastAccess    time.Time
	OptimizedSQL  string                 // SQL with applied optimizations
	Hints         *QueryOptimizationHint // Applied optimization hints
	CacheExpiry   *time.Time             // When this cache entry expires
	EstimatedCost float64                // Estimated query execution cost
}

// QueryCompiler compiles and caches queries for performance
type QueryCompiler struct {
	dialect  dialects.Dialect
	metadata *metadata.Registry
	cache    map[string]*CompiledQuery
	mu       sync.RWMutex
	maxSize  int
	// Performance tracking
	hitCount    int64
	missCount   int64
	evictCount  int64
	compileTime time.Duration
}

// NewQueryCompiler creates a new query compiler
func NewQueryCompiler(dialect dialects.Dialect, metadata *metadata.Registry, maxCacheSize int) *QueryCompiler {
	if maxCacheSize <= 0 {
		maxCacheSize = 1000
	}

	return &QueryCompiler{
		dialect:  dialect,
		metadata: metadata,
		cache:    make(map[string]*CompiledQuery),
		maxSize:  maxCacheSize,
	}
}

// Compile compiles a query and caches the result
func (qc *QueryCompiler) Compile(entityType interface{}, query *dialects.SelectQuery) (*CompiledQuery, error) {
	startTime := time.Now()
	defer func() {
		qc.mu.Lock()
		qc.compileTime += time.Since(startTime)
		qc.mu.Unlock()
	}()

	// Generate cache key
	cacheKey := qc.generateCacheKey(entityType, query)

	// Check cache first
	qc.mu.RLock()
	if compiled, exists := qc.cache[cacheKey]; exists {
		if compiled.CacheExpiry == nil || time.Now().Before(*compiled.CacheExpiry) {
			qc.mu.RUnlock()

			qc.mu.Lock()
			compiled.AccessCount++
			compiled.LastAccess = time.Now()
			qc.hitCount++
			qc.mu.Unlock()

			return compiled, nil
		}
		qc.mu.RUnlock()
		qc.mu.Lock()
		delete(qc.cache, cacheKey)
		qc.mu.Unlock()
	} else {
		qc.mu.RUnlock()
	}

	qc.mu.Lock()
	qc.missCount++
	qc.mu.Unlock()

	// Compile the query
	meta := qc.metadata.GetOrCreate(entityType)
	sql, args, err := qc.dialect.BuildSelect(meta.TableName, query)
	if err != nil {
		return nil, fmt.Errorf("failed to compile query: %w", err)
	}

	compiled := &CompiledQuery{
		SQL:           sql,
		ParamCount:    len(args),
		Metadata:      meta,
		CompiledAt:    time.Now(),
		AccessCount:   1,
		LastAccess:    time.Now(),
		OptimizedSQL:  sql, // Initially same as SQL
		EstimatedCost: qc.estimateQueryCost(query, meta),
	}

	// Cache the compiled query
	qc.mu.Lock()
	defer qc.mu.Unlock()

	// Check cache size and evict if necessary
	if len(qc.cache) >= qc.maxSize {
		qc.evictLeastRecentlyUsed()
	}

	qc.cache[cacheKey] = compiled

	return compiled, nil
}

// CompileInsert compiles an INSERT query
func (qc *QueryCompiler) CompileInsert(entityType interface{}) (*CompiledQuery, error) {
	startTime := time.Now()
	defer func() {
		qc.mu.Lock()
		qc.compileTime += time.Since(startTime)
		qc.mu.Unlock()
	}()

	cacheKey := qc.generateInsertCacheKey(entityType)

	// Check cache
	qc.mu.RLock()
	if compiled, exists := qc.cache[cacheKey]; exists {
		if compiled.CacheExpiry == nil || time.Now().Before(*compiled.CacheExpiry) {
			qc.mu.RUnlock()

			// ✅ CRITICAL FIX: Upgrade to write lock before modifying
			qc.mu.Lock()
			compiled.AccessCount++
			compiled.LastAccess = time.Now()
			qc.hitCount++
			qc.mu.Unlock()

			return compiled, nil
		}
		qc.mu.RUnlock()
		qc.mu.Lock()
		delete(qc.cache, cacheKey)
		qc.mu.Unlock()
	} else {
		qc.mu.RUnlock()
	}

	qc.mu.Lock()
	qc.missCount++
	qc.mu.Unlock()

	// Compile the insert query
	meta := qc.metadata.GetOrCreate(entityType)

	// Build field list and dummy values for non-auto-increment fields
	var fields []string
	var values []interface{}

	for _, field := range meta.Fields {
		if !field.IsAutoIncrement {
			fields = append(fields, field.ColumnName)
			values = append(values, nil) // Dummy value for compilation
		}
	}

	sql, args, err := qc.dialect.BuildInsert(meta.TableName, fields, values)
	if err != nil {
		return nil, fmt.Errorf("failed to compile insert query: %w", err)
	}

	compiled := &CompiledQuery{
		SQL:           sql,
		ParamCount:    len(args),
		Metadata:      meta,
		CompiledAt:    time.Now(),
		AccessCount:   1,
		LastAccess:    time.Now(),
		OptimizedSQL:  sql,
		EstimatedCost: 1.0, // INSERT operations have relatively fixed cost
	}

	// Cache the compiled query
	qc.mu.Lock()
	defer qc.mu.Unlock()

	if len(qc.cache) >= qc.maxSize {
		qc.evictLeastRecentlyUsed()
	}

	qc.cache[cacheKey] = compiled

	return compiled, nil
}

// CompileUpdate compiles an UPDATE query
func (qc *QueryCompiler) CompileUpdate(entityType interface{}, modifiedFields []string) (*CompiledQuery, error) {
	startTime := time.Now()
	defer func() {
		qc.mu.Lock()
		qc.compileTime += time.Since(startTime)
		qc.mu.Unlock()
	}()

	cacheKey := qc.generateUpdateCacheKey(entityType, modifiedFields)

	// Check cache
	qc.mu.RLock()
	if compiled, exists := qc.cache[cacheKey]; exists {
		if compiled.CacheExpiry == nil || time.Now().Before(*compiled.CacheExpiry) {
			qc.mu.RUnlock()

			// ✅ CRITICAL FIX: Upgrade to write lock before modifying
			qc.mu.Lock()
			compiled.AccessCount++
			compiled.LastAccess = time.Now()
			qc.hitCount++
			qc.mu.Unlock()

			return compiled, nil
		}
		qc.mu.RUnlock()
		qc.mu.Lock()
		delete(qc.cache, cacheKey)
		qc.mu.Unlock()
	} else {
		qc.mu.RUnlock()
	}

	qc.mu.Lock()
	qc.missCount++
	qc.mu.Unlock()

	// Compile the update query
	meta := qc.metadata.GetOrCreate(entityType)

	var fields []string
	var values []interface{}

	// Sort fields for consistent cache keys
	sortedFields := make([]string, len(modifiedFields))
	copy(sortedFields, modifiedFields)
	sort.Strings(sortedFields)

	for _, fieldName := range sortedFields {
		if field := meta.FieldsByName[fieldName]; field != nil && !field.IsPrimaryKey && !field.IsAutoIncrement {
			fields = append(fields, field.ColumnName)
			values = append(values, nil) // Dummy value for compilation
		}
	}

	// Build WHERE clause for primary key
	whereClause := qc.buildPrimaryKeyWhereClause(meta)
	whereArgs := qc.buildPrimaryKeyWhereArgs(meta)

	sql, args, err := qc.dialect.BuildUpdate(meta.TableName, fields, values, whereClause, whereArgs)
	if err != nil {
		return nil, fmt.Errorf("failed to compile update query: %w", err)
	}

	compiled := &CompiledQuery{
		SQL:           sql,
		ParamCount:    len(args),
		Metadata:      meta,
		CompiledAt:    time.Now(),
		AccessCount:   1,
		LastAccess:    time.Now(),
		OptimizedSQL:  sql,
		EstimatedCost: qc.estimateUpdateCost(len(fields)),
	}

	// Cache the compiled query
	qc.mu.Lock()
	defer qc.mu.Unlock()

	if len(qc.cache) >= qc.maxSize {
		qc.evictLeastRecentlyUsed()
	}

	qc.cache[cacheKey] = compiled

	return compiled, nil
}

// CompileDelete compiles a DELETE query
func (qc *QueryCompiler) CompileDelete(entityType interface{}) (*CompiledQuery, error) {
	startTime := time.Now()
	defer func() {
		qc.mu.Lock()
		qc.compileTime += time.Since(startTime)
		qc.mu.Unlock()
	}()

	cacheKey := qc.generateDeleteCacheKey(entityType)

	qc.mu.RLock()
	if compiled, exists := qc.cache[cacheKey]; exists {
		if compiled.CacheExpiry == nil || time.Now().Before(*compiled.CacheExpiry) {
			qc.mu.RUnlock()

			qc.mu.Lock()
			compiled.AccessCount++
			compiled.LastAccess = time.Now()
			qc.hitCount++
			qc.mu.Unlock()

			return compiled, nil
		}
		qc.mu.RUnlock()
		qc.mu.Lock()
		delete(qc.cache, cacheKey)
		qc.mu.Unlock()
	} else {
		qc.mu.RUnlock()
	}

	qc.mu.Lock()
	qc.missCount++
	qc.mu.Unlock()

	// Compile the delete query
	meta := qc.metadata.GetOrCreate(entityType)

	// Check if soft delete is enabled (has deleted_at field)
	var sql string
	var args []interface{}
	var err error

	if qc.hasSoftDeleteField(meta) {
		// Use soft delete (UPDATE SET deleted_at = NOW())
		fields := []string{"deleted_at"}
		values := []interface{}{nil} // Will be replaced with current timestamp
		whereClause := qc.buildPrimaryKeyWhereClause(meta)
		whereArgs := qc.buildPrimaryKeyWhereArgs(meta)

		sql, args, err = qc.dialect.BuildUpdate(meta.TableName, fields, values, whereClause, whereArgs)
	} else {
		// Use hard delete
		whereClause := qc.buildPrimaryKeyWhereClause(meta)
		whereArgs := qc.buildPrimaryKeyWhereArgs(meta)

		sql, args, err = qc.dialect.BuildDelete(meta.TableName, whereClause, whereArgs)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to compile delete query: %w", err)
	}

	compiled := &CompiledQuery{
		SQL:           sql,
		ParamCount:    len(args),
		Metadata:      meta,
		CompiledAt:    time.Now(),
		AccessCount:   1,
		LastAccess:    time.Now(),
		OptimizedSQL:  sql,
		EstimatedCost: 1.0, // DELETE operations have relatively fixed cost
	}

	// Cache the compiled query
	qc.mu.Lock()
	defer qc.mu.Unlock()

	if len(qc.cache) >= qc.maxSize {
		qc.evictLeastRecentlyUsed()
	}

	qc.cache[cacheKey] = compiled

	return compiled, nil
}

// ClearCache clears the query cache
func (qc *QueryCompiler) ClearCache() {
	qc.mu.Lock()
	defer qc.mu.Unlock()

	qc.cache = make(map[string]*CompiledQuery)
	qc.hitCount = 0
	qc.missCount = 0
	qc.evictCount = 0
	qc.compileTime = 0
}

// GetCacheStats returns comprehensive cache statistics
func (qc *QueryCompiler) GetCacheStats() map[string]interface{} {
	qc.mu.RLock()
	defer qc.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["cache_size"] = len(qc.cache)
	stats["max_size"] = qc.maxSize
	stats["hit_count"] = qc.hitCount
	stats["miss_count"] = qc.missCount
	stats["evict_count"] = qc.evictCount
	stats["total_compile_time_ms"] = qc.compileTime.Milliseconds()

	// Calculate hit rate
	totalRequests := qc.hitCount + qc.missCount
	if totalRequests > 0 {
		stats["hit_rate"] = float64(qc.hitCount) / float64(totalRequests)
	} else {
		stats["hit_rate"] = 0.0
	}

	var totalAccess int64
	var totalCost float64
	oldestAccess := time.Now()
	newestAccess := time.Time{}

	for _, compiled := range qc.cache {
		totalAccess += compiled.AccessCount
		totalCost += compiled.EstimatedCost
		if compiled.LastAccess.Before(oldestAccess) {
			oldestAccess = compiled.LastAccess
		}
		if compiled.LastAccess.After(newestAccess) {
			newestAccess = compiled.LastAccess
		}
	}

	stats["total_access"] = totalAccess
	stats["total_estimated_cost"] = totalCost
	if len(qc.cache) > 0 {
		stats["average_access"] = totalAccess / int64(len(qc.cache))
		stats["average_estimated_cost"] = totalCost / float64(len(qc.cache))
		stats["oldest_access"] = oldestAccess
		stats["newest_access"] = newestAccess
	}

	return stats
}

// generateCacheKey generates a cache key for a select query using SHA-256
func (qc *QueryCompiler) generateCacheKey(entityType interface{}, query *dialects.SelectQuery) string {
	meta := qc.metadata.GetOrCreate(entityType)

	// Create a hash of the query components using SHA-256
	hash := sha256.New()
	hash.Write([]byte(meta.Type.Name()))
	hash.Write([]byte(query.Table))

	// Sort columns for consistent cache keys
	sortedColumns := make([]string, len(query.Columns))
	copy(sortedColumns, query.Columns)
	sort.Strings(sortedColumns)

	for _, col := range sortedColumns {
		hash.Write([]byte(col))
	}

	// Sort WHERE clauses for consistent cache keys
	sortedWhere := make([]dialects.WhereClause, len(query.Where))
	copy(sortedWhere, query.Where)
	sort.Slice(sortedWhere, func(i, j int) bool {
		return sortedWhere[i].Column < sortedWhere[j].Column
	})

	for _, where := range sortedWhere {
		hash.Write([]byte(where.Column))
		hash.Write([]byte(where.Operator))
		hash.Write([]byte(where.Logic))
		// Don't include actual values in cache key to allow parameter reuse
	}

	// Sort ORDER BY clauses for consistent cache keys
	sortedOrderBy := make([]dialects.OrderByClause, len(query.OrderBy))
	copy(sortedOrderBy, query.OrderBy)
	sort.Slice(sortedOrderBy, func(i, j int) bool {
		return sortedOrderBy[i].Column < sortedOrderBy[j].Column
	})

	for _, order := range sortedOrderBy {
		hash.Write([]byte(order.Column))
		hash.Write([]byte(order.Direction))
	}

	if query.Limit != nil {
		_, err := fmt.Fprintf(hash, "limit:%d", *query.Limit)
		if err != nil {
			return err.Error()
		}
	}

	if query.Offset != nil {
		_, err := fmt.Fprintf(hash, "offset:%d", *query.Offset)
		if err != nil {
			return err.Error()
		}
	}

	// Include JOIN information if present
	if query.Joins != nil {
		for _, join := range query.Joins {
			hash.Write([]byte(join.Type))
			hash.Write([]byte(join.Table))
			hash.Write([]byte(join.Condition))
		}
	}

	return fmt.Sprintf("select:%x", hash.Sum(nil))
}

// generateInsertCacheKey generates a cache key for an insert query
func (qc *QueryCompiler) generateInsertCacheKey(entityType interface{}) string {
	meta := qc.metadata.GetOrCreate(entityType)
	return fmt.Sprintf("insert:%s:%s", meta.Type.Name(), meta.TableName)
}

// generateUpdateCacheKey generates a cache key for an update query using SHA-256
func (qc *QueryCompiler) generateUpdateCacheKey(entityType interface{}, modifiedFields []string) string {
	meta := qc.metadata.GetOrCreate(entityType)

	// Sort fields for consistent cache keys
	sortedFields := make([]string, len(modifiedFields))
	copy(sortedFields, modifiedFields)
	sort.Strings(sortedFields)

	// Use SHA-256 for secure hashing
	hash := sha256.New()
	hash.Write([]byte(meta.Type.Name()))
	hash.Write([]byte(meta.TableName))
	for _, field := range sortedFields {
		hash.Write([]byte(field))
	}

	return fmt.Sprintf("update:%x", hash.Sum(nil))
}

// generateDeleteCacheKey generates a cache key for a delete query
func (qc *QueryCompiler) generateDeleteCacheKey(entityType interface{}) string {
	meta := qc.metadata.GetOrCreate(entityType)
	deleteType := "hard"
	if qc.hasSoftDeleteField(meta) {
		deleteType = "soft"
	}
	return fmt.Sprintf("delete:%s:%s:%s", deleteType, meta.Type.Name(), meta.TableName)
}

// evictLeastRecentlyUsed removes the least recently used queries from cache
func (qc *QueryCompiler) evictLeastRecentlyUsed() {
	if len(qc.cache) == 0 {
		return
	}

	// Find the oldest accessed query
	var oldestKey string
	var oldestTime time.Time
	var lowestAccessCount int64 = -1

	for key, compiled := range qc.cache {
		// Prioritize by access count first, then by last access time
		if lowestAccessCount == -1 || compiled.AccessCount < lowestAccessCount ||
			(compiled.AccessCount == lowestAccessCount && compiled.LastAccess.Before(oldestTime)) {
			oldestKey = key
			oldestTime = compiled.LastAccess
			lowestAccessCount = compiled.AccessCount
		}
	}

	if oldestKey != "" {
		delete(qc.cache, oldestKey)
		qc.evictCount++
	}
}

// estimateQueryCost estimates the execution cost of a query
func (qc *QueryCompiler) estimateQueryCost(query *dialects.SelectQuery, meta *metadata.EntityMetadata) float64 {
	cost := 1.0 // Base cost

	// Add cost for WHERE clauses
	cost += float64(len(query.Where)) * 0.5

	// Add cost for JOINs
	if query.Joins != nil {
		cost += float64(len(query.Joins)) * 2.0
	}

	// Add cost for ORDER BY
	cost += float64(len(query.OrderBy)) * 0.3

	// Add cost for aggregations
	if query.GroupBy != nil {
		cost += float64(len(query.GroupBy)) * 1.5
	}

	// Reduce cost if primary key is in WHERE clause
	for _, where := range query.Where {
		for _, field := range meta.Fields {
			if field.IsPrimaryKey && field.ColumnName == where.Column && where.Operator == "=" {
				cost *= 0.1 // Primary key lookup is very fast
				break
			}
		}
	}

	return cost
}

// estimateUpdateCost estimates the execution cost of an update query
func (qc *QueryCompiler) estimateUpdateCost(fieldCount int) float64 {
	return 1.0 + float64(fieldCount)*0.1
}

// buildPrimaryKeyWhereClause builds a WHERE clause for primary key fields
func (qc *QueryCompiler) buildPrimaryKeyWhereClause(meta *metadata.EntityMetadata) string {
	var conditions []string
	placeholderIndex := 1

	for _, field := range meta.Fields {
		if field.IsPrimaryKey {
			condition := fmt.Sprintf("%s = %s",
				qc.dialect.Quote(field.ColumnName),
				qc.dialect.Placeholder(placeholderIndex))
			conditions = append(conditions, condition)
			placeholderIndex++
		}
	}

	if len(conditions) == 0 {
		// Fallback to ID field if no explicit primary key
		return fmt.Sprintf("%s = %s", qc.dialect.Quote("id"), qc.dialect.Placeholder(1))
	}

	return strings.Join(conditions, " AND ")
}

// buildPrimaryKeyWhereArgs builds arguments for primary key WHERE clause
func (qc *QueryCompiler) buildPrimaryKeyWhereArgs(meta *metadata.EntityMetadata) []interface{} {
	var args []interface{}

	for _, field := range meta.Fields {
		if field.IsPrimaryKey {
			args = append(args, nil) // Dummy value for compilation
		}
	}

	if len(args) == 0 {
		// Fallback to single ID argument
		args = append(args, nil)
	}

	return args
}

// hasSoftDeleteField checks if the entity has a soft delete field
func (qc *QueryCompiler) hasSoftDeleteField(meta *metadata.EntityMetadata) bool {
	for _, field := range meta.Fields {
		if field.ColumnName == "deleted_at" || field.Name == "DeletedAt" {
			return true
		}
	}
	return false
}

// BatchCompiler compiles multiple queries in a batch for better performance
type BatchCompiler struct {
	*QueryCompiler
	batchSize  int
	pending    []QueryCompilationRequest
	mu         sync.Mutex
	processing bool
}

// QueryCompilationRequest represents a request to compile a query in a batch
type QueryCompilationRequest struct {
	ID         string // Unique request ID
	EntityType interface{}
	Query      *dialects.SelectQuery
	QueryType  string   // "select", "insert", "update", "delete"
	Fields     []string // For update queries
	Callback   func(*CompiledQuery, error)
	Priority   int       // Higher numbers = higher priority
	Submitted  time.Time // When the request was submitted
}

// NewBatchCompiler creates a new batch compiler
func NewBatchCompiler(queryCompiler *QueryCompiler, batchSize int) *BatchCompiler {
	if batchSize <= 0 {
		batchSize = 10
	}

	return &BatchCompiler{
		QueryCompiler: queryCompiler,
		batchSize:     batchSize,
		pending:       make([]QueryCompilationRequest, 0, batchSize),
	}
}

// AddToBatch adds a query compilation request to the batch
func (bc *BatchCompiler) AddToBatch(request QueryCompilationRequest) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if request.Submitted.IsZero() {
		request.Submitted = time.Now()
	}

	bc.pending = append(bc.pending, request)

	// Sort by priority (higher priority first)
	sort.Slice(bc.pending, func(i, j int) bool {
		return bc.pending[i].Priority > bc.pending[j].Priority
	})

	// Process batch if it's full or if we have high-priority requests
	if len(bc.pending) >= bc.batchSize || bc.hasHighPriorityRequests() {
		bc.processBatchUnsafe()
	}
}

// ProcessBatch processes any remaining queries in the batch
func (bc *BatchCompiler) ProcessBatch() {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if len(bc.pending) > 0 && !bc.processing {
		bc.processBatchUnsafe()
	}
}

// hasHighPriorityRequests checks if there are high-priority requests that should trigger immediate processing
func (bc *BatchCompiler) hasHighPriorityRequests() bool {
	for _, req := range bc.pending {
		if req.Priority > 5 { // Priority > 5 triggers immediate processing
			return true
		}
	}
	return false
}

// processBatchUnsafe processes the current batch (must be called with lock held)
func (bc *BatchCompiler) processBatchUnsafe() {
	if bc.processing {
		return
	}

	requests := make([]QueryCompilationRequest, len(bc.pending))
	copy(requests, bc.pending)
	bc.pending = bc.pending[:0] // Clear slice but keep capacity
	bc.processing = true

	// Process requests concurrently
	go func() {
		defer func() {
			bc.mu.Lock()
			bc.processing = false
			pending := len(bc.pending) > 0
			bc.mu.Unlock()
			if pending {
				bc.ProcessBatch()
			}
		}()

		// Use worker pool for parallel processing
		workerCount := 4
		if len(requests) < workerCount {
			workerCount = len(requests)
		}

		requestChan := make(chan QueryCompilationRequest, len(requests))
		var wg sync.WaitGroup

		// Start workers
		for i := 0; i < workerCount; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for request := range requestChan {
					bc.processRequest(request)
				}
			}()
		}

		// Send requests to workers
		for _, request := range requests {
			requestChan <- request
		}
		close(requestChan)

		wg.Wait()
	}()
}

// processRequest processes a single compilation request
func (bc *BatchCompiler) processRequest(request QueryCompilationRequest) {
	var compiled *CompiledQuery
	var err error

	switch request.QueryType {
	case "select":
		compiled, err = bc.Compile(request.EntityType, request.Query)
	case "insert":
		compiled, err = bc.CompileInsert(request.EntityType)
	case "update":
		compiled, err = bc.CompileUpdate(request.EntityType, request.Fields)
	case "delete":
		compiled, err = bc.CompileDelete(request.EntityType)
	default:
		err = fmt.Errorf("unknown query type: %s", request.QueryType)
	}

	if request.Callback != nil {
		request.Callback(compiled, err)
	}
}

// GetBatchStats returns statistics about batch processing
func (bc *BatchCompiler) GetBatchStats() map[string]interface{} {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	stats := make(map[string]interface{})
	stats["batch_size"] = bc.batchSize
	stats["pending_requests"] = len(bc.pending)
	stats["processing"] = bc.processing

	// Analyze pending requests
	if len(bc.pending) > 0 {
		priorities := make([]int, len(bc.pending))
		var totalWaitTime time.Duration
		now := time.Now()

		for i, req := range bc.pending {
			priorities[i] = req.Priority
			totalWaitTime += now.Sub(req.Submitted)
		}

		sort.Ints(priorities)
		stats["pending_priorities"] = priorities
		stats["average_wait_time_ms"] = totalWaitTime.Milliseconds() / int64(len(bc.pending))
	}

	return stats
}

// CompilerPool manages a pool of query compilers for high concurrency scenarios
type CompilerPool struct {
	compilers []*QueryCompiler
	current   int
	mu        sync.Mutex
	// Load balancing metrics
	loads []int64 // Request count per compiler
}

// NewCompilerPool creates a new pool of query compilers
func NewCompilerPool(dialect dialects.Dialect, metadata *metadata.Registry, poolSize int, maxCacheSize int) *CompilerPool {
	if poolSize <= 0 {
		poolSize = 4
	}

	compilers := make([]*QueryCompiler, poolSize)
	loads := make([]int64, poolSize)
	for i := 0; i < poolSize; i++ {
		compilers[i] = NewQueryCompiler(dialect, metadata, maxCacheSize)
	}

	return &CompilerPool{
		compilers: compilers,
		loads:     loads,
	}
}

// GetCompiler returns the least loaded compiler for better load distribution
func (cp *CompilerPool) GetCompiler() *QueryCompiler {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	// Find the compiler with the lowest load
	minLoad := cp.loads[0]
	minIndex := 0

	for i := 1; i < len(cp.loads); i++ {
		if cp.loads[i] < minLoad {
			minLoad = cp.loads[i]
			minIndex = i
		}
	}

	cp.loads[minIndex]++
	return cp.compilers[minIndex]
}

// GetCompilerRoundRobin returns the next available compiler using round-robin
func (cp *CompilerPool) GetCompilerRoundRobin() *QueryCompiler {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	compiler := cp.compilers[cp.current]
	cp.loads[cp.current]++
	cp.current = (cp.current + 1) % len(cp.compilers)
	return compiler
}

// GetPoolStats returns comprehensive statistics for all compilers in the pool
func (cp *CompilerPool) GetPoolStats() map[string]interface{} {
	cp.mu.Lock()
	loads := make([]int64, len(cp.loads))
	copy(loads, cp.loads)
	cp.mu.Unlock()

	stats := make(map[string]interface{})
	stats["pool_size"] = len(cp.compilers)
	stats["request_loads"] = loads

	var totalCacheSize int
	var totalAccess int64
	var totalLoad int64
	compilerStats := make([]map[string]interface{}, len(cp.compilers))

	for i, compiler := range cp.compilers {
		compilerStat := compiler.GetCacheStats()
		compilerStats[i] = compilerStat
		totalCacheSize += compilerStat["cache_size"].(int)
		if totalAccessVal, ok := compilerStat["total_access"].(int64); ok {
			totalAccess += totalAccessVal
		}
		totalLoad += loads[i]
	}

	stats["total_cache_size"] = totalCacheSize
	stats["total_access"] = totalAccess
	stats["total_load"] = totalLoad
	stats["compilers"] = compilerStats

	// Calculate load distribution statistics
	if len(loads) > 0 {
		var maxLoad, minLoad int64
		maxLoad = loads[0]
		minLoad = loads[0]

		for _, load := range loads {
			if load > maxLoad {
				maxLoad = load
			}
			if load < minLoad {
				minLoad = load
			}
		}

		stats["max_load"] = maxLoad
		stats["min_load"] = minLoad
		stats["average_load"] = float64(totalLoad) / float64(len(loads))

		if totalLoad > 0 {
			stats["load_imbalance_ratio"] = float64(maxLoad-minLoad) / float64(totalLoad)
		}
	}

	return stats
}

// ClearAllCaches clears caches for all compilers in the pool
func (cp *CompilerPool) ClearAllCaches() {
	for _, compiler := range cp.compilers {
		compiler.ClearCache()
	}
}

// RebalancePool redistributes load counters (can be called periodically)
func (cp *CompilerPool) RebalancePool() {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	// Reset load counters to rebalance
	for i := range cp.loads {
		cp.loads[i] = 0
	}
}

// QueryOptimizationHint represents hints for query optimization
type QueryOptimizationHint struct {
	UseIndex      string        // Hint to use a specific index
	ForceJoin     string        // Force a specific join algorithm
	CacheDuration time.Duration // How long to cache this specific query
	Priority      int           // Query priority (higher = more important)
	// Database-specific hints
	PostgreSQLHints map[string]string // PostgreSQL-specific hints
	MySQLHints      map[string]string // MySQL-specific hints
	SQLServerHints  map[string]string // SQL Server-specific hints
	// Performance hints
	MaxRows       *int    // Limit maximum rows to return
	Timeout       *int    // Query timeout in seconds
	ReadOnly      bool    // Mark query as read-only
	NoCache       bool    // Skip caching for this query
	EstimatedCost float64 // Estimated cost override
}

// OptimizedQueryCompiler extends QueryCompiler with optimization hints
type OptimizedQueryCompiler struct {
	*QueryCompiler
	hints           map[string]QueryOptimizationHint
	patternHints    map[string]QueryOptimizationHint // Regex pattern-based hints
	mu              sync.RWMutex
	optimizationLog []OptimizationLogEntry
}

// OptimizationLogEntry represents a logged optimization action
type OptimizationLogEntry struct {
	Timestamp   time.Time
	QueryKey    string
	HintApplied string
	Performance float64 // Performance improvement ratio
	Success     bool
}

// NewOptimizedQueryCompiler creates a new optimized query compiler
func NewOptimizedQueryCompiler(dialect dialects.Dialect, metadata *metadata.Registry, maxCacheSize int) *OptimizedQueryCompiler {
	return &OptimizedQueryCompiler{
		QueryCompiler:   NewQueryCompiler(dialect, metadata, maxCacheSize),
		hints:           make(map[string]QueryOptimizationHint),
		patternHints:    make(map[string]QueryOptimizationHint),
		optimizationLog: make([]OptimizationLogEntry, 0, 1000),
	}
}

// SetOptimizationHint sets an optimization hint for a specific query key
func (oqc *OptimizedQueryCompiler) SetOptimizationHint(queryKey string, hint QueryOptimizationHint) {
	oqc.mu.Lock()
	defer oqc.mu.Unlock()
	oqc.hints[queryKey] = hint
}

// SetPatternHint sets an optimization hint for queries matching a regex pattern
func (oqc *OptimizedQueryCompiler) SetPatternHint(pattern string, hint QueryOptimizationHint) error {
	// Validate regex pattern
	if _, err := regexp.Compile(pattern); err != nil {
		return fmt.Errorf("invalid regex pattern: %w", err)
	}

	oqc.mu.Lock()
	defer oqc.mu.Unlock()
	oqc.patternHints[pattern] = hint
	return nil
}

// CompileWithHints compiles a query with optimization hints
func (oqc *OptimizedQueryCompiler) CompileWithHints(entityType interface{}, query *dialects.SelectQuery, hint *QueryOptimizationHint) (*CompiledQuery, error) {
	startTime := time.Now()

	// Generate base cache key
	baseKey := oqc.generateCacheKey(entityType, query)

	// Check for specific hints
	var appliedHint *QueryOptimizationHint
	if hint != nil {
		appliedHint = hint
	} else {
		// Look for cached hints
		oqc.mu.RLock()
		if cachedHint, exists := oqc.hints[baseKey]; exists {
			appliedHint = &cachedHint
		} else {
			// Check pattern-based hints
			for pattern, patternHint := range oqc.patternHints {
				if matched, _ := regexp.MatchString(pattern, baseKey); matched {
					patternHintCopy := patternHint
					appliedHint = &patternHintCopy
					break
				}
			}
		}
		oqc.mu.RUnlock()
	}

	// Compile base query
	compiled, err := oqc.Compile(entityType, query)
	if err != nil {
		return nil, err
	}

	// Apply optimization hints
	if appliedHint != nil {
		optimizedSQL := compiled.SQL
		optimizationApplied := false

		// Apply index hints
		if appliedHint.UseIndex != "" {
			optimizedSQL = oqc.applyIndexHint(optimizedSQL, appliedHint.UseIndex)
			optimizationApplied = true
		}

		// Apply join hints
		if appliedHint.ForceJoin != "" {
			optimizedSQL = oqc.applyJoinHint(optimizedSQL, appliedHint.ForceJoin)
			optimizationApplied = true
		}

		// Apply database-specific hints
		optimizedSQL = oqc.applyDatabaseSpecificHints(optimizedSQL, appliedHint)

		// Update compiled query with optimizations
		if optimizationApplied {
			compiled.OptimizedSQL = optimizedSQL
			compiled.Hints = appliedHint

			// Set custom cache expiry if specified
			if appliedHint.CacheDuration > 0 {
				expiry := time.Now().Add(appliedHint.CacheDuration)
				compiled.CacheExpiry = &expiry
			}

			// Override estimated cost if provided
			if appliedHint.EstimatedCost > 0 {
				compiled.EstimatedCost = appliedHint.EstimatedCost
			}
		}

		// Log optimization
		oqc.logOptimization(baseKey, appliedHint, time.Since(startTime).Seconds(), true)
	}

	return compiled, nil
}

// applyIndexHint applies database-specific index hints to SQL
func (oqc *OptimizedQueryCompiler) applyIndexHint(sql string, indexName string) string {
	switch oqc.dialect.Name() {
	case dialects.DriverMySQL:
		return oqc.applyMySQLIndexHint(sql, indexName)
	case dialects.DriverPostgres:
		return oqc.applyPostgreSQLIndexHint(sql, indexName)
	case dialects.DriverSQLServer:
		return oqc.applySQLServerIndexHint(sql, indexName)
	case dialects.DriverSQLite3, dialects.DriverSQLite:
		return oqc.applySQLiteIndexHint(sql, indexName)
	default:
		// Fallback to comment-based hint
		return sql + fmt.Sprintf(" /* USE INDEX (%s) */", indexName)
	}
}

// applyMySQLIndexHint applies MySQL-specific index hints
func (oqc *OptimizedQueryCompiler) applyMySQLIndexHint(sql string, indexName string) string {
	// MySQL syntax: SELECT ... FROM table USE INDEX (index_name) WHERE ...
	// Find the FROM clause and insert the hint after the table name

	fromRegex := regexp.MustCompile(`(?i)\bFROM\s+(\w+)`)
	match := fromRegex.FindStringSubmatch(sql)

	if len(match) > 1 {
		tableName := match[1]
		oldTableRef := fmt.Sprintf("FROM %s", tableName)
		newTableRef := fmt.Sprintf("FROM %s USE INDEX (%s)", tableName, indexName)
		return strings.Replace(sql, oldTableRef, newTableRef, 1)
	}

	return sql
}

// applyPostgreSQLIndexHint applies PostgreSQL-specific hints
func (oqc *OptimizedQueryCompiler) applyPostgreSQLIndexHint(sql string, indexName string) string {
	// PostgreSQL doesn't have direct index hints, but we can use query comments
	// and set enable_seqscan = off to encourage index usage
	return fmt.Sprintf("/*+ IndexScan(%s) */ %s", indexName, sql)
}

// applySQLServerIndexHint applies SQL Server-specific index hints
func (oqc *OptimizedQueryCompiler) applySQLServerIndexHint(sql string, indexName string) string {
	// SQL Server syntax: SELECT ... FROM table WITH (INDEX(index_name)) WHERE ...
	fromRegex := regexp.MustCompile(`(?i)\bFROM\s+(\w+)`)
	match := fromRegex.FindStringSubmatch(sql)

	if len(match) > 1 {
		tableName := match[1]
		oldTableRef := fmt.Sprintf("FROM %s", tableName)
		newTableRef := fmt.Sprintf("FROM %s WITH (INDEX(%s))", tableName, indexName)
		return strings.Replace(sql, oldTableRef, newTableRef, 1)
	}

	return sql
}

// applySQLiteIndexHint applies SQLite-specific index hints
func (oqc *OptimizedQueryCompiler) applySQLiteIndexHint(sql string, indexName string) string {
	// SQLite syntax: SELECT ... FROM table INDEXED BY index_name WHERE ...
	fromRegex := regexp.MustCompile(`(?i)\bFROM\s+(\w+)`)
	match := fromRegex.FindStringSubmatch(sql)

	if len(match) > 1 {
		tableName := match[1]
		oldTableRef := fmt.Sprintf("FROM %s", tableName)
		newTableRef := fmt.Sprintf("FROM %s INDEXED BY %s", tableName, indexName)
		return strings.Replace(sql, oldTableRef, newTableRef, 1)
	}

	return sql
}

// applyJoinHint applies join algorithm hints
func (oqc *OptimizedQueryCompiler) applyJoinHint(sql string, joinType string) string {
	switch oqc.dialect.Name() {
	case dialects.DriverMySQL:
		// MySQL supports join algorithm hints
		return fmt.Sprintf("/*+ %s */ %s", strings.ToUpper(joinType), sql)
	case dialects.DriverPostgres:
		// PostgresSQL join hints via comments
		titleCaser := cases.Title(language.Und)
		return fmt.Sprintf("/*+ %sJoin */ %s", titleCaser.String(joinType), sql)
	case dialects.DriverSQLServer:
		// SQL Server join hints
		joinHints := map[string]string{
			"hash":  "HASH",
			"merge": "MERGE",
			"loop":  "LOOP",
		}
		if hint, exists := joinHints[strings.ToLower(joinType)]; exists {
			return fmt.Sprintf("OPTION (%s JOIN) %s", hint, sql)
		}
	}

	return sql
}

// applyDatabaseSpecificHints applies database-specific optimization hints
func (oqc *OptimizedQueryCompiler) applyDatabaseSpecificHints(sql string, hint *QueryOptimizationHint) string {
	switch oqc.dialect.Name() {
	case dialects.DriverPostgres:
		if hint.PostgreSQLHints != nil {
			for hintType, value := range hint.PostgreSQLHints {
				sql = fmt.Sprintf("/*+ %s(%s) */ %s", hintType, value, sql)
			}
		}
	case dialects.DriverMySQL:
		if hint.MySQLHints != nil {
			for hintType, value := range hint.MySQLHints {
				sql = fmt.Sprintf("/*+ %s(%s) */ %s", hintType, value, sql)
			}
		}
	case dialects.DriverSQLServer:
		if hint.SQLServerHints != nil {
			var options []string
			for hintType, value := range hint.SQLServerHints {
				options = append(options, fmt.Sprintf("%s(%s)", hintType, value))
			}
			if len(options) > 0 {
				sql = fmt.Sprintf("%s OPTION (%s)", sql, strings.Join(options, ", "))
			}
		}
	}

	// Apply row limit hints
	if hint.MaxRows != nil && *hint.MaxRows > 0 {
		sql = oqc.applyRowLimitHint(sql, *hint.MaxRows)
	}

	// Apply timeout hints
	if hint.Timeout != nil && *hint.Timeout > 0 {
		sql = oqc.applyTimeoutHint(sql, *hint.Timeout)
	}

	return sql
}

// applyRowLimitHint applies row limit hints for performance
func (oqc *OptimizedQueryCompiler) applyRowLimitHint(sql string, maxRows int) string {
	// Only apply if query doesn't already have LIMIT
	if !strings.Contains(strings.ToUpper(sql), "LIMIT") {
		switch oqc.dialect.Name() {
		case dialects.DriverMySQL, dialects.DriverPostgres, dialects.DriverSQLite3, dialects.DriverSQLite:
			return fmt.Sprintf("%s LIMIT %d", sql, maxRows)
		case dialects.DriverSQLServer:
			// SQL Server uses TOP
			if strings.Contains(strings.ToUpper(sql), "SELECT") {
				return strings.Replace(sql, "SELECT", fmt.Sprintf("SELECT TOP %d", maxRows), 1)
			}
		}
	}
	return sql
}

// applyTimeoutHint applies query timeout hints
func (oqc *OptimizedQueryCompiler) applyTimeoutHint(sql string, timeoutSeconds int) string {
	switch oqc.dialect.Name() {
	case dialects.DriverMySQL:
		return fmt.Sprintf("/*+ MAX_EXECUTION_TIME(%d) */ %s", timeoutSeconds*1000, sql)
	case dialects.DriverPostgres:
		return fmt.Sprintf("SET statement_timeout = '%ds'; %s", timeoutSeconds, sql)
	case dialects.DriverSQLServer:
		return fmt.Sprintf("%s OPTION (QUERY_TIMEOUT %d)", sql, timeoutSeconds)
	}
	return sql
}

// logOptimization logs an optimization action
func (oqc *OptimizedQueryCompiler) logOptimization(queryKey string, hint *QueryOptimizationHint, performance float64, success bool) {
	oqc.mu.Lock()
	defer oqc.mu.Unlock()

	entry := OptimizationLogEntry{
		Timestamp:   time.Now(),
		QueryKey:    queryKey,
		HintApplied: oqc.formatHintDescription(hint),
		Performance: performance,
		Success:     success,
	}

	oqc.optimizationLog = append(oqc.optimizationLog, entry)

	// Keep only last 1000 entries
	if len(oqc.optimizationLog) > 1000 {
		oqc.optimizationLog = oqc.optimizationLog[len(oqc.optimizationLog)-1000:]
	}
}

// formatHintDescription creates a human-readable description of applied hints
func (oqc *OptimizedQueryCompiler) formatHintDescription(hint *QueryOptimizationHint) string {
	var descriptions []string

	if hint.UseIndex != "" {
		descriptions = append(descriptions, fmt.Sprintf("INDEX:%s", hint.UseIndex))
	}
	if hint.ForceJoin != "" {
		descriptions = append(descriptions, fmt.Sprintf("JOIN:%s", hint.ForceJoin))
	}
	if hint.MaxRows != nil {
		descriptions = append(descriptions, fmt.Sprintf("LIMIT:%d", *hint.MaxRows))
	}
	if hint.CacheDuration > 0 {
		descriptions = append(descriptions, fmt.Sprintf("CACHE:%v", hint.CacheDuration))
	}

	if len(descriptions) == 0 {
		return "NONE"
	}

	return strings.Join(descriptions, ",")
}

// GetOptimizationStats returns statistics about query optimizations
func (oqc *OptimizedQueryCompiler) GetOptimizationStats() map[string]interface{} {
	oqc.mu.RLock()
	defer oqc.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["total_hints"] = len(oqc.hints)
	stats["pattern_hints"] = len(oqc.patternHints)
	stats["optimization_log_entries"] = len(oqc.optimizationLog)

	if len(oqc.optimizationLog) > 0 {
		var successCount int
		var totalPerformance float64

		for _, entry := range oqc.optimizationLog {
			if entry.Success {
				successCount++
			}
			totalPerformance += entry.Performance
		}

		stats["success_rate"] = float64(successCount) / float64(len(oqc.optimizationLog))
		stats["average_performance"] = totalPerformance / float64(len(oqc.optimizationLog))
	}

	// Include base compiler stats
	baseStats := oqc.GetCacheStats()
	for k, v := range baseStats {
		stats[k] = v
	}

	return stats
}

// ClearOptimizationLog clears the optimization log
func (oqc *OptimizedQueryCompiler) ClearOptimizationLog() {
	oqc.mu.Lock()
	defer oqc.mu.Unlock()
	oqc.optimizationLog = oqc.optimizationLog[:0]
}

// GetOptimizationLog returns a copy of the optimization log
func (oqc *OptimizedQueryCompiler) GetOptimizationLog() []OptimizationLogEntry {
	oqc.mu.RLock()
	defer oqc.mu.RUnlock()

	logCopy := make([]OptimizationLogEntry, len(oqc.optimizationLog))
	copy(logCopy, oqc.optimizationLog)
	return logCopy
}
