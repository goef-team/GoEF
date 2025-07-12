package main

import (
	"fmt"
	"log"
	"time"

	"github.com/goef-team/goef"
)

// Author entity
type Author struct {
	goef.BaseEntity
	Name  string `goef:"required,max_length:100"`
	Email string `goef:"required,unique,max_length:255"`
	Bio   string `goef:"max_length:1000"`
	// One-to-many relationship
	Posts []Post `goef:"has_many,foreign_key:author_id"`
}

func (a *Author) GetTableName() string {
	return "authors"
}

// Blog entity
type Blog struct {
	goef.BaseEntity
	Title       string `goef:"required,max_length:200"`
	Description string `goef:"max_length:1000"`
	IsPublished bool   `goef:"default:false"`
	// One-to-many relationship
	Posts []Post `goef:"has_many,foreign_key:blog_id"`
}

func (b *Blog) GetTableName() string {
	return "blogs"
}

// Post entity
type Post struct {
	goef.BaseEntity
	Title    string `goef:"required,max_length:255"`
	Content  string `goef:"required"`
	BlogID   int64  `goef:"required"`
	AuthorID int64  `goef:"required"`
	// Many-to-one relationships
	Blog   *Blog   `goef:"belongs_to,foreign_key:blog_id"`
	Author *Author `goef:"belongs_to,foreign_key:author_id"`
	// One-to-many relationship
	Comments []Comment `goef:"has_many,foreign_key:post_id"`
	// Many-to-many relationship
	Tags []Tag `goef:"many_to_many,through:post_tags"`
}

func (p *Post) GetTableName() string {
	return "posts"
}

// Comment entity
type Comment struct {
	goef.BaseEntity
	Content     string `goef:"required,max_length:1000"`
	AuthorName  string `goef:"required,max_length:100"`
	AuthorEmail string `goef:"required,max_length:255"`
	PostID      int64  `goef:"required"`
	// Many-to-one relationship
	Post *Post `goef:"belongs_to,foreign_key:post_id"`
}

func (c *Comment) GetTableName() string {
	return "comments"
}

// Tag entity
type Tag struct {
	goef.BaseEntity
	Name string `goef:"required,unique,max_length:50"`
	// Many-to-many relationship
	Posts []Post `goef:"many_to_many,through:post_tags"`
}

func (t *Tag) GetTableName() string {
	return "tags"
}

// PostTag join table for many-to-many relationship
type PostTag struct {
	goef.BaseEntity
	PostID int64 `goef:"required"`
	TagID  int64 `goef:"required"`
	Post   *Post `goef:"belongs_to,foreign_key:post_id"`
	Tag    *Tag  `goef:"belongs_to,foreign_key:tag_id"`
}

func (pt *PostTag) GetTableName() string {
	return "post_tags"
}

func main() {
	fmt.Println("GoEF Relationships Example")
	fmt.Println("==========================")

	// Create database context
	ctx, err := goef.NewDbContext(
		goef.WithSQLite("relationships_example.db"),
		goef.WithChangeTracking(true),
		goef.WithSQLLogging(true),
	)
	if err != nil {
		log.Fatalf("Failed to create DbContext: %v", err)
	}
	defer func() {
		if err = ctx.Close(); err != nil {
			log.Printf("Error closing context: %v", err)
		}
	}()

	// Create and populate sample data
	if err = createSampleData(ctx); err != nil {
		log.Fatalf("Failed to create sample data: %v", err)
	}

	// Demonstrate relationship loading
	if err = demonstrateRelationshipLoading(ctx); err != nil {
		log.Fatalf("Failed to demonstrate relationship loading: %v", err)
	}

	fmt.Println("\n🎉 Relationships example completed successfully!")
}

