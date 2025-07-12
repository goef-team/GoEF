# GoEF Architecture Overview

GoEF is designed as a modular, high-performance ORM that follows modern software architecture principles. This document provides a comprehensive overview of the system design, component interactions, and architectural decisions.

## 🏗️ System Architecture

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    Application Layer                            │
├─────────────────────────────────────────────────────────────────┤
│                      GoEF ORM                                   │
│                                                                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐              │
│  │   DbContext │  │   DbSet     │  │   Entity    │              │
│  │             │  │             │  │             │              │
│  │ • UnitOfWork│  │ • Queries   │  │ • Models    │              │
│  │ • Tracking  │  │ • CRUD      │  │ • Relations │              │
│  │ • Txns      │  │ • Filtering │  │ • Metadata  │              │
│  └─────────────┘  └─────────────┘  └─────────────┘              │
│                                                                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐              │
│  │   Query     │  │   Change    │  │  Migration  │              │
│  │   Engine    │  │   Tracking  │  │   Engine    │              │
│  │             │  │             │  │             │              │
│  │ • LINQ      │  │ • State     │  │ • Schema    │              │
│  │ • Compiler  │  │ • Detector  │  │ • Diff      │              │
│  │ • Cache     │  │ • UoW       │  │ • Versioning│              │
│  └─────────────┘  └─────────────┘  └─────────────┘              │
│                                                                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐              │
│  │  Metadata   │  │  Dialect    │  │  Loader     │              │
│  │  Registry   │  │  Engine     │  │  Engine     │              │
│  │             │  │             │  │             │              │
│  │ • Entities  │  │ • SQL Gen   │  │ • Eager     │              │
│  │ • Relations │  │ • Type Map  │  │ • Lazy      │              │
│  │ • Cache     │  │ • Features  │  │ • Relations │              │
│  └─────────────┘  └─────────────┘  └─────────────┘              │
├─────────────────────────────────────────────────────────────────┤
│                    Database Layer                               │
│                                                                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐              │
│  │ PostgreSQL  │  │   MySQL     │  │   SQLite    │              │
│  └─────────────┘  └─────────────┘  └─────────────┘              │
│                                                                 │
│  ┌─────────────┐  ┌─────────────┐                               │
│  │ SQL Server  │  │  MongoDB    │                               │
│  └─────────────┘  └─────────────┘                               │
└─────────────────────────────────────────────────────────────────┘
```

## 🔧 Core Components

### 1. DbContext - The Central Coordinator

The `DbContext` serves as the primary interface for database operations and implements the Unit of Work pattern.

#### Responsibilities:
- **Connection Management**: Database connection lifecycle and pooling
- **Transaction Coordination**: Begin, commit, and rollback operations
- **Change Tracking**: Monitor entity modifications
- **Query Execution**: Coordinate query compilation and execution
- **Cache Management**: Entity and query result caching

#### Key Features:
```go
type DbContext struct {
    db            *sql.DB              // Database connection
    dialect       dialects.Dialect     // Database-specific operations
    metadata      *metadata.Registry   // Entity metadata cache
    changeTracker *change.Tracker      // Change tracking system
    loader        *loader.Loader       // Relationship loader
    options       *Options             // Configuration options
    tx            *sql.Tx              // Current transaction
}
```

#### Thread Safety:
- **Per-Request Scoped**: Each HTTP request should use its own DbContext instance
- **Not Thread-Safe**: A single DbContext should not be shared across goroutines
- **Connection Pool**: The underlying connection pool is thread-safe

#### Best Practices:
```go
// ✅ Good: Per-request context
func UserHandler(w http.ResponseWriter, r *http.Request) {
    ctx := createDbContext()
    defer ctx.Close()
    
    // Use ctx for this request only
}

