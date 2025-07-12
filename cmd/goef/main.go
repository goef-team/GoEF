package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/goef-team/goef/dialects"
	"github.com/goef-team/goef/metadata"
	"github.com/goef-team/goef/migration"
)

// Build-time variables injected via ldflags
var (
	buildTime = "unknown"
	commit    = "unknown"
	version   = "dev"
)

// Config represents the CLI configuration
type Config struct {
	Database struct {
		Driver           string `json:"driver"`
		ConnectionString string `json:"connection_string"`
	} `json:"database"`
	Migrations struct {
		Directory string `json:"directory"`
		TableName string `json:"table_name"`
	} `json:"migrations"`
}

// validateConfigPath validates the configuration file path to prevent directory traversal attacks
func validateConfigPath(configPath string) error {
	cleanPath := filepath.Clean(configPath)

	// Ensure the path doesn't contain any dangerous elements
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("invalid config path: path traversal detected")
	}

	// Ensure the path doesn't start with /etc, /sys, /proc on Unix systems
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return fmt.Errorf("unable to resolve absolute path: %w", err)
	}

	// Check for suspicious absolute paths
	dangerousPrefixes := []string{"/etc/", "/sys/", "/proc/", "/dev/"}
	for _, prefix := range dangerousPrefixes {
		if strings.HasPrefix(absPath, prefix) {
			return fmt.Errorf("access to system directory denied: %s", absPath)
		}
	}

	return nil
}

// validateMigrationPath validates migration file paths to prevent directory traversal
func validateMigrationPath(migrationPath string) error {
	cleanPath := filepath.Clean(migrationPath)

	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("invalid migration path: path traversal detected")
	}

	// Ensure it's a .sql file
	if !strings.HasSuffix(cleanPath, ".sql") {
		return fmt.Errorf("invalid migration file: must be a .sql file")
	}

	return nil
}

func main() {
	var (
		command    = flag.String("command", "", "Command to execute (migrate, generate, status, rollback)")
		config     = flag.String("config", "goef.json", "Configuration file path")
		verbose    = flag.Bool("verbose", false, "Enable verbose logging")
		name       = flag.String("name", "", "Migration name (for generate command)")
		target     = flag.String("target", "", "Target migration (for rollback command)")
		dryRun     = flag.Bool("dry-run", false, "Show what would be done without executing")
		showVer    = flag.Bool("version", false, "Show version information")
		versionAlt = flag.Bool("v", false, "Show version information (short)")
	)
	flag.Parse()

	// Handle version flags
	if *showVer || *versionAlt {
		showVersion()
		return
	}

	if *command == "" {
		printUsage()
		os.Exit(1)
	}

	// Load configuration
	cfg, err := loadConfig(*config)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	switch *command {
	case "migrate":
		runMigrations(cfg, *verbose, *dryRun)
	case "generate":
		generateMigration(cfg, *name, *verbose)
	case "status":
		showMigrationStatus(cfg, *verbose)
	case "rollback":
		rollbackMigration(cfg, *target, *verbose, *dryRun)
	case "version":
		showVersion()
	default:
		fmt.Printf("Unknown command: %s\n", *command)
		printUsage()
		os.Exit(1)
	}
}

// showVersion displays version information
func showVersion() {
	fmt.Printf("GoEF CLI Tool\n")
	fmt.Printf("Version: %s\n", version)
	fmt.Printf("Build Time: %s\n", buildTime)
	fmt.Printf("Commit: %s\n", commit)
	fmt.Printf("Go Version: %s\n", runtime.Version())
}

func printUsage() {
	fmt.Println("GoEF CLI Tool - Database Migration Management")
	fmt.Println("")
	fmt.Printf("Version: %s\n", version)
	fmt.Println("")
	fmt.Println("Usage: goef [options] -command <command>")
	fmt.Println("   or: goef --version")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  migrate   - Apply pending migrations")
	fmt.Println("  generate  - Generate a new migration")
	fmt.Println("  status    - Show migration status")
	fmt.Println("  rollback  - Rollback a migration")
	fmt.Println("  version   - Show version information")
	fmt.Println("")
	fmt.Println("Options:")
	fmt.Println("  -config   - Configuration file path (default: goef.json)")
	fmt.Println("  -verbose  - Enable verbose logging")
	fmt.Println("  -name     - Migration name (for generate command)")
	fmt.Println("  -target   - Target migration (for rollback command)")
	fmt.Println("  -dry-run  - Show what would be done without executing")
	fmt.Println("  -version  - Show version information")
	fmt.Println("  -v        - Show version information (short)")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  goef --version")
	fmt.Println("  goef -command migrate")
	fmt.Println("  goef -command generate -name \"AddUserTable\"")
	fmt.Println("  goef -command status -verbose")
	fmt.Println("  goef -command rollback -target 001_create_users")
}

