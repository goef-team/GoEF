package unit

import (
	"fmt"
	"strings"
	"testing"

	"github.com/goef-team/goef"
)

// TestLogger implements the Logger interface for testing
type TestLogger struct {
	InfoLogs  []string
	ErrorLogs []string
	DebugLogs []string
}

func (l *TestLogger) Info(msg string, args ...interface{}) {
	formatted := fmt.Sprintf(msg, args...)
	l.InfoLogs = append(l.InfoLogs, formatted)
}

func (l *TestLogger) Error(msg string, args ...interface{}) {
	formatted := fmt.Sprintf(msg, args...)
	l.ErrorLogs = append(l.ErrorLogs, formatted)
}

func (l *TestLogger) Debug(msg string, args ...interface{}) {
	formatted := fmt.Sprintf(msg, args...)
	l.DebugLogs = append(l.DebugLogs, formatted)
}

func TestDbContext_CustomLogger(t *testing.T) {
	testLogger := &TestLogger{}

	// Create context with custom logger
	ctx, err := goef.NewDbContext(
		goef.WithSQLite(":memory:"),
		goef.WithSQLLogging(true),
		goef.WithLogger(testLogger),
	)
	if err != nil {
		t.Fatalf("Failed to create DbContext: %v", err)
	}
	defer func(ctx *goef.DbContext) {
		err = ctx.Close()
		if err != nil {
			t.Errorf("Failed to close DbContext: %v", err)
		}
	}(ctx)

	// Test that logSQL uses the custom logger
	ctx.LogSQL("SELECT * FROM users WHERE id = ?", []interface{}{1})

	// Verify the debug log was called
	if len(testLogger.DebugLogs) == 0 {
		t.Error("Expected debug log to be called")
	}

	expectedContent := "SQL executed: SELECT * FROM users WHERE id = ? with args: [1]"
	if !strings.Contains(testLogger.DebugLogs[0], expectedContent) {
		t.Errorf("Expected debug log to contain '%s', got: %s", expectedContent, testLogger.DebugLogs[0])
	}
}

func TestDbContext_DefaultLogger(t *testing.T) {
	// Create context without custom logger (should use default)
	ctx, err := goef.NewDbContext(
		goef.WithSQLite(":memory:"),
		goef.WithSQLLogging(true),
	)
	if err != nil {
		t.Fatalf("Failed to create DbContext: %v", err)
	}
	defer func(ctx *goef.DbContext) {
		err = ctx.Close()
		if err != nil {
			t.Errorf("Failed to close DbContext: %v", err)
		}
	}(ctx)

	// Verify default logger is set
	if ctx.Options().Logger == nil {
		t.Error("Expected default logger to be set")
	}

	// Test that logSQL works with default logger (no panic)
	ctx.LogSQL("SELECT * FROM users", []interface{}{})
}

func TestDbContext_NoLogger(t *testing.T) {
	// Create context with nil logger
	ctx, err := goef.NewDbContext(
		goef.WithSQLite(":memory:"),
		goef.WithSQLLogging(true),
		goef.WithLogger(nil),
	)
	if err != nil {
		t.Fatalf("Failed to create DbContext: %v", err)
	}
	defer func(ctx *goef.DbContext) {
		err = ctx.Close()
		if err != nil {
			t.Fatalf("Failed to close DbContext: %v", err)
		}
	}(ctx)

	// Test that logSQL works even with nil logger (should not panic)
	ctx.LogSQL("SELECT * FROM users", []interface{}{})
}

func TestDbContext_LoggingDisabled(t *testing.T) {
	testLogger := &TestLogger{}

	// Create context with logging disabled
	ctx, err := goef.NewDbContext(
		goef.WithSQLite(":memory:"),
		goef.WithSQLLogging(false), // Disabled
		goef.WithLogger(testLogger),
	)
	if err != nil {
		t.Fatalf("Failed to create DbContext: %v", err)
	}
	defer func(ctx *goef.DbContext) {
		err = ctx.Close()
		if err != nil {
			t.Errorf("Failed to close DbContext: %v", err)
		}
	}(ctx)

	// Test that logSQL doesn't log when disabled
	ctx.LogSQL("SELECT * FROM users", []interface{}{})

	// Verify no logs were written
	if len(testLogger.DebugLogs) > 0 {
		t.Error("Expected no debug logs when SQL logging is disabled")
	}
}
