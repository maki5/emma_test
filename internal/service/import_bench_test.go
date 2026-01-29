package service

import (
	"bytes"
	"encoding/csv"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"bulk-import-export/internal/domain"
	"bulk-import-export/internal/validator"
)

func BenchmarkValidateUserOnly(b *testing.B) {
	v := validator.NewValidator()
	user := &domain.User{
		ID:     "test-id",
		Email:  "user@example.com",
		Name:   "Test User",
		Role:   "admin",
		Active: true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = v.ValidateUser(user)
	}
}

func BenchmarkValidateUserWithInvalidRole(b *testing.B) {
	v := validator.NewValidator()
	user := &domain.User{
		ID:     "test-id",
		Email:  "user@example.com",
		Name:   "Test User",
		Role:   "invalid_role",
		Active: true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = v.ValidateUser(user)
	}
}

func BenchmarkFullLoopWith1000Users(b *testing.B) {
	v := validator.NewValidator()

	var buf bytes.Buffer
	buf.WriteString("id,email,name,role,active,created_at,updated_at\n")
	roles := []string{"admin", "user", "moderator", "invalid1", "invalid2"}
	for i := 0; i < 1000; i++ {
		role := roles[i%5]
		buf.WriteString(",user")
		buf.WriteString(string(rune('0' + i%10)))
		buf.WriteString("@example.com,User Name,")
		buf.WriteString(role)
		buf.WriteString(",true,2024-01-01T00:00:00Z,2024-01-01T00:00:00Z\n")
	}
	data := buf.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(data)
		csvReader := csv.NewReader(reader)

		header, _ := csvReader.Read()
		colMap := make(map[string]int)
		for j, col := range header {
			colMap[strings.ToLower(strings.TrimSpace(col))] = j
		}

		var errors []domain.RecordError
		for {
			record, err := csvReader.Read()
			if err == io.EOF {
				break
			}

			user := domain.User{
				ID:     uuid.New().String(),
				Active: true,
			}

			if idx, ok := colMap["email"]; ok && idx < len(record) {
				user.Email = strings.TrimSpace(record[idx])
			}
			if idx, ok := colMap["name"]; ok && idx < len(record) {
				user.Name = strings.TrimSpace(record[idx])
			}
			if idx, ok := colMap["role"]; ok && idx < len(record) {
				user.Role = strings.TrimSpace(record[idx])
			}

			start := time.Now()
			if err := v.ValidateUser(&user); err != nil {
				_ = time.Since(start)
				validator.AppendValidationErrors(&errors, 0, err)
				continue
			}
			_ = time.Since(start)
		}
		_ = errors
	}
}

func BenchmarkRealCSVFile(b *testing.B) {
	data, err := os.ReadFile("../../testdata/users_huge.csv")
	if err != nil {
		b.Skip("testdata/users_huge.csv not found")
	}

	v := validator.NewValidator()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(data)
		csvReader := csv.NewReader(reader)

		header, _ := csvReader.Read()
		colMap := make(map[string]int)
		for j, col := range header {
			colMap[strings.ToLower(strings.TrimSpace(col))] = j
		}

		var errors []domain.RecordError
		recordCount := 0
		for {
			record, err := csvReader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				continue
			}
			recordCount++

			user := domain.User{
				ID:     uuid.New().String(),
				Active: true,
			}

			if idx, ok := colMap["email"]; ok && idx < len(record) {
				user.Email = strings.TrimSpace(record[idx])
			}
			if idx, ok := colMap["name"]; ok && idx < len(record) {
				user.Name = strings.TrimSpace(record[idx])
			}
			if idx, ok := colMap["role"]; ok && idx < len(record) {
				user.Role = strings.TrimSpace(record[idx])
			}

			if err := v.ValidateUser(&user); err != nil {
				validator.AppendValidationErrors(&errors, 0, err)
			}
		}
		_ = errors
	}
}