func loadConfig(configPath string) (*Config, error) {
	// Validate config path before use (fixes G304)
	if err := validateConfigPath(configPath); err != nil {
		return nil, fmt.Errorf("invalid config path: %w", err)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create default config if it doesn't exist
		return createDefaultConfig(configPath)
	}

	// #nosec G304 - path has been validated
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

func createDefaultConfig(configPath string) (*Config, error) {
	cfg := &Config{}
	cfg.Database.Driver = "sqlite3"
	cfg.Database.ConnectionString = "app.db"
	cfg.Migrations.Directory = "migrations"
	cfg.Migrations.TableName = "goef_migrations"

	// Write default config
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal default config: %w", err)
	}

	// Fix G306: Use 0600 instead of 0644 for better security
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return nil, fmt.Errorf("failed to write default config: %w", err)
	}

	fmt.Printf("Created default configuration file: %s\n", configPath)
	return cfg, nil
}

func runMigrations(cfg *Config, verbose bool, dryRun bool) {
	if verbose {
		log.Println("Running migrations...")
	}

	// Open database connection
	db, err := sql.Open(cfg.Database.Driver, cfg.Database.ConnectionString)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer func() {
		if err = db.Close(); err != nil {
			log.Printf("failed to close database: %v", err)
		}
	}()

	// Create dialect
	dialect, err := dialects.New(cfg.Database.Driver, nil)
	if err != nil {
		log.Fatalf("Failed to create dialect: %v", err)
	}

	// Create migrator
	options := &migration.MigratorOptions{
		TableName: cfg.Migrations.TableName,
	}
	migrator := migration.NewMigrator(db, dialect, metadata.NewRegistry(), options)

	// Load migrations from directory
	migrations, err := loadMigrationsFromDirectory(cfg.Migrations.Directory)
	if err != nil {
		log.Fatalf("Failed to load migrations: %v", err)
	}

	if len(migrations) == 0 {
		fmt.Println("No migrations found")
		return
	}

	// Get pending migrations
	ctx := context.Background()
	pending, err := migrator.GetPendingMigrations(ctx, migrations)
	if err != nil {
		log.Fatalf("Failed to get pending migrations: %v", err)
	}

	if len(pending) == 0 {
		fmt.Println("No pending migrations")
		return
	}

	fmt.Printf("Found %d pending migration(s):\n", len(pending))
	for _, m := range pending {
		fmt.Printf("  - %s: %s\n", m.ID, m.Name)
	}

	if dryRun {
		fmt.Println("Dry run - migrations would be applied")
		return
	}

	// Apply migrations
	if err := migrator.ApplyMigrations(ctx, pending); err != nil {
		log.Fatalf("Failed to apply migrations: %v", err)
	}

	fmt.Printf("Successfully applied %d migration(s)\n", len(pending))
}

func generateMigration(cfg *Config, name string, verbose bool) {
	if name == "" {
		log.Fatal("Migration name is required")
	}

	if verbose {
		log.Printf("Generating migration: %s", name)
	}

	// Fix G301: Use 0750 instead of 0755 for better security
	if err := os.MkdirAll(cfg.Migrations.Directory, 0750); err != nil {
		log.Fatalf("Failed to create migrations directory: %v", err)
	}

	// Generate migration file
	timestamp := time.Now().Unix()
	filename := fmt.Sprintf("%d_%s.sql", timestamp, toSnakeCase(name))
	filepathJoined := filepath.Join(cfg.Migrations.Directory, filename)

	// Create comprehensive migration template
	template := generateMigrationTemplate(name)

	// Fix G306: Use 0600 instead of 0644 for better security
	if err := os.WriteFile(filepathJoined, []byte(template), 0600); err != nil {
		log.Fatalf("Failed to write migration file: %v", err)
	}

	fmt.Printf("Generated migration: %s\n", filepathJoined)
}