// ❌ Bad: Shared context across requests
var globalCtx *goef.DbContext // Don't do this!
```

### 2. Query Engine - Type-Safe Query Building

The query engine provides LINQ-style querying with compile-time type safety and runtime optimization.

#### Architecture:
```
Query Request → Expression Tree → SQL Generation → Execution → Result Mapping
```

#### Components:

##### Query Builder
```go
type QueryBuilder struct {
    dialect     dialects.Dialect
    metadata    *metadata.Registry
    expression  Expression          // Expression tree
    compiler    *QueryCompiler      // Query compilation
    cache       *QueryCache         // Compiled query cache
}
```

##### Expression System
- **Expression Trees**: Represent queries as typed expression trees
- **Type Safety**: Compile-time validation of queries
- **SQL Translation**: Convert expressions to database-specific SQL

```go
// Type-safe query building
users := queryable.
    WhereExpr(query.GreaterThanEx(func(u User) interface{} { return u.Age }, 21)).
    WhereExpr(query.EqualEx(func(u User) interface{} { return u.IsActive }, true)).
    OrderByColumn("created_at").
    Take(50)
```

##### Query Compilation and Caching
- **Compilation**: Convert expression trees to optimized SQL
- **Caching**: Store compiled queries for reuse
- **Parameterization**: Automatic parameter binding and SQL injection prevention

#### Performance Optimizations:
- **Query Plan Caching**: Compiled queries are cached by expression signature
- **Batch Compilation**: Multiple queries compiled together for efficiency
- **Lazy Evaluation**: Queries execute only when results are materialized

### 3. Change Tracking System

Automatic detection and management of entity changes with configurable tracking modes.

#### State Management:
```go
type EntityState int

const (
    StateUnchanged EntityState = iota  // No changes
    StateAdded                         // New entity
    StateModified                      // Changed entity
    StateDeleted                       // Marked for deletion
)
```

#### Change Detection:
- **Automatic**: Monitor property changes via reflection
- **Manual**: Explicit change detection calls
- **Snapshot**: Compare current vs original values

#### Implementation:
```go
type Tracker struct {
    enabled   bool                        // Feature toggle
    entities  map[string]*EntityEntry     // Tracked entities
    snapshots map[string]interface{}      // Original values
}
```

#### Performance Considerations:
- **Memory Usage**: Tracking stores entity snapshots
- **CPU Overhead**: Change detection requires value comparison
- **Configurable**: Can be disabled for read-only scenarios

```go
// Disable for read-heavy workloads
ctx := goef.NewDbContext(
    goef.WithPostgreSQL(connectionString),
    goef.WithChangeTracking(false), // Improves performance
)
```

### 4. Metadata System

Efficient entity introspection and relationship discovery with aggressive caching.

#### Registry Architecture:
```go
type Registry struct {
    entities map[reflect.Type]*EntityMetadata  // Type → Metadata
    cache    *sync.RWMutex                     // Thread-safe access
}

type EntityMetadata struct {
    Type          reflect.Type              // Go type
    TableName     string                    // Database table
    Fields        []FieldMetadata           // Column mappings
    Relationships []RelationshipMetadata    // Entity relations
    Indexes       []IndexMetadata           // Database indexes
    Constraints   []ConstraintMetadata      // Table constraints
}
```

#### Caching Strategy:
- **Lazy Loading**: Metadata extracted on first access
- **Immutable**: Metadata is cached permanently once extracted
- **Thread-Safe**: Protected by RWMutex for concurrent access

#### Field Mapping:
```go
type FieldMetadata struct {
    Name            string              // Go field name
    ColumnName      string              // Database column
    Type            reflect.Type        // Go type
    IsPrimaryKey    bool               // Primary key flag
    IsRequired      bool               // NOT NULL constraint
    MaxLength       int                // String length limit
    DefaultValue    interface{}        // Default value
}
```

### 5. Dialect System

Database-specific SQL generation and feature support with a plugin architecture.

#### Interface Design:
```go
type Dialect interface {
    Name() string
    Quote(identifier string) string
    Placeholder(n int) string
    BuildSelect(table string, query *SelectQuery) (string, []interface{}, error)
    BuildInsert(table string, fields []string, values []interface{}) (string, []interface{}, error)
    BuildUpdate(table string, fields []string, values []interface{}, where string, args []interface{}) (string, []interface{}, error)
    BuildDelete(table string, where string, args []interface{}) (string, []interface{}, error)
    MapGoTypeToSQL(goType reflect.Type, maxLength int, isAutoIncrement bool) string
    SupportsReturning() bool
    SupportsTransactions() bool
    SupportsCTE() bool
}
```

#### Supported Databases:

##### PostgreSQL
- **Advanced Features**: JSONB, arrays, CTEs, window functions
- **Performance**: Native RETURNING clause, bulk operations
- **Type Mapping**: Rich type system support

##### MySQL
- **Compatibility**: MySQL 5.7+ and MariaDB support
- **Features**: JSON columns, generated columns
- **Optimization**: Batch operations, connection pooling

##### SQLite
- **Embedded**: Perfect for development and small applications
- **Features**: Modern SQLite features (CTEs, window functions)
- **Performance**: Optimized for single-user scenarios

##### SQL Server
- **Enterprise**: Full T-SQL support
- **Features**: OUTPUT clause, stored procedures
- **Integration**: Windows authentication, Azure SQL

##### MongoDB
- **Document Store**: NoSQL document database support
- **Aggregation**: Pipeline-based queries
- **Flexibility**: Schema-less design with validation

### 6. Migration Engine

Code-first database schema management with automatic diffing and version control.

#### Migration Workflow:
```
Model Changes → Schema Diff → Migration Script → Database Update
```

#### Components:
```go
type Migrator struct {
    db          *sql.DB
    dialect     dialects.Dialect
    metadata    *metadata.Registry
    history     *MigrationHistory
}

