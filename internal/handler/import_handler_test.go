package handler

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"bulk-import-export/internal/domain"
	"bulk-import-export/internal/mocks"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestImportHandler_CreateImport(t *testing.T) {
	t.Run("creates import job successfully", func(t *testing.T) {
		mockService := mocks.NewMockImportServiceInterface(t)
		handler := NewImportHandler(mockService)

		now := time.Now()
		expectedJob := &domain.ImportJob{
			ID:               uuid.New().String(),
			ResourceType:     "users",
			Status:           domain.JobStatusPending,
			IdempotencyToken: uuid.New().String(),
			CreatedAt:        now,
			UpdatedAt:        now,
		}

		mockService.EXPECT().
			StartImport(mock.Anything, "users", mock.AnythingOfType("string"), "users.csv", mock.Anything, mock.Anything).
			Return(expectedJob, nil)

		router := gin.New()
		router.POST("/api/v1/imports", handler.CreateImport)

		// Create multipart form
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("resource_type", "users")
		part, _ := writer.CreateFormFile("file", "users.csv")
		part.Write([]byte("email,name,role\ntest@example.com,Test User,user"))
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/api/v1/imports", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusAccepted, w.Code)

		var response ImportJobResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, expectedJob.ID, response.ID)
		assert.Equal(t, "users", response.ResourceType)
		assert.Equal(t, "pending", response.Status)
	})

	t.Run("returns error when resource_type is missing", func(t *testing.T) {
		mockService := mocks.NewMockImportServiceInterface(t)
		handler := NewImportHandler(mockService)

		router := gin.New()
		router.POST("/api/v1/imports", handler.CreateImport)

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, _ := writer.CreateFormFile("file", "users.csv")
		part.Write([]byte("email,name,role\ntest@example.com,Test User,user"))
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/api/v1/imports", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "resource_type is required")
	})

	t.Run("returns error for invalid resource_type", func(t *testing.T) {
		mockService := mocks.NewMockImportServiceInterface(t)
		handler := NewImportHandler(mockService)

		router := gin.New()
		router.POST("/api/v1/imports", handler.CreateImport)

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("resource_type", "invalid")
		part, _ := writer.CreateFormFile("file", "file.csv")
		part.Write([]byte("data"))
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/api/v1/imports", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "resource_type must be one of")
	})

	t.Run("returns error when file is missing", func(t *testing.T) {
		mockService := mocks.NewMockImportServiceInterface(t)
		handler := NewImportHandler(mockService)

		router := gin.New()
		router.POST("/api/v1/imports", handler.CreateImport)

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("resource_type", "users")
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/api/v1/imports", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "file is required")
	})

	t.Run("returns error for invalid idempotency_token", func(t *testing.T) {
		mockService := mocks.NewMockImportServiceInterface(t)
		handler := NewImportHandler(mockService)

		router := gin.New()
		router.POST("/api/v1/imports", handler.CreateImport)

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("resource_type", "users")
		_ = writer.WriteField("idempotency_token", "not-a-uuid")
		part, _ := writer.CreateFormFile("file", "users.csv")
		part.Write([]byte("email,name,role\ntest@example.com,Test User,user"))
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/api/v1/imports", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "idempotency_token must be a valid UUID")
	})

	t.Run("uses provided idempotency_token", func(t *testing.T) {
		mockService := mocks.NewMockImportServiceInterface(t)
		handler := NewImportHandler(mockService)

		token := uuid.New().String()
		now := time.Now()
		expectedJob := &domain.ImportJob{
			ID:               uuid.New().String(),
			ResourceType:     "users",
			Status:           domain.JobStatusPending,
			IdempotencyToken: token,
			CreatedAt:        now,
			UpdatedAt:        now,
		}

		mockService.EXPECT().
			StartImport(mock.Anything, "users", token, "users.csv", mock.Anything, mock.Anything).
			Return(expectedJob, nil)

		router := gin.New()
		router.POST("/api/v1/imports", handler.CreateImport)

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("resource_type", "users")
		_ = writer.WriteField("idempotency_token", token)
		part, _ := writer.CreateFormFile("file", "users.csv")
		part.Write([]byte("email,name,role\ntest@example.com,Test User,user"))
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/api/v1/imports", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusAccepted, w.Code)
	})

	t.Run("supports articles resource type", func(t *testing.T) {
		mockService := mocks.NewMockImportServiceInterface(t)
		handler := NewImportHandler(mockService)

		now := time.Now()
		expectedJob := &domain.ImportJob{
			ID:           uuid.New().String(),
			ResourceType: "articles",
			Status:       domain.JobStatusPending,
			CreatedAt:    now,
			UpdatedAt:    now,
		}

		mockService.EXPECT().
			StartImport(mock.Anything, "articles", mock.Anything, "articles.ndjson", mock.Anything, mock.Anything).
			Return(expectedJob, nil)

		router := gin.New()
		router.POST("/api/v1/imports", handler.CreateImport)

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("resource_type", "articles")
		part, _ := writer.CreateFormFile("file", "articles.ndjson")
		part.Write([]byte(`{"id":"123","slug":"test","title":"Test","body":"Body"}`))
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/api/v1/imports", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusAccepted, w.Code)
	})

	t.Run("supports comments resource type", func(t *testing.T) {
		mockService := mocks.NewMockImportServiceInterface(t)
		handler := NewImportHandler(mockService)

		now := time.Now()
		expectedJob := &domain.ImportJob{
			ID:           uuid.New().String(),
			ResourceType: "comments",
			Status:       domain.JobStatusPending,
			CreatedAt:    now,
			UpdatedAt:    now,
		}

		mockService.EXPECT().
			StartImport(mock.Anything, "comments", mock.Anything, "comments.ndjson", mock.Anything, mock.Anything).
			Return(expectedJob, nil)

		router := gin.New()
		router.POST("/api/v1/imports", handler.CreateImport)

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("resource_type", "comments")
		part, _ := writer.CreateFormFile("file", "comments.ndjson")
		part.Write([]byte(`{"id":"cm_123","body":"Comment"}`))
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/api/v1/imports", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusAccepted, w.Code)
	})
}

