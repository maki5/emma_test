package handler

import (
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"bulk-import-export/internal/domain"
	"bulk-import-export/internal/middleware"
	"bulk-import-export/internal/service"
)

// ExportHandler handles export-related HTTP requests.
type ExportHandler struct {
	exportService *service.ExportService
}

// NewExportHandler creates a new ExportHandler.
func NewExportHandler(exportService *service.ExportService) *ExportHandler {
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
	ErrorMessage *string `json:"error_message,omitempty"`
	DownloadURL  *string `json:"download_url,omitempty"`
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
		ErrorMessage: job.ErrorMessage,
		CreatedAt:    job.CreatedAt.Format(TimeFormat),
		UpdatedAt:    job.UpdatedAt.Format(TimeFormat),
	}
	if job.CompletedAt != nil {
		completedAt := job.CompletedAt.Format(TimeFormat)
		response.CompletedAt = &completedAt
	}
	if job.FilePath != "" {
		downloadURL := "/api/v1/exports/" + job.ID + "/download"
		response.DownloadURL = &downloadURL
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

// ListExports handles GET /api/v1/exports
func (h *ExportHandler) ListExports(c *gin.Context) {
	jobs, err := h.exportService.ListExportJobs(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	items := make([]ExportJobResponse, len(jobs))
	for i := range jobs {
		items[i] = toExportJobResponse(&jobs[i])
	}

	c.JSON(http.StatusOK, gin.H{
		"items": items,
		"total": len(items),
	})
}

// DownloadExport handles GET /api/v1/exports/:id/download
func (h *ExportHandler) DownloadExport(c *gin.Context) {
	id := c.Param("id")

	// Validate that the ID is a valid UUID
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id must be a valid UUID"})
		return
	}

	filePath, err := h.exportService.GetExportFilePath(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	filename := filepath.Base(filePath)
	c.FileAttachment(filePath, filename)
}
