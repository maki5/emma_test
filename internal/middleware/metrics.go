// Package middleware provides HTTP middleware for the Gin framework.
package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"bulk-import-export/internal/metrics"
)

// Metrics returns a Gin middleware that records Prometheus metrics for HTTP requests.
// It tracks:
// - Total requests by method, path, and status code
// - Request duration histogram
// - Requests currently in flight
func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip metrics endpoint to avoid self-referential metrics
		if c.FullPath() == "/metrics" {
			c.Next()
			return
		}

		start := time.Now()

		// Track in-flight requests
		metrics.HTTPRequestsInFlight.Inc()
		defer metrics.HTTPRequestsInFlight.Dec()

		// Process request
		c.Next()

		// Record metrics after request completes
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())
		path := c.FullPath()

		// Use the actual path for unmatched routes
		if path == "" {
			path = "unmatched"
		}

		metrics.HTTPRequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(c.Request.Method, path).Observe(duration)
	}
}