// generateMigrationTemplate creates a comprehensive production-ready migration template
func generateMigrationTemplate(name string) string {
	return fmt.Sprintf(`-- =====================================================
-- Migration: %s
-- Created: %s
-- Description: [TODO: Add a brief description of what this migration does]
-- =====================================================

-- Migration Checklist:
-- [ ] 1. Reviewed for backward compatibility
-- [ ] 2. Tested rollback in development environment
-- [ ] 3. Considered performance impact of schema changes
-- [ ] 4. Verified data integrity constraints
-- [ ] 5. Checked for naming convention compliance

-- =====================================================
-- UP MIGRATION
-- =====================================================

-- WARNING: Always ensure your Up migration can be safely rolled back
-- Consider impact on running applications and plan maintenance windows accordingly

-- Example: Create a new table
-- CREATE TABLE IF NOT EXISTS users (
--     id BIGINT PRIMARY KEY AUTOINCREMENT,
--     email VARCHAR(255) NOT NULL UNIQUE,
--     name VARCHAR(100) NOT NULL,
--     created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
--     updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
--     deleted_at DATETIME NULL,
--     
--     -- Constraints
--     CONSTRAINT chk_email_format CHECK (email LIKE '%%@%%'),
--     CONSTRAINT chk_name_length CHECK (LENGTH(name) >= 2)
-- );

-- Example: Add a new column (safe operation)
-- ALTER TABLE users ADD COLUMN phone VARCHAR(20) NULL;

-- Example: Create an index for performance
-- CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
-- CREATE INDEX IF NOT EXISTS idx_users_created_at ON users(created_at);

-- Example: Add foreign key relationship
-- ALTER TABLE posts ADD CONSTRAINT fk_posts_user_id 
--     FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- Example: Create join table for many-to-many relationships
-- CREATE TABLE IF NOT EXISTS user_roles (
--     user_id BIGINT NOT NULL,
--     role_id BIGINT NOT NULL,
--     created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
--     
--     PRIMARY KEY (user_id, role_id),
--     FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
--     FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE
-- );

-- Example: Rename column (requires multiple steps for SQLite compatibility)
-- Step 1: Add new column
-- ALTER TABLE users ADD COLUMN full_name VARCHAR(100);
-- Step 2: Copy data (handled in application code or data migration)
-- UPDATE users SET full_name = name WHERE full_name IS NULL;
-- Step 3: Remove old column (in separate migration for safety)

-- TODO: Add your up migration SQL here
-- Replace this comment with your actual migration code



-- =====================================================
-- DOWN MIGRATION (ROLLBACK)
-- =====================================================

-- WARNING: Rollback operations can cause data loss
-- Always backup production data before running rollbacks
-- Test rollback thoroughly in staging environment

-- Example: Drop table (DANGEROUS - causes data loss)
-- DROP TABLE IF EXISTS user_roles;

-- Example: Drop foreign key constraint
-- ALTER TABLE posts DROP CONSTRAINT IF EXISTS fk_posts_user_id;

-- Example: Drop index
-- DROP INDEX IF EXISTS idx_users_email;
-- DROP INDEX IF EXISTS idx_users_created_at;

-- Example: Remove column (DANGEROUS - causes data loss)
-- ALTER TABLE users DROP COLUMN IF EXISTS phone;

-- Example: Drop table (DANGEROUS - causes data loss)
-- DROP TABLE IF EXISTS users;

-- TODO: Add your down migration SQL here
-- Ensure this reverses EXACTLY what the up migration does
-- Order matters: reverse the order of up migration operations



-- =====================================================
-- NOTES AND BEST PRACTICES
-- =====================================================

-- Database Compatibility Notes:
-- - Use IF NOT EXISTS / IF EXISTS for idempotent operations
-- - VARCHAR vs TEXT: Use VARCHAR(n) for bounded strings, TEXT for unlimited
-- - AUTOINCREMENT vs SERIAL: Use appropriate syntax for your database
-- - DATETIME vs TIMESTAMP: Consider timezone handling requirements

-- Performance Considerations:
-- - Add indexes AFTER bulk data operations, not before
-- - Consider partitioning for very large tables
-- - Use partial indexes where appropriate
-- - Monitor query performance after schema changes

-- Data Safety:
-- - Always use transactions for multi-statement operations
-- - Test migrations on production-sized datasets
-- - Consider using feature flags for application changes
-- - Plan rollback strategy before applying migration

-- Common Patterns:
-- 1. Add column (nullable first, then make required in separate migration)
-- 2. Create lookup tables before referencing tables
-- 3. Add constraints after data cleanup
-- 4. Use soft deletes (deleted_at) instead of hard deletes
-- 5. Include audit fields (created_at, updated_at, created_by, updated_by)

-- Naming Conventions:
-- - Tables: snake_case, plural (users, user_roles)
-- - Columns: snake_case (first_name, created_at)
-- - Indexes: idx_table_column(s) (idx_users_email)
-- - Foreign Keys: fk_table_referenced_table (fk_posts_users)
-- - Constraints: chk_table_condition (chk_users_email_format)

-- Multi-Database Support:
-- PostgreSQL specific: Use SERIAL, BIGSERIAL for auto-increment
-- MySQL specific: Use AUTO_INCREMENT, ENGINE=InnoDB
-- SQLite specific: Use AUTOINCREMENT, limited ALTER TABLE support
-- SQL Server specific: Use IDENTITY(1,1), nvarchar for Unicode

-- Example Database-Specific Variations:
-- PostgreSQL:
-- CREATE TABLE users (
--     id BIGSERIAL PRIMARY KEY,
--     email VARCHAR(255) NOT NULL UNIQUE,
--     created_at TIMESTAMP NOT NULL DEFAULT NOW()
-- );

-- MySQL:
-- CREATE TABLE users (
--     id BIGINT AUTO_INCREMENT PRIMARY KEY,
--     email VARCHAR(255) NOT NULL UNIQUE,
--     created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
-- ) ENGINE=InnoDB;

-- SQLite:
-- CREATE TABLE users (
--     id INTEGER PRIMARY KEY AUTOINCREMENT,
--     email TEXT NOT NULL UNIQUE,
--     created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
-- );

-- SQL Server:
-- CREATE TABLE users (
--     id BIGINT IDENTITY(1,1) PRIMARY KEY,
--     email NVARCHAR(255) NOT NULL UNIQUE,
--     created_at DATETIME2 NOT NULL DEFAULT GETUTCDATE()
-- );

-- End of Migration Template
`, name, time.Now().Format("2006-01-02 15:04:05"))
}

