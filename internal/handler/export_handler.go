package handler

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"bulk-import-export/internal/domain"
	"bulk-import-export/internal/middleware"
	"bulk-import-export/internal/service"
)

// ExportHandler handles export-related HTTP requests.
type ExportHandler struct {
	exportService service.ExportServiceInterface
}

// NewExportHandler creates a new ExportHandler.
func NewExportHandler(exportService service.ExportServiceInterface) *ExportHandler {
	return &ExportHandler{
		exportService: exportService,
	}
}

// CreateExportRequest represents the request for creating an export job.
type CreateExportRequest struct {
	ResourceType     string `json:"resource_type" binding:"required,oneof=users articles comments"`
	Format           string `json:"format" binding:"required,oneof=csv ndjson"`
	IdempotencyToken string `json:"idempotency_token" binding:"required,uuid"`
}

// ExportJobResponse represents an export job in the API response.
type ExportJobResponse struct {
	ID           string  `json:"id"`
	ResourceType string  `json:"resource_type"`
	Format       string  `json:"format"`
	Status       string  `json:"status"`
	TotalRecords int     `json:"total_records"`
	FilePath     string  `json:"file_path,omitempty"`
	ErrorMessage *string `json:"error_message,omitempty"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
	CompletedAt  *string `json:"completed_at,omitempty"`
}

// toExportJobResponse converts a domain.ExportJob to an ExportJobResponse.
func toExportJobResponse(job *domain.ExportJob) ExportJobResponse {
	response := ExportJobResponse{
		ID:           job.ID,
		ResourceType: job.ResourceType,
		Format:       job.Format,
		Status:       string(job.Status),
		TotalRecords: job.TotalRecords,
		FilePath:     job.FilePath,
		ErrorMessage: job.ErrorMessage,
		CreatedAt:    job.CreatedAt.Format(TimeFormat),
		UpdatedAt:    job.UpdatedAt.Format(TimeFormat),
	}
	if job.CompletedAt != nil {
		completedAt := job.CompletedAt.Format(TimeFormat)
		response.CompletedAt = &completedAt
	}
	return response
}

// CreateExport handles POST /api/v1/exports
func (h *ExportHandler) CreateExport(c *gin.Context) {
	var req CreateExportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.IdempotencyToken == "" {
		req.IdempotencyToken = uuid.New().String()
	}

	if _, err := uuid.Parse(req.IdempotencyToken); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "idempotency_token must be a valid UUID"})
		return
	}

	requestID := middleware.GetRequestID(c)
	job, err := h.exportService.StartExport(c.Request.Context(), req.ResourceType, req.Format, req.IdempotencyToken, requestID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, toExportJobResponse(job))
}

// GetExport handles GET /api/v1/exports/:id
func (h *ExportHandler) GetExport(c *gin.Context) {
	id := c.Param("id")

	// Validate that the ID is a valid UUID
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id must be a valid UUID"})
		return
	}

	job, err := h.exportService.GetExportJob(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if job == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "export job not found"})
		return
	}

	c.JSON(http.StatusOK, toExportJobResponse(job))
}

// StreamExportRequest represents query parameters for streaming export.
type StreamExportRequest struct {
	Resource string `form:"resource" binding:"required,oneof=users articles comments"`
	Format   string `form:"format" binding:"omitempty,oneof=csv ndjson"`
}

// ginStreamWriter wraps gin.ResponseWriter for streaming.
type ginStreamWriter struct {
	writer gin.ResponseWriter
}

func (w *ginStreamWriter) Write(data []byte) error {
	_, err := w.writer.Write(data)
	return err
}

func (w *ginStreamWriter) Flush() {
	w.writer.Flush()
}

// StreamExport handles GET /api/v1/exports?resource=...&format=...
func (h *ExportHandler) StreamExport(c *gin.Context) {
	var req StreamExportRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Default format is ndjson
	if req.Format == "" {
		req.Format = "ndjson"
	}

	requestID := middleware.GetRequestID(c)
	log.Printf("[request_id=%s] Streaming export: resource=%s, format=%s", requestID, req.Resource, req.Format)

	// Set appropriate content type
	contentType := "application/x-ndjson"
	if req.Format == "csv" {
		contentType = "text/csv"
	}

	c.Header("Content-Type", contentType)
	c.Header("Transfer-Encoding", "chunked")
	c.Header("X-Content-Type-Options", "nosniff")

	// Set filename for download
	filename := req.Resource + "." + req.Format
	c.Header("Content-Disposition", "attachment; filename=\""+filename+"\"")

	writer := &ginStreamWriter{writer: c.Writer}

	var count int
	var err error

	switch req.Resource {
	case "users":
		count, err = h.exportService.StreamUsers(c.Request.Context(), req.Format, writer)
	case "articles":
		count, err = h.exportService.StreamArticles(c.Request.Context(), req.Format, writer)
	case "comments":
		count, err = h.exportService.StreamComments(c.Request.Context(), req.Format, writer)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid resource type"})
		return
	}

	if err != nil {
		log.Printf("[request_id=%s] Streaming export error: %v", requestID, err)
		// Can't return error at this point as we've started writing
		return
	}

	log.Printf("[request_id=%s] Streaming export completed: resource=%s, count=%d", requestID, req.Resource, count)
}