type Migration struct {
    ID          string        // Unique migration identifier
    Name        string        // Human-readable name
    Version     int64         // Sequential version number
    UpSQL       string        // Forward migration SQL
    DownSQL     string        // Rollback migration SQL
    Checksum    string        // Content verification
    AppliedAt   *time.Time    // Application timestamp
}
```

#### Features:
- **Auto-Diff**: Compare current model with database schema
- **Versioning**: Sequential migration ordering
- **Rollback**: Safe rollback to previous versions
- **Verification**: Checksum validation for migration integrity

## 📊 Performance Architecture

### Connection Management

#### Connection Pooling:
```go
// Optimized pool configuration
ctx, err := goef.NewDbContext(
    goef.WithPostgreSQL(connectionString),
    goef.WithConnectionPool(
        100,                    // Max open connections
        25,                     // Max idle connections  
        time.Hour*2,           // Connection max lifetime
        time.Minute*30,        // Connection max idle time
    ),
)
```

#### Pool Sizing Guidelines:
- **CPU-bound**: Pool size = CPU cores * 2
- **I/O-bound**: Pool size = CPU cores * 4-8
- **Mixed workload**: Start with CPU cores * 4, tune based on monitoring

### Query Performance

#### Query Compilation Cache:
```go
type QueryCompiler struct {
    cache    map[string]*CompiledQuery  // Query cache
    maxSize  int                        // Cache size limit
    stats    CacheStats                 // Performance metrics
}
```

#### Cache Strategy:
- **LRU Eviction**: Least recently used queries evicted first
- **Cache Key**: Based on expression tree signature
- **Hit Rate**: Target >90% hit rate for optimal performance

#### Memory Management:
- **Object Pooling**: Reuse query objects to reduce GC pressure
- **Smart Batching**: Group operations to minimize round trips
- **Lazy Loading**: Load related data on-demand

### Concurrency Design

#### Thread Safety Model:
- **DbContext**: Not thread-safe (per-request scope)
- **Metadata Registry**: Thread-safe with RWMutex
- **Query Compiler**: Thread-safe with proper locking
- **Connection Pool**: Thread-safe via database/sql

#### Scaling Patterns:
```go
// Worker pool pattern for bulk operations
func ProcessUsers(users []User) error {
    const workerCount = 10
    const batchSize = 100
    
    work := make(chan []User, workerCount)
    errors := make(chan error, workerCount)
    
    // Start workers
    for i := 0; i < workerCount; i++ {
        go worker(work, errors)
    }
    
    // Distribute work
    for i := 0; i < len(users); i += batchSize {
        end := i + batchSize
        if end > len(users) {
            end = len(users)
        }
        work <- users[i:end]
    }
    
    // Collect results
    close(work)
    for i := 0; i < workerCount; i++ {
        if err := <-errors; err != nil {
            return err
        }
    }
    
    return nil
}

