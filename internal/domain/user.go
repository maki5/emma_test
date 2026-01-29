package domain

import "time"

// User represents a user entity in the system.
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	Active    bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ValidRoles contains all valid user roles.
// This is the single source of truth for role validation across the application.
var ValidRoles = []string{"admin", "user", "moderator"}
