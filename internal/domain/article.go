package domain

import "time"

// Article represents an article entity in the system.
type Article struct {
	ID          string     `json:"id"`
	Slug        string     `json:"slug"`
	Title       string     `json:"title"`
	Description *string    `json:"description,omitempty"`
	Body        string     `json:"body"`
	Tags        []string   `json:"tags,omitempty"`
	AuthorID    string     `json:"author_id"`
	Status      string     `json:"status"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// ValidStatuses contains all valid article statuses.
// This is the single source of truth for status validation across the application.
var ValidStatuses = []string{"draft", "published", "archived"}
