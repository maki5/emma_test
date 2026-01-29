package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"bulk-import-export/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestID_GeneratesNewID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestID())

	router.GET("/test", func(c *gin.Context) {
		requestID := middleware.GetRequestID(c)
		c.JSON(http.StatusOK, gin.H{"request_id": requestID})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Check response header contains request ID
	requestID := w.Header().Get(middleware.RequestIDHeader)
	assert.NotEmpty(t, requestID)
	assert.Len(t, requestID, 36) // UUID v4 length
}

func TestRequestID_UsesClientProvidedID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestID())

	router.GET("/test", func(c *gin.Context) {
		requestID := middleware.GetRequestID(c)
		c.JSON(http.StatusOK, gin.H{"request_id": requestID})
	})

	clientRequestID := "client-provided-id-12345"
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(middleware.RequestIDHeader, clientRequestID)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Check response header contains client-provided request ID
	requestID := w.Header().Get(middleware.RequestIDHeader)
	assert.Equal(t, clientRequestID, requestID)
}

func TestRequestID_SetInContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestID())

	var capturedRequestID string
	router.GET("/test", func(c *gin.Context) {
		capturedRequestID = middleware.GetRequestID(c)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, capturedRequestID)
}

func TestRequestID_SetInResponseHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestID())

	router.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	requestID := w.Header().Get(middleware.RequestIDHeader)
	assert.NotEmpty(t, requestID)
}

func TestGetRequestID_ReturnsEmptyWhenNotSet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	requestID := middleware.GetRequestID(c)
	assert.Empty(t, requestID)
}

func TestGetRequestID_ReturnsRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	expectedID := "test-request-id"
	c.Set(middleware.RequestIDKey, expectedID)

	requestID := middleware.GetRequestID(c)
	assert.Equal(t, expectedID, requestID)
}

func TestGetRequestID_ReturnsEmptyWhenWrongType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	// Set wrong type intentionally
	c.Set(middleware.RequestIDKey, 12345)

	requestID := middleware.GetRequestID(c)
	assert.Empty(t, requestID)
}

func TestRequestID_MultipleRequests_DifferentIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestID())

	var requestIDs []string
	router.GET("/test", func(c *gin.Context) {
		requestID := middleware.GetRequestID(c)
		requestIDs = append(requestIDs, requestID)
		c.Status(http.StatusOK)
	})

	// Make 3 requests
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	}

	// All request IDs should be different
	require.Len(t, requestIDs, 3)
	assert.NotEqual(t, requestIDs[0], requestIDs[1])
	assert.NotEqual(t, requestIDs[1], requestIDs[2])
	assert.NotEqual(t, requestIDs[0], requestIDs[2])
}
