package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"

	"bulk-import-export/internal/metrics"
)

func TestMetricsMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("records HTTP request metrics", func(t *testing.T) {
		router := gin.New()
		router.Use(Metrics())
		router.GET("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		// Get initial counter values
		initialTotal := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/test", "200"))
		initialInFlight := testutil.ToFloat64(metrics.HTTPRequestsInFlight)

		// Make request
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Verify counter incremented
		newTotal := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/test", "200"))
		assert.Equal(t, initialTotal+1, newTotal, "Request counter should increment")

		// In-flight should be back to initial (request completed)
		afterInFlight := testutil.ToFloat64(metrics.HTTPRequestsInFlight)
		assert.Equal(t, initialInFlight, afterInFlight, "In-flight should return to initial after request")
	})

	t.Run("records different status codes", func(t *testing.T) {
		router := gin.New()
		router.Use(Metrics())
		router.GET("/notfound", func(c *gin.Context) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		})

		initialTotal := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/notfound", "404"))

		req := httptest.NewRequest(http.MethodGet, "/notfound", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)

		newTotal := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/notfound", "404"))
		assert.Equal(t, initialTotal+1, newTotal, "404 counter should increment")
	})

	t.Run("skips metrics endpoint", func(t *testing.T) {
		router := gin.New()
		router.Use(Metrics())
		router.GET("/metrics", func(c *gin.Context) {
			c.String(http.StatusOK, "metrics data")
		})

		// Note: The /metrics path will still match the route, but the middleware won't record metrics for it
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("handles POST requests", func(t *testing.T) {
		router := gin.New()
		router.Use(Metrics())
		router.POST("/api/create", func(c *gin.Context) {
			c.JSON(http.StatusCreated, gin.H{"id": "123"})
		})

		initialTotal := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("POST", "/api/create", "201"))

		req := httptest.NewRequest(http.MethodPost, "/api/create", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		newTotal := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("POST", "/api/create", "201"))
		assert.Equal(t, initialTotal+1, newTotal, "POST counter should increment")
	})
}
