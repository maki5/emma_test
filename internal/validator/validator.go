package validator

import (
	"regexp"
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"

	"bulk-import-export/internal/domain"
)

var (
	slugRegex   = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	// Simple email regex - fast and avoids catastrophic backtracking
	// Matches: local@domain.tld format (at least 2 char TLD required)
	emailRegex  = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	// UUID format regex (standard UUID without prefix)
	uuidRegex   = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	// Comment ID regex: allows cm_ prefix followed by UUID
	commentIDRegex = regexp.MustCompile(`^cm_[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	// Pre-computed slices/maps for O(1) lookups using domain as source of truth
	validStatus   = buildValidStatusSlice()
	validRolesMap = buildValidRolesMap()
)

// buildValidRolesMap creates a map from domain.ValidRoles for O(1) lookups
func buildValidRolesMap() map[string]bool {
	m := make(map[string]bool, len(domain.ValidRoles))
	for _, role := range domain.ValidRoles {
		m[role] = true
	}
	return m
}

// buildValidStatusSlice creates an interface slice from domain.ValidStatuses for ozzo-validation
func buildValidStatusSlice() []interface{} {
	s := make([]interface{}, len(domain.ValidStatuses))
	for i, status := range domain.ValidStatuses {
		s[i] = status
	}
	return s
}

// Validator provides validation methods for domain entities.
type Validator struct{}

// NewValidator creates a new Validator instance.
func NewValidator() *Validator {
	return &Validator{}
}

// ValidateUser validates a User entity.
// Optimized to use direct map lookup for role validation instead of ozzo In() rule.
func (v *Validator) ValidateUser(u *domain.User) error {
	// Fast path: check role first with map lookup (O(1) instead of O(n))
	if u.Role == "" {
		return validation.Errors{"role": validation.NewError("role_required", "role_required")}
	}
	if !validRolesMap[u.Role] {
		return validation.Errors{"role": validation.NewError("invalid_role", "invalid_role")}
	}

	// Now validate remaining fields
	return validation.ValidateStruct(u,
		validation.Field(&u.ID,
			validation.Required.Error("id_required"),
			validation.Match(uuidRegex).Error("invalid_id_format"),
		),
		validation.Field(&u.Email,
			validation.Required.Error("email_required"),
			validation.Match(emailRegex).Error("invalid_email_format"),
		),
		validation.Field(&u.Name,
			validation.Required.Error("name_required"),
		),
	)
}

// ValidateArticle validates an Article entity.
func (v *Validator) ValidateArticle(a *domain.Article) error {
	err := validation.ValidateStruct(a,
		validation.Field(&a.ID,
			validation.Required.Error("id_required"),
			validation.Match(uuidRegex).Error("invalid_id_format"),
		),
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
			validation.Match(uuidRegex).Error("invalid_author_id_format"),
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
		validation.Field(&c.ID,
			validation.Required.Error("id_required"),
			validation.Match(commentIDRegex).Error("invalid_id_format"),
		),
		validation.Field(&c.Body,
			validation.Required.Error("body_required"),
			validation.By(wordCountRule(500)),
		),
		validation.Field(&c.ArticleID,
			validation.Required.Error("article_id_required"),
			validation.Match(uuidRegex).Error("invalid_article_id_format"),
		),
		validation.Field(&c.UserID,
			validation.Required.Error("user_id_required"),
			validation.Match(uuidRegex).Error("invalid_user_id_format"),
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

// AppendValidationErrors appends validation errors directly to the target slice.
// This is more efficient than ConvertValidationErrors as it avoids slice spread copies.
func AppendValidationErrors(target *[]domain.RecordError, rowNum int, err error) {
	if ve, ok := err.(validation.Errors); ok {
		for field, fieldErr := range ve {
			*target = append(*target, domain.RecordError{
				Row:    rowNum,
				Field:  field,
				Reason: fieldErr.Error(),
			})
		}
	} else if err != nil {
		*target = append(*target, domain.RecordError{
			Row:    rowNum,
			Field:  "unknown",
			Reason: err.Error(),
		})
	}
}
