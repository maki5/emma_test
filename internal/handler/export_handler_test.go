package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"bulk-import-export/internal/domain"
	"bulk-import-export/internal/mocks"
	"bulk-import-export/internal/service"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestStreamExport_Users_NDJSON(t *testing.T) {
	mockService := mocks.NewMockExportServiceInterface(t)
	handler := NewExportHandler(mockService)

	// Setup mock expectation for StreamUsers
	mockService.EXPECT().
		StreamUsers(mock.Anything, "ndjson", mock.AnythingOfType("*handler.ginStreamWriter")).
		Run(func(ctx context.Context, format string, writer service.StreamWriter) {
			// Simulate writing NDJSON data
			_ = writer.Write([]byte(`{"id":"user-1","email":"user1@example.com","name":"User One"}` + "\n"))
			_ = writer.Write([]byte(`{"id":"user-2","email":"user2@example.com","name":"User Two"}` + "\n"))
			_ = writer.Write([]byte(`{"id":"user-3","email":"user3@example.com","name":"User Three"}` + "\n"))
		}).
		Return(3, nil)

	router := gin.New()
	router.GET("/api/v1/exports", handler.StreamExport)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports?resource=users&format=ndjson", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "application/x-ndjson")

	// Verify NDJSON output
	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	require.Equal(t, 3, len(lines), "Expected 3 lines")

	// Verify each line is valid JSON
	for i, line := range lines {
		var user map[string]interface{}
		err := json.Unmarshal([]byte(line), &user)
		require.NoError(t, err, "Line %d should be valid JSON", i)
		require.Contains(t, user, "email", "Line %d should have email field", i)
	}
}

func TestStreamExport_Users_CSV(t *testing.T) {
	mockService := mocks.NewMockExportServiceInterface(t)
	handler := NewExportHandler(mockService)

	// Setup mock expectation for StreamUsers
	mockService.EXPECT().
		StreamUsers(mock.Anything, "csv", mock.AnythingOfType("*handler.ginStreamWriter")).
		Run(func(ctx context.Context, format string, writer service.StreamWriter) {
			// Simulate writing CSV data
			_ = writer.Write([]byte("id,email,name,role,is_active,created_at,updated_at\n"))
			_ = writer.Write([]byte("user-1,user1@example.com,User One,user,true,2024-01-01T00:00:00Z,2024-01-01T00:00:00Z\n"))
			_ = writer.Write([]byte("user-2,user2@example.com,User Two,admin,true,2024-01-01T00:00:00Z,2024-01-01T00:00:00Z\n"))
		}).
		Return(2, nil)

	router := gin.New()
	router.GET("/api/v1/exports", handler.StreamExport)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports?resource=users&format=csv", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "text/csv")

	// Verify CSV output
	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	require.GreaterOrEqual(t, len(lines), 2, "Expected at least header + 1 row")
	require.Contains(t, lines[0], "id,email", "First line should be header")
}

func TestStreamExport_Articles_NDJSON(t *testing.T) {
	mockService := mocks.NewMockExportServiceInterface(t)
	handler := NewExportHandler(mockService)

	mockService.EXPECT().
		StreamArticles(mock.Anything, "ndjson", mock.AnythingOfType("*handler.ginStreamWriter")).
		Run(func(ctx context.Context, format string, writer service.StreamWriter) {
			_ = writer.Write([]byte(`{"id":"article-1","slug":"test-article","title":"Test Article"}` + "\n"))
			_ = writer.Write([]byte(`{"id":"article-2","slug":"test-article-2","title":"Test Article 2"}` + "\n"))
		}).
		Return(2, nil)

	router := gin.New()
	router.GET("/api/v1/exports", handler.StreamExport)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports?resource=articles&format=ndjson", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	require.Equal(t, 2, len(lines), "Expected 2 articles")
}

func TestStreamExport_Comments_NDJSON(t *testing.T) {
	mockService := mocks.NewMockExportServiceInterface(t)
	handler := NewExportHandler(mockService)

	mockService.EXPECT().
		StreamComments(mock.Anything, "ndjson", mock.AnythingOfType("*handler.ginStreamWriter")).
		Run(func(ctx context.Context, format string, writer service.StreamWriter) {
			_ = writer.Write([]byte(`{"id":"comment-1","body":"Comment 1"}` + "\n"))
			_ = writer.Write([]byte(`{"id":"comment-2","body":"Comment 2"}` + "\n"))
		}).
		Return(2, nil)

	router := gin.New()
	router.GET("/api/v1/exports", handler.StreamExport)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports?resource=comments&format=ndjson", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	require.Equal(t, 2, len(lines), "Expected 2 comments")
}

