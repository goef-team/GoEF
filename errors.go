package goef

import "errors"

var (
	// ErrInvalidConnectionString is returned when the connection string is invalid
	ErrInvalidConnectionString = errors.New("invalid connection string")

	// ErrContextClosed is returned when attempting to use a closed context
	ErrContextClosed = errors.New("database context is closed")

	// ErrTransactionAlreadyStarted is returned when trying to start a transaction
	// when one is already active
	ErrTransactionAlreadyStarted = errors.New("transaction already started")

	// ErrNoActiveTransaction is returned when trying to commit/rollback without
	// an active transaction
	ErrNoActiveTransaction = errors.New("no active transaction")

	// ErrEntityNotFound is returned when an entity is not found
	ErrEntityNotFound = errors.New("entity not found")

	// ErrInvalidEntity is returned when an entity is invalid
	ErrInvalidEntity = errors.New("invalid entity")

	// ErrUnsupportedDialect is returned when a dialect is not supported
	ErrUnsupportedDialect = errors.New("unsupported dialect")

	// ErrMigrationFailed is returned when a migration fails
	ErrMigrationFailed = errors.New("migration failed")

	// ErrInvalidQuery is returned when a query is invalid
	ErrInvalidQuery = errors.New("invalid query")

	// ErrOptimisticConcurrency is returned when an optimistic concurrency violation occurs
	ErrOptimisticConcurrency = errors.New("optimistic concurrency violation")
)

// GoEFError represents a GoEF-specific error with additional context
type GoEFError struct {
	Code    string
	Message string
	Cause   error
}

// Error returns the error message
func (e *GoEFError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

// Unwrap returns the underlying error
func (e *GoEFError) Unwrap() error {
	return e.Cause
}

// NewGoEFError creates a new GoEF error
func NewGoEFError(code, message string, cause error) *GoEFError {
	return &GoEFError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}
