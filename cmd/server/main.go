package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"bulk-import-export/internal/config"
	"bulk-import-export/internal/handler"
	"bulk-import-export/internal/infrastructure/database"
	"bulk-import-export/internal/logger"
	"bulk-import-export/internal/metrics"
	"bulk-import-export/internal/middleware"
	"bulk-import-export/internal/repository"
	"bulk-import-export/internal/service"
	"bulk-import-export/internal/validator"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load configuration",
			slog.String("error", err.Error()))
	}

	// Connect to database
	pool, err := database.NewPostgres(context.Background(), database.PoolConfig{
		Host:              cfg.DBHost,
		Port:              cfg.DBPort,
		User:              cfg.DBUser,
		Password:          cfg.DBPassword,
		Database:          cfg.DBName,
		SSLMode:           cfg.DBSSLMode,
		MaxConns:          cfg.DBMaxConns,
		MinConns:          cfg.DBMinConns,
		MaxConnLifetime:   cfg.DBMaxConnLifetime,
		MaxConnIdleTime:   cfg.DBMaxConnIdleTime,
		HealthCheckPeriod: cfg.DBHealthCheckPeriod,
	})
	if err != nil {
		logger.Fatal("Failed to connect to database",
			slog.String("error", err.Error()))
	}
	defer pool.Close()

	// Start database pool metrics collector
	poolStatsCollector := metrics.NewPoolStatsCollector(pool)
	poolStatsCollector.Start(15 * time.Second)
	defer poolStatsCollector.Stop()

	// Initialize repositories
	userRepo := repository.NewPostgresUserRepository(pool)
	articleRepo := repository.NewPostgresArticleRepository(pool)
	commentRepo := repository.NewPostgresCommentRepository(pool)
	jobRepo := repository.NewPostgresJobRepository(pool)

	// Initialize validator
	v := validator.NewValidator()

	// Initialize services
	importService := service.NewImportService(
		userRepo,
		articleRepo,
		commentRepo,
		jobRepo,
		v,
		cfg.BatchSize,
		cfg.WorkerPoolSize,
	)

	exportService, err := service.NewExportService(
		userRepo,
		articleRepo,
		commentRepo,
		jobRepo,
		cfg.ExportDir,
		cfg.WorkerPoolSize,
	)
	if err != nil {
		logger.Fatal("Failed to create export service",
			slog.String("error", err.Error()))
	}

	// Initialize handlers
	importHandler := handler.NewImportHandler(importService)
	exportHandler := handler.NewExportHandler(exportService)
	healthHandler := handler.NewHealthHandler(pool)

	// Setup Gin router
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.RequestID())
	router.Use(middleware.Metrics())
	router.Use(gin.Logger())

	// Health and metrics endpoints
	router.GET("/health", healthHandler.Health)
	router.GET("/ready", healthHandler.Ready)
	router.GET("/live", healthHandler.Live)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Import routes
		imports := v1.Group("/imports")
		{
			imports.POST("", importHandler.CreateImport)
			imports.GET("/:id", importHandler.GetImport)
		}

		// Export routes
		exports := v1.Group("/exports")
		{
			exports.POST("", exportHandler.CreateExport)
			exports.GET("", exportHandler.StreamExport)
			exports.GET("/:id", exportHandler.GetExport)
		}
	}

	// Create HTTP server
	srv := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      router,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	// Start server in goroutine
	go func() {
		logger.Info("Starting server",
			slog.String("port", cfg.ServerPort))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server",
				slog.String("error", err.Error()))
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down server")

	// Close services first to stop accepting new work
	logger.Info("Closing import service")
	importService.Close()
	logger.Info("Closing export service")
	exportService.Close()

	// Shutdown HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server shutdown error",
			slog.String("error", err.Error()))
	}

	logger.Info("Server exited")
}
