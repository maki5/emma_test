package validator

import (
	"regexp"
	"testing"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"
)

var benchEmailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

type benchUser struct {
	Email string
	Name  string
}

func BenchmarkIsEmail(b *testing.B) {
	u := &benchUser{Email: "user@example.com", Name: "Test User"}
	for i := 0; i < b.N; i++ {
		validation.ValidateStruct(u,
			validation.Field(&u.Email, is.Email),
			validation.Field(&u.Name, validation.Required),
		)
	}
}

func BenchmarkRegexEmail(b *testing.B) {
	u := &benchUser{Email: "user@example.com", Name: "Test User"}
	for i := 0; i < b.N; i++ {
		validation.ValidateStruct(u,
			validation.Field(&u.Email, validation.Match(benchEmailRegex)),
			validation.Field(&u.Name, validation.Required),
		)
	}
}

func BenchmarkDirectRegex(b *testing.B) {
	email := "user@example.com"
	for i := 0; i < b.N; i++ {
		_ = benchEmailRegex.MatchString(email)
	}
}