func showMigrationStatus(cfg *Config, verbose bool) {
	if verbose {
		log.Println("Checking migration status...")
	}

	// Open database connection
	db, err := sql.Open(cfg.Database.Driver, cfg.Database.ConnectionString)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer func() {
		if err = db.Close(); err != nil {
			log.Printf("failed to close database: %v", err)
		}
	}()

	// Create dialect
	dialect, err := dialects.New(cfg.Database.Driver, nil)
	if err != nil {
		log.Fatalf("Failed to create dialect: %v", err)
	}

	// Create migrator
	options := &migration.MigratorOptions{
		TableName: cfg.Migrations.TableName,
	}
	migrator := migration.NewMigrator(db, dialect, metadata.NewRegistry(), options)

	// Load migrations from directory
	migrations, err := loadMigrationsFromDirectory(cfg.Migrations.Directory)
	if err != nil {
		log.Fatalf("Failed to load migrations: %v", err)
	}

	// Get applied migrations
	ctx := context.Background()
	applied, err := migrator.GetAppliedMigrations(ctx)
	if err != nil {
		log.Fatalf("Failed to get applied migrations: %v", err)
	}

	// Get pending migrations
	pending, err := migrator.GetPendingMigrations(ctx, migrations)
	if err != nil {
		log.Fatalf("Failed to get pending migrations: %v", err)
	}

	// Display status
	fmt.Printf("Migration Status:\n")
	fmt.Printf("  Total migrations: %d\n", len(migrations))
	fmt.Printf("  Applied: %d\n", len(applied))
	fmt.Printf("  Pending: %d\n", len(pending))
	fmt.Println()

	if len(applied) > 0 {
		fmt.Println("Applied migrations:")
		for _, m := range applied {
			fmt.Printf("  ✓ %s: %s (applied %s)\n",
				m.ID,
				m.Name,
				m.AppliedAt.Format("2006-01-02 15:04:05"))
		}
		fmt.Println()
	}

	if len(pending) > 0 {
		fmt.Println("Pending migrations:")
		for _, m := range pending {
			fmt.Printf("  - %s: %s\n", m.ID, m.Name)
		}
	}
}

