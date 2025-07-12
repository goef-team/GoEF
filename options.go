package goef

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// Logger interface for GoEF logging - compatible with migration logger
type Logger interface {
	Info(msg string, args ...interface{})
	Error(msg string, args ...interface{})
	Debug(msg string, args ...interface{})
}

// DefaultLogger is a simple console logger for GoEF
type DefaultLogger struct{}

func (l *DefaultLogger) Info(msg string, args ...interface{}) {
	log.Printf("[INFO] "+msg, args...)
}

func (l *DefaultLogger) Error(msg string, args ...interface{}) {
	log.Printf("[ERROR] "+msg, args...)
}

func (l *DefaultLogger) Debug(msg string, args ...interface{}) {
	log.Printf("[DEBUG] "+msg, args...)
}

// Options contains configuration options for DbContext
type Options struct {
	// Database connection settings
	DB               *sql.DB // Existing database connection
	DriverName       string
	ConnectionString string

	// Feature flags
	ChangeTracking bool
	LazyLoading    bool
	AutoMigrate    bool
	LogSQL         bool

	// Logging
	Logger Logger

	// Performance settings
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration

	// Dialect-specific options
	DialectOptions map[string]interface{}
}

// Option is a function that configures Options
type Option func(*Options) error

// WithPostgreSQL configures the context to use PostgreSQL
func WithPostgreSQL(connectionString string) Option {
	return func(o *Options) error {
		o.DriverName = "postgres"
		o.ConnectionString = connectionString
		return nil
	}
}

// WithDB allows providing an existing *sql.DB connection
func WithDB(db *sql.DB) Option {
	return func(o *Options) error {
		if db == nil {
			return fmt.Errorf("provided *sql.DB connection cannot be nil")
		}
		o.DB = db
		// If an existing DB is provided, we might not need DriverName/ConnectionString
		// but the dialect still needs a driver name.
		// We assume the user will also provide a compatible driver name option (e.g. WithSQLite)
		// or the dialect can be inferred if the *sql.DB driver provides this info (not standard).
		return nil
	}
}

// WithMySQL configures the context to use MySQL
func WithMySQL(connectionString string) Option {
	return func(o *Options) error {
		o.DriverName = "mysql"
		o.ConnectionString = connectionString
		return nil
	}
}

// WithSQLite configures the context to use SQLite
func WithSQLite(connectionString string) Option {
	return func(o *Options) error {
		o.DriverName = "sqlite3"
		o.ConnectionString = connectionString
		return nil
	}
}

// WithSQLServer configures the context to use SQL Server
func WithSQLServer(connectionString string) Option {
	return func(o *Options) error {
		o.DriverName = "sqlserver"
		o.ConnectionString = connectionString
		return nil
	}
}

func WithLogger(logger Logger) Option {
	return func(o *Options) error {
		o.Logger = logger
		return nil
	}
}

// WithMongoDB configures the context to use MongoDB
func WithMongoDB(connectionString string) Option {
	return func(o *Options) error {
		o.DriverName = "mongodb"
		o.ConnectionString = connectionString
		return nil
	}
}

// WithChangeTracking enables or disables change tracking
func WithChangeTracking(enabled bool) Option {
	return func(o *Options) error {
		o.ChangeTracking = enabled
		return nil
	}
}

// WithLazyLoading enables or disables lazy loading
func WithLazyLoading(enabled bool) Option {
	return func(o *Options) error {
		o.LazyLoading = enabled
		return nil
	}
}

// WithAutoMigrate enables or disables automatic migrations
func WithAutoMigrate(enabled bool) Option {
	return func(o *Options) error {
		o.AutoMigrate = enabled
		return nil
	}
}

// WithSQLLogging enables or disables SQL logging
func WithSQLLogging(enabled bool) Option {
	return func(o *Options) error {
		o.LogSQL = enabled
		return nil
	}
}

// WithConnectionPool configures the connection pool settings
func WithConnectionPool(maxOpen, maxIdle int, maxLifetime, maxIdleTime time.Duration) Option {
	return func(o *Options) error {
		o.MaxOpenConns = maxOpen
		o.MaxIdleConns = maxIdle
		o.ConnMaxLifetime = maxLifetime
		o.ConnMaxIdleTime = maxIdleTime
		return nil
	}
}

// WithDialectOptions sets dialect-specific options
func WithDialectOptions(options map[string]interface{}) Option {
	return func(o *Options) error {
		if o.DialectOptions == nil {
			o.DialectOptions = make(map[string]interface{})
		}
		for k, v := range options {
			o.DialectOptions[k] = v
		}
		return nil
	}
}
