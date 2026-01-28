package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthHandler handles health check requests.
type HealthHandler struct {
	db *pgxpool.Pool
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(db *pgxpool.Pool) *HealthHandler {
	return &HealthHandler{db: db}
}

// HealthResponse represents the response for health check endpoints.
type HealthResponse struct {
	Status   string            `json:"status"`
	Version  string            `json:"version,omitempty"`
	Services map[string]string `json:"services,omitempty"`
}

// Health handles GET /health - comprehensive health check.
func (h *HealthHandler) Health(c *gin.Context) {
	services := map[string]string{
		"database": "healthy",
	}

	if err := h.db.Ping(c.Request.Context()); err != nil {
		services["database"] = "unhealthy"
		c.JSON(http.StatusServiceUnavailable, HealthResponse{
			Status:   "unhealthy",
			Services: services,
		})
		return
	}

	c.JSON(http.StatusOK, HealthResponse{
		Status:   "healthy",
		Version:  "1.0.0",
		Services: services,
	})
}

// Ready handles GET /ready - readiness probe for Kubernetes.
func (h *HealthHandler) Ready(c *gin.Context) {
	if err := h.db.Ping(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}

// Live handles GET /live - liveness probe for Kubernetes.
func (h *HealthHandler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "alive"})
}
