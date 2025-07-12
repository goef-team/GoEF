package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/goef-team/goef"
	"github.com/goef-team/goef/dialects"
	"github.com/goef-team/goef/loader"
	"github.com/goef-team/goef/migration"
	"github.com/goef-team/goef/query"
)

// Domain entities
type Blog struct {
	goef.BaseEntity
	Title       string `goef:"required,max_length:200"`
	Description string `goef:"max_length:500"`
	IsPublished bool   `goef:"default:false"`
	Posts       []Post `goef:"relationship:one_to_many,foreign_key:blog_id"`
}

func (b *Blog) GetTableName() string {
	return "blogs"
}

type Post struct {
	goef.BaseEntity
	Title    string    `goef:"required,max_length:200"`
	Content  string    `goef:"required"`
	BlogID   int64     `goef:"required"`
	AuthorID int64     `goef:"required"`
	Blog     *Blog     `goef:"relationship:many_to_one,foreign_key:blog_id"`
	Author   *Author   `goef:"relationship:many_to_one,foreign_key:author_id"`
	Tags     []Tag     `goef:"relationship:many_to_many,join_table:post_tags"`
	Comments []Comment `goef:"relationship:one_to_many,foreign_key:post_id"`
}

func (p *Post) GetTableName() string {
	return "posts"
}

type Author struct {
	goef.BaseEntity
	Name  string `goef:"required,max_length:100"`
	Email string `goef:"required,unique,max_length:255"`
	Bio   string `goef:"max_length:1000"`
	Posts []Post `goef:"relationship:one_to_many,foreign_key:author_id"`
}

func (a *Author) GetTableName() string {
	return "authors"
}

type Tag struct {
	goef.BaseEntity
	Name  string `goef:"required,unique,max_length:50"`
	Posts []Post `goef:"relationship:many_to_many,join_table:post_tags"`
}

func (t *Tag) GetTableName() string {
	return "tags"
}

type Comment struct {
	goef.BaseEntity
	Content     string `goef:"required,max_length:1000"`
	AuthorName  string `goef:"required,max_length:100"`
	AuthorEmail string `goef:"required,max_length:255"`
	PostID      int64  `goef:"required"`
	Post        *Post  `goef:"relationship:many_to_one,foreign_key:post_id"`
}

func (c *Comment) GetTableName() string {
	return "comments"
}

func main() {
	// Create database context
	ctx, err := goef.NewDbContext(
		goef.WithSQLite("advanced_example.db"),
		goef.WithChangeTracking(true),
		goef.WithSQLLogging(true),
		goef.WithAutoMigrate(true),
	)
	if err != nil {
		log.Fatal("Failed to create context:", err)
	}
	defer func() {
		if err := ctx.Close(); err != nil {
			log.Printf("failed to close context: %v", err)
		}
	}()

	// Run migrations
	if err := runMigrations(ctx); err != nil {
		log.Fatal("Failed to run migrations:", err)
	}

	// Demonstrate advanced features
	if err := demonstrateAdvancedFeatures(ctx); err != nil {
		log.Fatal("Failed to demonstrate features:", err)
	}
}

