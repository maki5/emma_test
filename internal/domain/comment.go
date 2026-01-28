package domain

import "time"

// Comment represents a comment entity in the system.
type Comment struct {
	ID        string    `json:"id"`
	Body      string    `json:"body"`
	ArticleID string    `json:"article_id"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}