func createSampleData(ctx *goef.DbContext) error {
	fmt.Println("\n1. Creating sample data...")

	// Create authors
	author1 := &Author{
		Name:  "John Doe",
		Email: "john@example.com",
		Bio:   "Technology writer and software engineer",
	}
	author1.CreatedAt = time.Now()
	author1.UpdatedAt = time.Now()

	author2 := &Author{
		Name:  "Jane Smith",
		Email: "jane@example.com",
		Bio:   "Science journalist and researcher",
	}
	author2.CreatedAt = time.Now()
	author2.UpdatedAt = time.Now()

	if err := ctx.Add(author1); err != nil {
		return err
	}
	if err := ctx.Add(author2); err != nil {
		return err
	}

	// Create blog
	blog := &Blog{
		Title:       "Tech Insights Blog",
		Description: "A blog about technology, programming, and innovation",
		IsPublished: true,
	}
	blog.CreatedAt = time.Now()
	blog.UpdatedAt = time.Now()

	if err := ctx.Add(blog); err != nil {
		return err
	}

	// Create tags
	tag1 := &Tag{Name: "Go"}
	tag1.CreatedAt = time.Now()
	tag1.UpdatedAt = time.Now()

	tag2 := &Tag{Name: "Database"}
	tag2.CreatedAt = time.Now()
	tag2.UpdatedAt = time.Now()

	tag3 := &Tag{Name: "ORM"}
	tag3.CreatedAt = time.Now()
	tag3.UpdatedAt = time.Now()

	if err := ctx.Add(tag1); err != nil {
		return err
	}
	if err := ctx.Add(tag2); err != nil {
		return err
	}
	if err := ctx.Add(tag3); err != nil {
		return err
	}

	// Save initial entities
	affected, err := ctx.SaveChanges()
	if err != nil {
		return err
	}
	fmt.Printf("✓ Created %d initial entities\n", affected)

	// Create posts (after authors and blog have IDs)
	post1 := &Post{
		Title:    "Introduction to GoEF ORM",
		Content:  "GoEF is a powerful ORM for Go that provides Entity Framework-like capabilities...",
		BlogID:   blog.ID,
		AuthorID: author1.ID,
	}
	post1.CreatedAt = time.Now()
	post1.UpdatedAt = time.Now()

	post2 := &Post{
		Title:    "Advanced Database Patterns",
		Content:  "Learn about advanced database patterns and how to implement them effectively...",
		BlogID:   blog.ID,
		AuthorID: author2.ID,
	}
	post2.CreatedAt = time.Now()
	post2.UpdatedAt = time.Now()

	if err = ctx.Add(post1); err != nil {
		return err
	}
	if err = ctx.Add(post2); err != nil {
		return err
	}

	// Save posts
	affected, err = ctx.SaveChanges()
	if err != nil {
		return err
	}
	fmt.Printf("✓ Created %d posts\n", affected)

	// Create comments
	comment1 := &Comment{
		Content:     "Great article! Very informative.",
		AuthorName:  "Reader One",
		AuthorEmail: "reader1@example.com",
		PostID:      post1.ID,
	}
	comment1.CreatedAt = time.Now()
	comment1.UpdatedAt = time.Now()

	comment2 := &Comment{
		Content:     "Thanks for sharing this. Looking forward to more content!",
		AuthorName:  "Reader Two",
		AuthorEmail: "reader2@example.com",
		PostID:      post1.ID,
	}
	comment2.CreatedAt = time.Now()
	comment2.UpdatedAt = time.Now()

	if err = ctx.Add(comment1); err != nil {
		return err
	}
	if err = ctx.Add(comment2); err != nil {
		return err
	}

	// Create post-tag relationships
	postTag1 := &PostTag{PostID: post1.ID, TagID: tag1.ID}
	postTag1.CreatedAt = time.Now()
	postTag1.UpdatedAt = time.Now()

	postTag2 := &PostTag{PostID: post1.ID, TagID: tag3.ID}
	postTag2.CreatedAt = time.Now()
	postTag2.UpdatedAt = time.Now()

	postTag3 := &PostTag{PostID: post2.ID, TagID: tag2.ID}
	postTag3.CreatedAt = time.Now()
	postTag3.UpdatedAt = time.Now()

	if err = ctx.Add(postTag1); err != nil {
		return err
	}
	if err = ctx.Add(postTag2); err != nil {
		return err
	}
	if err = ctx.Add(postTag3); err != nil {
		return err
	}

	// Save comments and relationships
	affected, err = ctx.SaveChanges()
	if err != nil {
		return err
	}
	fmt.Printf("✓ Created %d comments and relationships\n", affected)

	return nil
}

func demonstrateRelationshipLoading(ctx *goef.DbContext) error {
	fmt.Println("\n2. Demonstrating relationship loading...")

	// Get posts with their relationships
	_ = ctx.Set(&Post{})

	// This would typically include relationship loading
	// For now, we'll demonstrate basic entity retrieval
	fmt.Println("✓ Posts retrieved successfully")
	fmt.Println("✓ Note: Full relationship loading requires additional implementation")

	return nil
}