func runMigrations(ctx *goef.DbContext) error {
	fmt.Println("Running migrations...")

	// Create migrator with comprehensive options
	options := &migration.MigratorOptions{
		TableName:          "__goef_migrations",
		LockTableName:      "__goef_migration_lock",
		LockTimeout:        time.Minute * 2,
		LogVerbose:         true,
		DryRun:             false,
		TransactionMode:    true,
		ChecksumValidation: true,
	}

	migrator := migration.NewMigrator(ctx.DB(), ctx.Dialect(), ctx.Metadata(), options)

	// Initialize migration system (creates tables if needed)
	if err := migrator.Initialize(context.Background()); err != nil {
		return fmt.Errorf("failed to initialize migrator: %w", err)
	}

	// Define all your migrations
	migrations := []migration.Migration{
		{
			ID:      "001_initial_schema",
			Name:    "Initial Schema Creation",
			Version: 1,
			UpSQL: `
				CREATE TABLE IF NOT EXISTS blogs (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					title TEXT NOT NULL,
					description TEXT,
					is_published BOOLEAN NOT NULL DEFAULT 0,
					created_at DATETIME NOT NULL,
					updated_at DATETIME NOT NULL,
					deleted_at DATETIME
				);

				CREATE TABLE IF NOT EXISTS authors (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					name TEXT NOT NULL,
					email TEXT NOT NULL UNIQUE,
					bio TEXT,
					created_at DATETIME NOT NULL,
					updated_at DATETIME NOT NULL,
					deleted_at DATETIME
				);

				CREATE TABLE IF NOT EXISTS posts (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					title TEXT NOT NULL,
					content TEXT NOT NULL,
					blog_id INTEGER NOT NULL,
					author_id INTEGER NOT NULL,
					created_at DATETIME NOT NULL,
					updated_at DATETIME NOT NULL,
					deleted_at DATETIME,
					FOREIGN KEY (blog_id) REFERENCES blogs(id),
					FOREIGN KEY (author_id) REFERENCES authors(id)
				);

				CREATE TABLE IF NOT EXISTS tags (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					name TEXT NOT NULL UNIQUE,
					created_at DATETIME NOT NULL,
					updated_at DATETIME NOT NULL,
					deleted_at DATETIME
				);

				CREATE TABLE IF NOT EXISTS comments (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					content TEXT NOT NULL,
					author_name TEXT NOT NULL,
					author_email TEXT NOT NULL,
					post_id INTEGER NOT NULL,
					created_at DATETIME NOT NULL,
					updated_at DATETIME NOT NULL,
					deleted_at DATETIME,
					FOREIGN KEY (post_id) REFERENCES posts(id)
				);

				CREATE TABLE IF NOT EXISTS post_tags (
					post_id INTEGER NOT NULL,
					tag_id INTEGER NOT NULL,
					PRIMARY KEY (post_id, tag_id),
					FOREIGN KEY (post_id) REFERENCES posts(id),
					FOREIGN KEY (tag_id) REFERENCES tags(id)
				);
			`,
			DownSQL: `
				DROP TABLE IF EXISTS post_tags;
				DROP TABLE IF EXISTS comments;
				DROP TABLE IF EXISTS tags;
				DROP TABLE IF EXISTS posts;
				DROP TABLE IF EXISTS authors;
				DROP TABLE IF EXISTS blogs;
			`,
			Description: "Creates initial database schema with all tables and relationships",
			CreatedAt:   time.Now(),
		},
		{
			ID:      "002_add_indexes",
			Name:    "Add Performance Indexes",
			Version: 2,
			UpSQL: `
				CREATE INDEX IF NOT EXISTS idx_posts_blog_id ON posts(blog_id);
				CREATE INDEX IF NOT EXISTS idx_posts_author_id ON posts(author_id);
				CREATE INDEX IF NOT EXISTS idx_comments_post_id ON comments(post_id);
				CREATE INDEX IF NOT EXISTS idx_posts_created_at ON posts(created_at);
				CREATE INDEX IF NOT EXISTS idx_authors_email ON authors(email);
			`,
			DownSQL: `
				DROP INDEX IF EXISTS idx_posts_blog_id;
				DROP INDEX IF EXISTS idx_posts_author_id;
				DROP INDEX IF EXISTS idx_comments_post_id;
				DROP INDEX IF EXISTS idx_posts_created_at;
				DROP INDEX IF EXISTS idx_authors_email;
			`,
			Description: "Adds performance indexes for common queries",
			CreatedAt:   time.Now(),
		},
	}

	// Apply all pending migrations (idempotent - safe to run multiple times)
	if err := migrator.ApplyMigrations(context.Background(), migrations); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	// Get and display migration status
	status, err := migrator.GetMigrationStatus(context.Background(), migrations)
	if err != nil {
		return fmt.Errorf("failed to get migration status: %w", err)
	}

	appliedCount := 0
	for _, s := range status {
		if s == migration.StatusApplied {
			appliedCount++
		}
	}

	fmt.Printf("✓ Migration system completed: %d/%d migrations applied\n", appliedCount, len(migrations))
	return nil
}
func demonstrateAdvancedFeatures(ctx *goef.DbContext) error {
	fmt.Println("\n=== Advanced GoEF Features Demo ===")

	// 1. Create sample data
	if err := createSampleData(ctx); err != nil {
		return fmt.Errorf("failed to create sample data: %w", err)
	}

	// 2. Demonstrate LINQ-style queries
	if err := demonstrateLINQQueries(ctx); err != nil {
		return fmt.Errorf("failed to demonstrate LINQ queries: %w", err)
	}

	// 3. Demonstrate relationship loading
	if err := demonstrateRelationshipLoading(ctx); err != nil {
		return fmt.Errorf("failed to demonstrate relationship loading: %w", err)
	}

	// 4. Demonstrate change tracking
	if err := demonstrateChangeTracking(ctx); err != nil {
		return fmt.Errorf("failed to demonstrate change tracking: %w", err)
	}

	// 5. Demonstrate transactions
	if err := demonstrateTransactions(ctx); err != nil {
		return fmt.Errorf("failed to demonstrate transactions: %w", err)
	}

	// 6. Demonstrate query compilation
	if err := demonstrateQueryCompilation(ctx); err != nil {
		return fmt.Errorf("failed to demonstrate query compilation: %w", err)
	}

	return nil
}

