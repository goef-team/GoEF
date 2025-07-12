package unit

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/goef-team/goef/dialects"
	"github.com/goef-team/goef/loader"
	"github.com/goef-team/goef/metadata"

	_ "github.com/mattn/go-sqlite3"
)

// Test entities for relationship loading
type Author struct {
	ID    int64  `goef:"primary_key,auto_increment"`
	Name  string `goef:"required"`
	Books []Book `goef:"relationship:one_to_many,foreign_key:author_id"`
}

func (a *Author) GetTableName() string {
	return "authors"
}

func (a *Author) GetID() interface{} {
	return a.ID
}

func (a *Author) SetID(id interface{}) {
	if v, ok := id.(int64); ok {
		a.ID = v
	}
}

type Book struct {
	ID       int64   `goef:"primary_key,auto_increment"`
	Title    string  `goef:"required"`
	AuthorID int64   `goef:"required"`
	Author   *Author `goef:"relationship:many_to_one,foreign_key:author_id"`
}

func (b *Book) GetTableName() string {
	return "books"
}

func (b *Book) GetID() interface{} {
	return b.ID
}

func (b *Book) SetID(id interface{}) {
	if v, ok := id.(int64); ok {
		b.ID = v
	}
}

func setupTestDatabase(t *testing.T) (*sql.DB, func()) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Create test tables
	_, err = db.Exec(`
		CREATE TABLE authors (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL
		);

		CREATE TABLE books (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			author_id INTEGER NOT NULL,
			FOREIGN KEY (author_id) REFERENCES authors(id)
		);
	`)
	if err != nil {
		t.Fatalf("Failed to create test tables: %v", err)
	}

	// Insert test data
	_, err = db.Exec(`
		INSERT INTO authors (name) VALUES ('J.K. Rowling'), ('George R.R. Martin');
		INSERT INTO books (title, author_id) VALUES 
			('Harry Potter and the Philosopher''s Stone', 1),
			('Harry Potter and the Chamber of Secrets', 1),
			('A Game of Thrones', 2);
	`)
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	cleanup := func() {
		err = db.Close()
		if err != nil {
			t.Fatalf("error on closing db, err: %v", err)
		}
	}

	return db, cleanup
}

func TestLoader_LoadRelation(t *testing.T) {
	db, cleanup := setupTestDatabase(t)
	defer cleanup()

	dialect := dialects.NewSQLiteDialect(nil)
	metadataRegistry := metadata.NewRegistry()

	relationLoader := loader.New(db, dialect, metadataRegistry)

	ctx := context.Background()

	// Create an author entity
	author := &Author{
		ID:   1,
		Name: "J.K. Rowling",
	}

	// Load the Books relationship
	err := relationLoader.LoadRelation(ctx, author, "Books")
	if err != nil {
		t.Fatalf("Failed to load Books relationship: %v", err)
	}

	// Check that books were loaded
	if len(author.Books) != 2 {
		t.Errorf("Expected 2 books, got %d", len(author.Books))
	}

	// Check book titles
	expectedTitles := []string{
		"Harry Potter and the Philosopher's Stone",
		"Harry Potter and the Chamber of Secrets",
	}

	for i, book := range author.Books {
		if i < len(expectedTitles) {
			if book.Title != expectedTitles[i] {
				t.Errorf("Expected book title %s, got %s", expectedTitles[i], book.Title)
			}
		}
	}
}

func TestLoader_LoadAllRelations(t *testing.T) {
	db, cleanup := setupTestDatabase(t)
	defer cleanup()

	dialect := dialects.NewSQLiteDialect(nil)
	metadataRegistry := metadata.NewRegistry()

	relationLoader := loader.New(db, dialect, metadataRegistry)

	ctx := context.Background()

	// Create an author entity
	author := &Author{
		ID:   1,
		Name: "J.K. Rowling",
	}

	// Load all relationships
	err := relationLoader.LoadAllRelations(ctx, author)
	if err != nil {
		t.Fatalf("Failed to load all relationships: %v", err)
	}

	// Check that books were loaded
	if len(author.Books) != 2 {
		t.Errorf("Expected 2 books, got %d", len(author.Books))
	}
}

func TestLazyLoader_Load(t *testing.T) {
	db, cleanup := setupTestDatabase(t)
	defer cleanup()

	dialect := dialects.NewSQLiteDialect(nil)
	metadataRegistry := metadata.NewRegistry()

	lazyLoader := loader.NewLazyLoader(db, dialect, metadataRegistry)

	ctx := context.Background()

	// Create an author entity
	author := &Author{
		ID:   1,
		Name: "J.K. Rowling",
	}

	// Check that relationship is not loaded initially
	if lazyLoader.IsLoaded(author, "Books") {
		t.Error("Expected Books relationship to not be loaded initially")
	}

	// Load the Books relationship
	err := lazyLoader.Load(ctx, author, "Books")
	if err != nil {
		t.Fatalf("Failed to lazy load Books relationship: %v", err)
	}

	// Check that relationship is now loaded
	if !lazyLoader.IsLoaded(author, "Books") {
		t.Error("Expected Books relationship to be loaded after Load call")
	}

	// Check that books were loaded
	if len(author.Books) != 2 {
		t.Errorf("Expected 2 books, got %d", len(author.Books))
	}

	// Load again - should not reload
	err = lazyLoader.Load(ctx, author, "Books")
	if err != nil {
		t.Fatalf("Failed to lazy load Books relationship second time: %v", err)
	}
}

func TestLoader_LoadSingleRelation(t *testing.T) {
	db, cleanup := setupTestDatabase(t)
	defer cleanup()

	dialect := dialects.NewSQLiteDialect(nil)
	metadataRegistry := metadata.NewRegistry()

	relationLoader := loader.New(db, dialect, metadataRegistry)

	ctx := context.Background()

	// Create a book entity
	book := &Book{
		ID:       1,
		Title:    "Harry Potter and the Philosopher's Stone",
		AuthorID: 1,
	}

	// Load the Author relationship
	err := relationLoader.LoadRelation(ctx, book, "Author")
	if err != nil {
		t.Fatalf("Failed to load Author relationship: %v", err)
	}

	// Check that author was loaded
	if book.Author == nil {
		t.Fatal("Expected Author to be loaded")
	}

	if book.Author.Name != "J.K. Rowling" {
		t.Errorf("Expected author name 'J.K. Rowling', got %s", book.Author.Name)
	}
}

func TestLoader_LoadNonExistentRelation(t *testing.T) {
	db, cleanup := setupTestDatabase(t)
	defer cleanup()

	dialect := dialects.NewSQLiteDialect(nil)
	metadataRegistry := metadata.NewRegistry()

	relationLoader := loader.New(db, dialect, metadataRegistry)

	ctx := context.Background()

	// Create an author entity
	author := &Author{
		ID:   1,
		Name: "J.K. Rowling",
	}

	// Try to load a non-existent relationship
	err := relationLoader.LoadRelation(ctx, author, "NonExistentRelation")
	if err == nil {
		t.Error("Expected error when loading non-existent relationship")
	}

	expectedError := "relationship 'NonExistentRelation' not found"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Expected error to contain '%s', got: %s", expectedError, err.Error())
	}
}
