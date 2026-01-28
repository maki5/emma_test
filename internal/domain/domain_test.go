package domain

import (
	"testing"
)

func TestIsValidRole(t *testing.T) {
	tests := []struct {
		role  string
		valid bool
	}{
		{"admin", true},
		{"user", true},
		{"moderator", true},
		{"invalid", false},
		{"", false},
		{"ADMIN", false},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			if got := IsValidRole(tt.role); got != tt.valid {
				t.Errorf("IsValidRole(%q) = %v, want %v", tt.role, got, tt.valid)
			}
		})
	}
}

func TestIsValidStatus(t *testing.T) {
	tests := []struct {
		status string
		valid  bool
	}{
		{"draft", true},
		{"published", true},
		{"archived", true},
		{"invalid", false},
		{"", false},
		{"DRAFT", false},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if got := IsValidStatus(tt.status); got != tt.valid {
				t.Errorf("IsValidStatus(%q) = %v, want %v", tt.status, got, tt.valid)
			}
		})
	}
}

func TestIsValidResourceType(t *testing.T) {
	tests := []struct {
		resourceType string
		valid        bool
	}{
		{"users", true},
		{"articles", true},
		{"comments", true},
		{"invalid", false},
		{"", false},
		{"USERS", false},
	}

	for _, tt := range tests {
		t.Run(tt.resourceType, func(t *testing.T) {
			if got := IsValidResourceType(tt.resourceType); got != tt.valid {
				t.Errorf("IsValidResourceType(%q) = %v, want %v", tt.resourceType, got, tt.valid)
			}
		})
	}
}

func TestIsValidFormat(t *testing.T) {
	tests := []struct {
		format string
		valid  bool
	}{
		{"csv", true},
		{"ndjson", true},
		{"invalid", false},
		{"", false},
		{"CSV", false},
		{"json", false},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			if got := IsValidFormat(tt.format); got != tt.valid {
				t.Errorf("IsValidFormat(%q) = %v, want %v", tt.format, got, tt.valid)
			}
		})
	}
}

func TestValidRoles(t *testing.T) {
	expectedRoles := []string{"admin", "user", "moderator"}

	if len(ValidRoles) != len(expectedRoles) {
		t.Errorf("ValidRoles has %d elements, expected %d", len(ValidRoles), len(expectedRoles))
	}

	for _, role := range expectedRoles {
		found := false
		for _, r := range ValidRoles {
			if r == role {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ValidRoles missing %q", role)
		}
	}
}

func TestValidStatuses(t *testing.T) {
	expectedStatuses := []string{"draft", "published", "archived"}

	if len(ValidStatuses) != len(expectedStatuses) {
		t.Errorf("ValidStatuses has %d elements, expected %d", len(ValidStatuses), len(expectedStatuses))
	}

	for _, status := range expectedStatuses {
		found := false
		for _, s := range ValidStatuses {
			if s == status {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ValidStatuses missing %q", status)
		}
	}
}