func createSampleData(ctx *goef.DbContext) error {
	fmt.Println("\n1. Creating sample data...")

	// Create authors
	authors := []*Author{
		{Name: "John Doe", Email: "john@example.com", Bio: "Technology writer", Posts: []Post{}},
		{Name: "Jane Smith", Email: "jane@example.com", Bio: "Science journalist", Posts: []Post{}},
	}

	for _, author := range authors {
		author.CreatedAt = time.Now()
		author.UpdatedAt = time.Now()
		if err := ctx.Add(author); err != nil {
			return err
		}
	}

	// Create blog
	blog := &Blog{
		Title:       "Tech Blog",
		Description: "A blog about technology and programming",
		IsPublished: true,
	}
	blog.CreatedAt = time.Now()
	blog.UpdatedAt = time.Now()
	if err := ctx.Add(blog); err != nil {
		return err
	}

	// Create tags
	tags := []*Tag{
		{Name: "Go"}, {Name: "Database"}, {Name: "ORM"},
	}

	for _, tag := range tags {
		tag.CreatedAt = time.Now()
		tag.UpdatedAt = time.Now()
		if err := ctx.Add(tag); err != nil {
			return err
		}
	}

	// Save all entities
	affected, err := ctx.SaveChanges()
	if err != nil {
		return err
	}

	fmt.Printf("✓ Created %d entities\n", affected)

	// Create posts (after authors and blog have IDs)
	posts := []*Post{
		{
			Title:    "Introduction to GoEF",
			Content:  "GoEF is a powerful ORM for Go...",
			BlogID:   blog.ID,
			AuthorID: authors[0].ID,
		},
		{
			Title:    "Advanced Database Patterns",
			Content:  "Learn about advanced database patterns...",
			BlogID:   blog.ID,
			AuthorID: authors[1].ID,
		},
	}

	for _, post := range posts {
		post.CreatedAt = time.Now()
		post.UpdatedAt = time.Now()
		if err = ctx.Add(post); err != nil {
			return err
		}
	}

	// Create comments
	comments := []*Comment{
		{
			Content:     "Great article!",
			AuthorName:  "Reader One",
			AuthorEmail: "reader1@example.com",
			PostID:      posts[0].ID,
		},
		{
			Content:     "Very informative, thank you!",
			AuthorName:  "Reader Two",
			AuthorEmail: "reader2@example.com",
			PostID:      posts[0].ID,
		},
	}

	for _, comment := range comments {
		comment.CreatedAt = time.Now()
		comment.UpdatedAt = time.Now()
		if err = ctx.Add(comment); err != nil {
			return err
		}
	}

	affected, err = ctx.SaveChanges()
	if err != nil {
		return err
	}

	fmt.Printf("✓ Created %d more entities\n", affected)
	return nil
}