func TestImportHandler_GetImport(t *testing.T) {
	t.Run("returns import job when found", func(t *testing.T) {
		mockService := mocks.NewMockImportServiceInterface(t)
		handler := NewImportHandler(mockService)

		jobID := uuid.New().String()
		now := time.Now()
		completedAt := now.Add(time.Minute)
		expectedJob := &domain.ImportJob{
			ID:               jobID,
			ResourceType:     "users",
			Status:           domain.JobStatusCompleted,
			TotalRecords:     100,
			ProcessedRecords: 100,
			SuccessCount:     95,
			FailureCount:     5,
			CreatedAt:        now,
			UpdatedAt:        completedAt,
			CompletedAt:      &completedAt,
		}

		mockService.EXPECT().
			GetImportJob(mock.Anything, jobID).
			Return(expectedJob, nil)

		router := gin.New()
		router.GET("/api/v1/imports/:id", handler.GetImport)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/imports/"+jobID, nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var response ImportJobResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, jobID, response.ID)
		assert.Equal(t, "users", response.ResourceType)
		assert.Equal(t, "completed", response.Status)
		assert.Equal(t, 100, response.TotalRecords)
		assert.Equal(t, 95, response.SuccessCount)
		assert.Equal(t, 5, response.FailureCount)
		assert.NotNil(t, response.CompletedAt)
	})

	t.Run("returns 404 when job not found", func(t *testing.T) {
		mockService := mocks.NewMockImportServiceInterface(t)
		handler := NewImportHandler(mockService)

		jobID := uuid.New().String()

		mockService.EXPECT().
			GetImportJob(mock.Anything, jobID).
			Return(nil, nil)

		router := gin.New()
		router.GET("/api/v1/imports/:id", handler.GetImport)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/imports/"+jobID, nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "import job not found")
	})

	t.Run("returns error for invalid UUID", func(t *testing.T) {
		mockService := mocks.NewMockImportServiceInterface(t)
		handler := NewImportHandler(mockService)

		router := gin.New()
		router.GET("/api/v1/imports/:id", handler.GetImport)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/imports/not-a-uuid", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "id must be a valid UUID")
	})

	t.Run("returns processing status correctly", func(t *testing.T) {
		mockService := mocks.NewMockImportServiceInterface(t)
		handler := NewImportHandler(mockService)

		jobID := uuid.New().String()
		now := time.Now()
		expectedJob := &domain.ImportJob{
			ID:               jobID,
			ResourceType:     "articles",
			Status:           domain.JobStatusProcessing,
			TotalRecords:     1000,
			ProcessedRecords: 500,
			SuccessCount:     490,
			FailureCount:     10,
			CreatedAt:        now,
			UpdatedAt:        now,
		}

		mockService.EXPECT().
			GetImportJob(mock.Anything, jobID).
			Return(expectedJob, nil)

		router := gin.New()
		router.GET("/api/v1/imports/:id", handler.GetImport)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/imports/"+jobID, nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var response ImportJobResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "processing", response.Status)
		assert.Equal(t, 500, response.ProcessedRecords)
		assert.Nil(t, response.CompletedAt)
	})

	t.Run("returns failed status with error message", func(t *testing.T) {
		mockService := mocks.NewMockImportServiceInterface(t)
		handler := NewImportHandler(mockService)

		jobID := uuid.New().String()
		now := time.Now()
		errMsg := "Database connection failed"
		expectedJob := &domain.ImportJob{
			ID:           jobID,
			ResourceType: "comments",
			Status:       domain.JobStatusFailed,
			ErrorMessage: &errMsg,
			CreatedAt:    now,
			UpdatedAt:    now,
		}

		mockService.EXPECT().
			GetImportJob(mock.Anything, jobID).
			Return(expectedJob, nil)

		router := gin.New()
		router.GET("/api/v1/imports/:id", handler.GetImport)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/imports/"+jobID, nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var response ImportJobResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "failed", response.Status)
		require.NotNil(t, response.ErrorMessage)
		assert.Equal(t, errMsg, *response.ErrorMessage)
	})
}