func rollbackMigration(cfg *Config, target string, verbose bool, dryRun bool) {
	if target == "" {
		log.Fatal("Target migration is required")
	}

	if verbose {
		log.Printf("Rolling back migration: %s", target)
	}

	// Open database connection
	db, err := sql.Open(cfg.Database.Driver, cfg.Database.ConnectionString)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer func() {
		if err = db.Close(); err != nil {
			log.Printf("failed to close database: %v", err)
		}
	}()

	// Create dialect
	dialect, err := dialects.New(cfg.Database.Driver, nil)
	if err != nil {
		log.Fatalf("Failed to create dialect: %v", err)
	}

	// Create migrator
	options := &migration.MigratorOptions{
		TableName: cfg.Migrations.TableName,
	}
	migrator := migration.NewMigrator(db, dialect, metadata.NewRegistry(), options)

	// Load migrations from directory
	migrations, err := loadMigrationsFromDirectory(cfg.Migrations.Directory)
	if err != nil {
		log.Fatalf("Failed to load migrations: %v", err)
	}

	// Find target migration
	var targetMigration *migration.Migration
	for _, m := range migrations {
		if m.ID == target {
			targetMigration = &m
			break
		}
	}

	if targetMigration == nil {
		log.Fatalf("Migration not found: %s", target)
	}

	ctx := context.Background()

	if dryRun {
		fmt.Printf("Dry run - would rollback migration: %s\n", target)
		return
	}

	// Rollback the specific migration using the correct method signature
	if err := migrator.RollbackMigration(ctx, *targetMigration); err != nil {
		log.Fatalf("Failed to rollback migration: %v", err)
	}

	fmt.Printf("Successfully rolled back migration: %s\n", target)
}

func loadMigrationsFromDirectory(dir string) ([]migration.Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []migration.Migration{}, nil
		}
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var migrations []migration.Migration

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		join := filepath.Join(dir, entry.Name())
		m, err := parseMigrationFile(join)
		if err != nil {
			return nil, fmt.Errorf("failed to parse migration file %s: %w", entry.Name(), err)
		}

		migrations = append(migrations, m)
	}

	// Sort migrations by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

func parseMigrationFile(filepath string) (migration.Migration, error) {
	// Validate migration file path before use (fixes G304)
	if err := validateMigrationPath(filepath); err != nil {
		return migration.Migration{}, fmt.Errorf("invalid migration file path: %w", err)
	}

	// #nosec G304 - path has been validated
	data, err := os.ReadFile(filepath)
	if err != nil {
		return migration.Migration{}, fmt.Errorf("failed to read file: %w", err)
	}

	content := string(data)

	// Extract migration ID and name from filename
	filename := path.Base(filepath)
	parts := strings.SplitN(filename, "_", 2)
	if len(parts) != 2 {
		return migration.Migration{}, fmt.Errorf("invalid migration filename format")
	}

	version, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return migration.Migration{}, fmt.Errorf("invalid version in filename: %w", err)
	}

	name := strings.TrimSuffix(parts[1], ".sql")
	id := strings.TrimSuffix(filename, ".sql")

	// Parse up and down SQL
	upSQL, downSQL := parseSQLSections(content)

	return migration.Migration{
		ID:          id,
		Name:        name,
		Version:     version,
		UpSQL:       upSQL,
		DownSQL:     downSQL,
		Description: fmt.Sprintf("Migration: %s", name),
		Checksum:    calculateChecksum(content),
	}, nil
}

func parseSQLSections(content string) (string, string) {
	lines := strings.Split(content, "\n")

	var upSQL, downSQL strings.Builder
	section := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "-- Up") {
			section = "up"
			continue
		}

		if strings.HasPrefix(line, "-- Down") {
			section = "down"
			continue
		}

		if strings.HasPrefix(line, "--") {
			continue
		}

		switch section {
		case "up":
			upSQL.WriteString(line)
			upSQL.WriteString("\n")
		case "down":
			downSQL.WriteString(line)
			downSQL.WriteString("\n")
		}
	}

	return strings.TrimSpace(upSQL.String()), strings.TrimSpace(downSQL.String())
}

func calculateChecksum(content string) string {
	// Simple checksum - in production, use a proper hash
	return fmt.Sprintf("%x", len(content))
}

func toSnakeCase(s string) string {
	var result []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '_')
		}
		result = append(result, unicode.ToLower(r))
	}
	return string(result)
}