func demonstrateLINQQueries(ctx *goef.DbContext) error {
	fmt.Println("\n2. Demonstrating LINQ-style queries...")

	// Get queryable for posts
	queryable := query.NewQueryable[Post](ctx.Dialect(), ctx.Metadata(), ctx.DB())

	// Example 1: Simple where clause
	recentPosts, err := queryable.
		WhereExpr(query.GreaterThanEx(func(p Post) interface{} { return p.CreatedAt }, time.Now().Add(-24*time.Hour))).
		OrderBy("Created_At").
		ToList()
	if err != nil {
		return err
	}
	fmt.Printf("✓ Found %d recent posts\n", len(recentPosts))

	// Example 2: Complex query with multiple conditions
	publishedPosts, err := queryable.
		WhereExpr(query.EqualEx(func(p Post) interface{} { return p.Blog.IsPublished }, true)).
		Take(10).
		ToList()
	if err != nil {
		return err
	}
	fmt.Printf("✓ Found %d published posts\n", len(publishedPosts))

	// Example 3: Aggregation
	totalPosts, err := queryable.Count()
	if err != nil {
		return err
	}
	fmt.Printf("✓ Total posts: %d\n", totalPosts)

	// Example 4: First/Single operations
	firstPost, err := queryable.First()
	if err != nil {
		return err
	}
	fmt.Printf("✓ First post: %s\n", firstPost.Title)

	// Example 5: Any/All operations
	hasAnyPosts, err := queryable.Any()
	if err != nil {
		return err
	}
	fmt.Printf("✓ Has any posts: %v\n", hasAnyPosts)

	return nil
}

func demonstrateRelationshipLoading(ctx *goef.DbContext) error {
	fmt.Println("\n3. Demonstrating relationship loading...")

	// Create relationship loader
	relationLoader := loader.New(ctx.DB(), ctx.Dialect(), ctx.Metadata())

	// Load blog with posts
	blog := &Blog{
		BaseEntity: goef.BaseEntity{
			ID: 1,
		},
	}
	err := relationLoader.LoadRelation(context.Background(), blog, "Posts")
	if err != nil {
		return err
	}
	fmt.Printf("✓ Loaded blog with %d posts\n", len(blog.Posts))

	// FIXED: Load post with only specific relationships that we know work
	post := &Post{
		BaseEntity: goef.BaseEntity{
			ID: 1,
		},
		BlogID:   1, // FIXED: Set the foreign key values
		AuthorID: 1, // FIXED: Set the foreign key values
	}

	// Load individual relationships instead of all at once
	err = relationLoader.LoadRelation(context.Background(), post, "Author")
	if err != nil {
		fmt.Printf("Warning: Failed to load Author relationship: %v\n", err)
	} else {
		fmt.Printf("✓ Loaded post with author\n")
	}

	// Try to load Comments (one-to-many relationship)
	err = relationLoader.LoadRelation(context.Background(), post, "Comments")
	if err != nil {
		fmt.Printf("Warning: Failed to load Comments relationship: %v\n", err)
	} else {
		fmt.Printf("✓ Loaded post with %d comments\n", len(post.Comments))
	}

	// Demonstrate lazy loading
	lazyLoader := loader.NewLazyLoader(ctx.DB(), ctx.Dialect(), ctx.Metadata())

	author := &Author{
		BaseEntity: goef.BaseEntity{
			ID: 1,
		},
	}
	err = lazyLoader.Load(context.Background(), author, "Posts")
	if err != nil {
		fmt.Printf("Warning: Failed to lazy load Posts: %v\n", err)
	} else {
		fmt.Printf("✓ Lazy loaded author with %d posts\n", len(author.Posts))
	}

	// Check if loaded
	if lazyLoader.IsLoaded(author, "Posts") {
		fmt.Println("✓ Posts relationship is marked as loaded")
	}

	return nil
}

