package validator

import (
	"regexp"
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"

	"bulk-import-export/internal/domain"
)

var (
	slugRegex   = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	validRoles  = []interface{}{"admin", "user", "moderator"}
	validStatus = []interface{}{"draft", "published", "archived"}
)

// Validator provides validation methods for domain entities.
type Validator struct{}

// NewValidator creates a new Validator instance.
func NewValidator() *Validator {
	return &Validator{}
}

// ValidateUser validates a User entity.
func (v *Validator) ValidateUser(u *domain.User) error {
	return validation.ValidateStruct(u,
		validation.Field(&u.Email,
			validation.Required.Error("email_required"),
			is.Email.Error("invalid_email_format"),
		),
		validation.Field(&u.Name,
			validation.Required.Error("name_required"),
		),
		validation.Field(&u.Role,
			validation.Required.Error("role_required"),
			validation.In(validRoles...).Error("invalid_role"),
		),
	)
}

// ValidateArticle validates an Article entity.
func (v *Validator) ValidateArticle(a *domain.Article) error {
	err := validation.ValidateStruct(a,
		validation.Field(&a.Slug,
			validation.Required.Error("slug_required"),
			validation.Match(slugRegex).Error("invalid_slug_format"),
		),
		validation.Field(&a.Title,
			validation.Required.Error("title_required"),
		),
		validation.Field(&a.Body,
			validation.Required.Error("body_required"),
		),
		validation.Field(&a.AuthorID,
			validation.Required.Error("author_id_required"),
		),
		validation.Field(&a.Status,
			validation.Required.Error("status_required"),
			validation.In(validStatus...).Error("invalid_status"),
		),
	)
	if err != nil {
		return err
	}

	// Custom rule: draft cannot have published_at
	if a.Status == "draft" && a.PublishedAt != nil {
		return validation.Errors{
			"published_at": validation.NewError("draft_cannot_have_published_at", "draft articles cannot have published_at"),
		}
	}

	// Custom rule: published must have published_at
	if a.Status == "published" && a.PublishedAt == nil {
		return validation.Errors{
			"published_at": validation.NewError("published_requires_published_at", "published articles must have published_at"),
		}
	}

	return nil
}

// ValidateComment validates a Comment entity.
func (v *Validator) ValidateComment(c *domain.Comment) error {
	return validation.ValidateStruct(c,
		validation.Field(&c.Body,
			validation.Required.Error("body_required"),
			validation.By(wordCountRule(500)),
		),
		validation.Field(&c.ArticleID,
			validation.Required.Error("article_id_required"),
		),
		validation.Field(&c.UserID,
			validation.Required.Error("user_id_required"),
		),
	)
}

// wordCountRule creates a validation rule for max word count.
func wordCountRule(maxWords int) validation.RuleFunc {
	return func(value interface{}) error {
		s, ok := value.(string)
		if !ok {
			return nil
		}
		wordCount := len(strings.Fields(strings.TrimSpace(s)))
		if wordCount > maxWords {
			return validation.NewError("body_exceeds_500_words", "body exceeds 500 words")
		}
		return nil
	}
}

// ConvertValidationErrors converts ozzo validation errors to domain RecordErrors.
func ConvertValidationErrors(rowNum int, err error) []domain.RecordError {
	var errors []domain.RecordError

	if ve, ok := err.(validation.Errors); ok {
		for field, fieldErr := range ve {
			errors = append(errors, domain.RecordError{
				Row:    rowNum,
				Field:  field,
				Reason: fieldErr.Error(),
			})
		}
	} else if err != nil {
		errors = append(errors, domain.RecordError{
			Row:    rowNum,
			Field:  "unknown",
			Reason: err.Error(),
		})
	}

	return errors
}
