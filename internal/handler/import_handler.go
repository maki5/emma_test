package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"bulk-import-export/internal/domain"
	"bulk-import-export/internal/middleware"
	"bulk-import-export/internal/service"
)

// ImportHandler handles import-related HTTP requests.
type ImportHandler struct {
	importService *service.ImportService
}

// NewImportHandler creates a new ImportHandler.
func NewImportHandler(importService *service.ImportService) *ImportHandler {
	return &ImportHandler{
		importService: importService,
	}
}

// ImportJobResponse represents an import job in the API response.
type ImportJobResponse struct {
	ID               string                 `json:"id"`
	ResourceType     string                 `json:"resource_type"`
	Status           string                 `json:"status"`
	TotalRecords     int                    `json:"total_records"`
	ProcessedRecords int                    `json:"processed_records"`
	SuccessCount     int                    `json:"success_count"`
	FailureCount     int                    `json:"failure_count"`
	ErrorMessage     *string                `json:"error_message,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt        string                 `json:"created_at"`
	UpdatedAt        string                 `json:"updated_at"`
	CompletedAt      *string                `json:"completed_at,omitempty"`
}

// toImportJobResponse converts a domain.ImportJob to an ImportJobResponse.
func toImportJobResponse(job *domain.ImportJob) ImportJobResponse {
	response := ImportJobResponse{
		ID:               job.ID,
		ResourceType:     job.ResourceType,
		Status:           string(job.Status),
		TotalRecords:     job.TotalRecords,
		ProcessedRecords: job.ProcessedRecords,
		SuccessCount:     job.SuccessCount,
		FailureCount:     job.FailureCount,
		ErrorMessage:     job.ErrorMessage,
		Metadata:         job.Metadata,
		CreatedAt:        job.CreatedAt.Format(TimeFormat),
		UpdatedAt:        job.UpdatedAt.Format(TimeFormat),
	}
	if job.CompletedAt != nil {
		completedAt := job.CompletedAt.Format(TimeFormat)
		response.CompletedAt = &completedAt
	}
	return response
}

// CreateImport handles POST /api/v1/imports
func (h *ImportHandler) CreateImport(c *gin.Context) {
	resourceType := c.PostForm("resource_type")
	if resourceType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resource_type is required"})
		return
	}

	if !domain.IsValidResourceType(resourceType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resource_type must be one of: users, articles, comments"})
		return
	}

	idempotencyToken := c.PostForm("idempotency_token")
	if idempotencyToken == "" {
		idempotencyToken = uuid.New().String()
	}

	if _, err := uuid.Parse(idempotencyToken); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "idempotency_token must be a valid UUID"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	defer file.Close()

	requestID := middleware.GetRequestID(c)
	job, err := h.importService.StartImport(c.Request.Context(), resourceType, idempotencyToken, header.Filename, requestID, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, toImportJobResponse(job))
}

// GetImport handles GET /api/v1/imports/:id
func (h *ImportHandler) GetImport(c *gin.Context) {
	id := c.Param("id")

	// Validate that the ID is a valid UUID
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id must be a valid UUID"})
		return
	}

	job, err := h.importService.GetImportJob(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if job == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "import job not found"})
		return
	}

	c.JSON(http.StatusOK, toImportJobResponse(job))
}

// ListImports handles GET /api/v1/imports
func (h *ImportHandler) ListImports(c *gin.Context) {
	jobs, err := h.importService.ListImportJobs(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	items := make([]ImportJobResponse, len(jobs))
	for i := range jobs {
		items[i] = toImportJobResponse(&jobs[i])
	}

	c.JSON(http.StatusOK, gin.H{
		"items": items,
		"total": len(items),
	})
}