func TestStreamExport_DefaultFormat(t *testing.T) {
	mockService := mocks.NewMockExportServiceInterface(t)
	handler := NewExportHandler(mockService)

	// When no format is specified, it should default to ndjson
	mockService.EXPECT().
		StreamUsers(mock.Anything, "ndjson", mock.AnythingOfType("*handler.ginStreamWriter")).
		Return(1, nil)

	router := gin.New()
	router.GET("/api/v1/exports", handler.StreamExport)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports?resource=users", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "application/x-ndjson")
}

func TestStreamExport_MissingResource(t *testing.T) {
	mockService := mocks.NewMockExportServiceInterface(t)
	handler := NewExportHandler(mockService)

	router := gin.New()
	router.GET("/api/v1/exports", handler.StreamExport)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestStreamExport_InvalidResource(t *testing.T) {
	mockService := mocks.NewMockExportServiceInterface(t)
	handler := NewExportHandler(mockService)

	router := gin.New()
	router.GET("/api/v1/exports", handler.StreamExport)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports?resource=invalid", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestStreamExport_InvalidFormat(t *testing.T) {
	mockService := mocks.NewMockExportServiceInterface(t)
	handler := NewExportHandler(mockService)

	router := gin.New()
	router.GET("/api/v1/exports", handler.StreamExport)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports?resource=users&format=json", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetExport_Success(t *testing.T) {
	mockService := mocks.NewMockExportServiceInterface(t)
	handler := NewExportHandler(mockService)

	now := time.Now()
	completedAt := now.Add(5 * time.Second)
	job := &domain.ExportJob{
		ID:           "550e8400-e29b-41d4-a716-446655440000",
		ResourceType: "users",
		Format:       "ndjson",
		Status:       domain.JobStatusCompleted,
		TotalRecords: 100,
		FilePath:     "/exports/users-export.ndjson",
		CreatedAt:    now,
		UpdatedAt:    now,
		CompletedAt:  &completedAt,
	}

	mockService.EXPECT().
		GetExportJob(mock.Anything, "550e8400-e29b-41d4-a716-446655440000").
		Return(job, nil)

	router := gin.New()
	router.GET("/api/v1/exports/:id", handler.GetExport)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/550e8400-e29b-41d4-a716-446655440000", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response ExportJobResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	require.Equal(t, "550e8400-e29b-41d4-a716-446655440000", response.ID)
	require.Equal(t, "users", response.ResourceType)
	require.Equal(t, "completed", response.Status)
	require.Equal(t, 100, response.TotalRecords)
	require.Equal(t, "/exports/users-export.ndjson", response.FilePath)
}

func TestGetExport_NotFound(t *testing.T) {
	mockService := mocks.NewMockExportServiceInterface(t)
	handler := NewExportHandler(mockService)

	mockService.EXPECT().
		GetExportJob(mock.Anything, "550e8400-e29b-41d4-a716-446655440000").
		Return(nil, nil)

	router := gin.New()
	router.GET("/api/v1/exports/:id", handler.GetExport)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/550e8400-e29b-41d4-a716-446655440000", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetExport_InvalidID(t *testing.T) {
	mockService := mocks.NewMockExportServiceInterface(t)
	handler := NewExportHandler(mockService)

	router := gin.New()
	router.GET("/api/v1/exports/:id", handler.GetExport)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/invalid-id", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetExport_ServiceError(t *testing.T) {
	mockService := mocks.NewMockExportServiceInterface(t)
	handler := NewExportHandler(mockService)

	mockService.EXPECT().
		GetExportJob(mock.Anything, "550e8400-e29b-41d4-a716-446655440000").
		Return(nil, errors.New("database error"))

	router := gin.New()
	router.GET("/api/v1/exports/:id", handler.GetExport)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/550e8400-e29b-41d4-a716-446655440000", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestCreateExport_Success(t *testing.T) {
	mockService := mocks.NewMockExportServiceInterface(t)
	handler := NewExportHandler(mockService)

	now := time.Now()
	job := &domain.ExportJob{
		ID:               "550e8400-e29b-41d4-a716-446655440000",
		ResourceType:     "users",
		Format:           "ndjson",
		Status:           domain.JobStatusPending,
		IdempotencyToken: "550e8400-e29b-41d4-a716-446655440001",
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	mockService.EXPECT().
		StartExport(mock.Anything, "users", "ndjson", "550e8400-e29b-41d4-a716-446655440001", mock.Anything).
		Return(job, nil)

	router := gin.New()
	router.POST("/api/v1/exports", handler.CreateExport)

	body := `{"resource_type":"users","format":"ndjson","idempotency_token":"550e8400-e29b-41d4-a716-446655440001"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusAccepted, w.Code)

	var response ExportJobResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	require.Equal(t, "pending", response.Status)
}

func TestCreateExport_InvalidRequest(t *testing.T) {
	mockService := mocks.NewMockExportServiceInterface(t)
	handler := NewExportHandler(mockService)

	router := gin.New()
	router.POST("/api/v1/exports", handler.CreateExport)

	// Missing required fields
	body := `{"format":"ndjson"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}