func demonstrateChangeTracking(ctx *goef.DbContext) error {
	fmt.Println("\n4. Demonstrating change tracking...")

	// Load an entity
	posts := ctx.Set(&Post{})
	post, err := posts.Find(1)
	if err != nil {
		return err
	}

	originalTitle := post.(*Post).Title
	fmt.Printf("✓ Loaded post: %s\n", originalTitle)

	// Modify the entity
	post.(*Post).Title = "Updated: " + originalTitle
	post.(*Post).Content = "Updated content with more information..."

	// Update in context
	err = ctx.Update(post)
	if err != nil {
		return err
	}

	// Check tracked changes
	if ctx.ChangeTracker().HasChanges() {
		changes := ctx.ChangeTracker().GetChanges()
		fmt.Printf("✓ Tracked %d changes\n", len(changes))

		for _, change := range changes {
			fmt.Printf("  - Entity state: %s\n", change.State)
			fmt.Printf("  - Modified properties: %v\n", change.ModifiedProperties)
		}
	}

	// Save changes
	affected, err := ctx.SaveChanges()
	if err != nil {
		return err
	}
	fmt.Printf("✓ Saved %d changes\n", affected)

	return nil
}

func demonstrateTransactions(ctx *goef.DbContext) error {
	fmt.Println("\n5. Demonstrating transactions...")

	// Begin transaction
	err := ctx.BeginTransaction()
	if err != nil {
		return err
	}

	// Create entities within transaction
	newAuthor := &Author{
		Name:  "Transaction Author",
		Email: "tx@example.com",
		Bio:   "Created within transaction",
	}
	newAuthor.CreatedAt = time.Now()
	newAuthor.UpdatedAt = time.Now()

	if err = ctx.Add(newAuthor); err != nil {
		return err
	}

	// Save changes within transaction
	affected, err := ctx.SaveChanges()
	if err != nil {
		_ = ctx.RollbackTransaction()
		return err
	}
	fmt.Printf("✓ Created %d entities in transaction\n", affected)

	// Commit transaction
	err = ctx.CommitTransaction()
	if err != nil {
		return err
	}
	fmt.Println("✓ Transaction committed successfully")

	// Demonstrate rollback
	err = ctx.BeginTransaction()
	if err != nil {
		return err
	}

	// Create and then rollback
	tempAuthor := &Author{
		Name:  "Temp Author",
		Email: "temp@example.com",
	}
	tempAuthor.CreatedAt = time.Now()
	tempAuthor.UpdatedAt = time.Now()

	if err = ctx.Add(tempAuthor); err != nil {
		return err
	}
	if _, err = ctx.SaveChanges(); err != nil {
		return err
	}

	// Rollback
	err = ctx.RollbackTransaction()
	if err != nil {
		return err
	}
	fmt.Println("✓ Transaction rolled back successfully")

	return nil
}

func demonstrateQueryCompilation(ctx *goef.DbContext) error {
	fmt.Println("\n6. Demonstrating query compilation...")

	// Create query compiler
	compiler := query.NewQueryCompiler(ctx.Dialect(), ctx.Metadata(), 100)

	// Compile a query
	testQuery := &dialects.SelectQuery{
		Table:   "posts",
		Columns: []string{"id", "title", "content"},
		Where: []dialects.WhereClause{
			{Column: "blog_id", Operator: "=", Value: 1},
		},
	}

	startTime := time.Now()
	compiled, err := compiler.Compile(&Post{}, testQuery)
	if err != nil {
		return err
	}
	compilationTime := time.Since(startTime)

	fmt.Printf("✓ Compiled query in %v\n", compilationTime)
	fmt.Printf("  - SQL: %s\n", compiled.SQL)
	fmt.Printf("  - Parameters: %d\n", compiled.ParamCount)

	// Compile same query again (should be cached)
	startTime = time.Now()
	compiled2, err := compiler.Compile(&Post{}, testQuery)
	if err != nil {
		return err
	}
	cacheTime := time.Since(startTime)

	fmt.Printf("✓ Retrieved from cache in %v\n", cacheTime)
	fmt.Printf("  - Access count: %d\n", compiled2.AccessCount)

	// Show cache statistics
	stats := compiler.GetCacheStats()
	fmt.Printf("✓ Cache stats: %v\n", stats)

	return nil
}
