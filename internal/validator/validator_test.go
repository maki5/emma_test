package validator

import (
	"strings"
	"testing"
	"time"

	"bulk-import-export/internal/domain"
)

func TestValidateUser(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		user    *domain.User
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid user",
			user: &domain.User{
				ID:     "123e4567-e89b-12d3-a456-426614174000",
				Email:  "test@example.com",
				Name:   "John Doe",
				Role:   "user",
				Active: true,
			},
			wantErr: false,
		},
		{
			name: "valid admin user",
			user: &domain.User{
				ID:     "123e4567-e89b-12d3-a456-426614174000",
				Email:  "admin@example.com",
				Name:   "Admin User",
				Role:   "admin",
				Active: true,
			},
			wantErr: false,
		},
		{
			name: "missing email",
			user: &domain.User{
				ID:   "123e4567-e89b-12d3-a456-426614174000",
				Name: "John Doe",
				Role: "user",
			},
			wantErr: true,
			errMsg:  "email",
		},
		{
			name: "invalid email format",
			user: &domain.User{
				ID:    "123e4567-e89b-12d3-a456-426614174000",
				Email: "invalid-email",
				Name:  "John Doe",
				Role:  "user",
			},
			wantErr: true,
			errMsg:  "email",
		},
		{
			name: "missing name",
			user: &domain.User{
				ID:    "123e4567-e89b-12d3-a456-426614174000",
				Email: "test@example.com",
				Role:  "user",
			},
			wantErr: true,
			errMsg:  "name",
		},
		{
			name: "invalid role",
			user: &domain.User{
				ID:    "123e4567-e89b-12d3-a456-426614174000",
				Email: "test@example.com",
				Name:  "John Doe",
				Role:  "invalid",
			},
			wantErr: true,
			errMsg:  "role",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateUser(tt.user)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUser() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateUser() error = %v, should contain %v", err, tt.errMsg)
				}
			}
		})
	}
}

func TestValidateArticle(t *testing.T) {
	v := NewValidator()
	now := time.Now()

	tests := []struct {
		name    string
		article *domain.Article
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid draft article",
			article: &domain.Article{
				ID:       "123e4567-e89b-12d3-a456-426614174000",
				Title:    "Test Article",
				Slug:     "test-article",
				Body:     "This is the article body.",
				AuthorID: "123e4567-e89b-12d3-a456-426614174001",
				Status:   "draft",
			},
			wantErr: false,
		},
		{
			name: "valid published article with published_at",
			article: &domain.Article{
				ID:          "123e4567-e89b-12d3-a456-426614174000",
				Title:       "Test Article",
				Slug:        "test-article-2",
				Body:        "This is the article body.",
				AuthorID:    "123e4567-e89b-12d3-a456-426614174001",
				Status:      "published",
				PublishedAt: &now,
			},
			wantErr: false,
		},
		{
			name: "missing title",
			article: &domain.Article{
				ID:       "123e4567-e89b-12d3-a456-426614174000",
				Slug:     "test-article",
				Body:     "This is the article body.",
				AuthorID: "123e4567-e89b-12d3-a456-426614174001",
				Status:   "draft",
			},
			wantErr: true,
			errMsg:  "title",
		},
		{
			name: "invalid slug format",
			article: &domain.Article{
				ID:       "123e4567-e89b-12d3-a456-426614174000",
				Title:    "Test Article",
				Slug:     "Invalid Slug With Spaces",
				Body:     "This is the article body.",
				AuthorID: "123e4567-e89b-12d3-a456-426614174001",
				Status:   "draft",
			},
			wantErr: true,
			errMsg:  "slug",
		},
		{
			name: "missing author_id",
			article: &domain.Article{
				ID:     "123e4567-e89b-12d3-a456-426614174000",
				Title:  "Test Article",
				Slug:   "test-article",
				Body:   "This is the article body.",
				Status: "draft",
			},
			wantErr: true,
			errMsg:  "author_id",
		},
		{
			name: "invalid status",
			article: &domain.Article{
				ID:       "123e4567-e89b-12d3-a456-426614174000",
				Title:    "Test Article",
				Slug:     "test-article",
				Body:     "This is the article body.",
				AuthorID: "123e4567-e89b-12d3-a456-426614174001",
				Status:   "invalid",
			},
			wantErr: true,
			errMsg:  "status",
		},
		{
			name: "published article without published_at",
			article: &domain.Article{
				ID:       "123e4567-e89b-12d3-a456-426614174000",
				Title:    "Test Article",
				Slug:     "test-article",
				Body:     "This is the article body.",
				AuthorID: "123e4567-e89b-12d3-a456-426614174001",
				Status:   "published",
			},
			wantErr: true,
			errMsg:  "published_at",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateArticle(tt.article)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateArticle() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateArticle() error = %v, should contain %v", err, tt.errMsg)
				}
			}
		})
	}
}

func TestValidateComment(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		comment *domain.Comment
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid comment",
			comment: &domain.Comment{
				ID:        "cm_123e4567-e89b-12d3-a456-426614174000",
				Body:      "This is a comment.",
				ArticleID: "123e4567-e89b-12d3-a456-426614174001",
				UserID:    "123e4567-e89b-12d3-a456-426614174002",
			},
			wantErr: false,
		},
		{
			name: "missing body",
			comment: &domain.Comment{
				ID:        "cm_123e4567-e89b-12d3-a456-426614174000",
				ArticleID: "123e4567-e89b-12d3-a456-426614174001",
				UserID:    "123e4567-e89b-12d3-a456-426614174002",
			},
			wantErr: true,
			errMsg:  "body",
		},
		{
			name: "missing article_id",
			comment: &domain.Comment{
				ID:     "cm_123e4567-e89b-12d3-a456-426614174000",
				Body:   "This is a comment.",
				UserID: "123e4567-e89b-12d3-a456-426614174002",
			},
			wantErr: true,
			errMsg:  "article_id",
		},
		{
			name: "missing user_id",
			comment: &domain.Comment{
				ID:        "cm_123e4567-e89b-12d3-a456-426614174000",
				Body:      "This is a comment.",
				ArticleID: "123e4567-e89b-12d3-a456-426614174001",
			},
			wantErr: true,
			errMsg:  "user_id",
		},
		{
			name: "body exceeds 500 words",
			comment: &domain.Comment{
				ID:        "cm_123e4567-e89b-12d3-a456-426614174000",
				Body:      strings.Repeat("word ", 501),
				ArticleID: "123e4567-e89b-12d3-a456-426614174001",
				UserID:    "123e4567-e89b-12d3-a456-426614174002",
			},
			wantErr: true,
			errMsg:  "body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateComment(tt.comment)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateComment() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateComment() error = %v, should contain %v", err, tt.errMsg)
				}
			}
		})
	}
}

func TestConvertValidationErrors(t *testing.T) {
	v := NewValidator()

	user := &domain.User{
		ID: "123",
	}

	err := v.ValidateUser(user)
	if err == nil {
		t.Fatal("expected validation error")
	}

	errors := ConvertValidationErrors(1, err)
	if len(errors) == 0 {
		t.Error("expected at least one error")
	}

	for _, e := range errors {
		if e.Row != 1 {
			t.Errorf("expected row 1, got %d", e.Row)
		}
		if e.Field == "" {
			t.Error("expected field to be set")
		}
		if e.Reason == "" {
			t.Error("expected reason to be set")
		}
	}
}