func worker(work <-chan []User, errors chan<- error) {
    ctx := createDbContext()
    defer ctx.Close()
    
    for batch := range work {
        if err := processBatch(ctx, batch); err != nil {
            errors <- err
            return
        }
    }
    errors <- nil
}
```

## 🔐 Security Architecture

### SQL Injection Prevention
- **Parameterized Queries**: All queries use parameter binding
- **Input Validation**: Automatic escaping and validation
- **No Dynamic SQL**: Query building prevents SQL injection

### Connection Security
- **TLS/SSL**: Encrypted database connections
- **Credential Management**: Secure credential storage
- **Connection Validation**: Verify connection integrity

### Access Control
- **Database Permissions**: Principle of least privilege
- **Audit Logging**: Track all database operations
- **Schema Validation**: Prevent unauthorized schema changes

## 📈 Monitoring and Observability

### Performance Metrics
```go
type Metrics struct {
    QueryCount          int64         // Total queries executed
    QueryDuration       time.Duration // Total query time
    ConnectionsActive   int32         // Active connections
    ConnectionsIdle     int32         // Idle connections
    CacheHitRate       float64       // Query cache hit rate
    EntityCount        int64         // Entities tracked
    TransactionCount   int64         // Transactions executed
}
```

### Logging Integration
```go
// Structured logging support
ctx, err := goef.NewDbContext(
    goef.WithPostgreSQL(connectionString),
    goef.WithSQLLogging(true),
    goef.WithQueryTimeout(time.Second*30),
    goef.WithMetricsEnabled(true),
)
```

### Health Checks
```go
func (ctx *DbContext) HealthCheck() error {
    // Verify database connectivity
    if err := ctx.db.Ping(); err != nil {
        return fmt.Errorf("database ping failed: %w", err)
    }
    
    // Check connection pool health
    stats := ctx.db.Stats()
    if stats.OpenConnections > stats.MaxOpenConnections*90/100 {
        return fmt.Errorf("connection pool nearly exhausted")
    }
    
    return nil
}
```

## 🎯 Best Practices

### Context Management
```go
// ✅ Proper context lifecycle
func HandleRequest(w http.ResponseWriter, r *http.Request) {
    ctx := createDbContext()
    defer ctx.Close()              // Always close
    
    // Set request timeout
    reqCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
    defer cancel()
    
    // Use context for operations
    users, err := ctx.Set(&User{}).ToListWithContext(reqCtx)
    if err != nil {
        http.Error(w, err.Error(), 500)
        return
    }
    
    json.NewEncoder(w).Encode(users)
}
```

### Error Handling
```go
// ✅ Comprehensive error handling
func CreateUser(ctx *goef.DbContext, user *User) error {
    // Validate input
    if err := validateUser(user); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }
    
    // Begin transaction
    if err := ctx.BeginTransaction(); err != nil {
        return fmt.Errorf("failed to begin transaction: %w", err)
    }
    
    // Ensure cleanup
    defer func() {
        if r := recover(); r != nil {
            ctx.RollbackTransaction()
            panic(r)
        }
    }()
    
    // Perform operations
    if err := ctx.Add(user); err != nil {
        ctx.RollbackTransaction()
        return fmt.Errorf("failed to add user: %w", err)
    }
    
    if _, err := ctx.SaveChanges(); err != nil {
        ctx.RollbackTransaction()
        return fmt.Errorf("failed to save changes: %w", err)
    }
    
    // Commit transaction
    if err := ctx.CommitTransaction(); err != nil {
        return fmt.Errorf("failed to commit transaction: %w", err)
    }
    
    return nil
}
```

### Performance Optimization
```go
// ✅ Optimized bulk operations
func BulkInsertUsers(ctx *goef.DbContext, users []User) error {
    const batchSize = 1000
    
    if err := ctx.BeginTransaction(); err != nil {
        return err
    }
    defer ctx.RollbackTransaction()
    
    for i := 0; i < len(users); i += batchSize {
        end := i + batchSize
        if end > len(users) {
            end = len(users)
        }
        
        // Add batch
        for j := i; j < end; j++ {
            if err := ctx.Add(&users[j]); err != nil {
                return err
            }
        }
        
        // Save batch
        if _, err := ctx.SaveChanges(); err != nil {
            return err
        }
    }
    
    return ctx.CommitTransaction()
}
```

This architecture provides a solid foundation for building high-performance, scalable applications with GoEF while maintaining type safety, reliability, and excellent developer experience.
