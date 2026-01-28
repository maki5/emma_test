# Architecture Design

## Simplified Clean Architecture (3 Layers)

This architecture uses a streamlined 3-layer design with embedded worker pools for high-performance bulk data processing.

---

## Project Structure

```
bulk-import-export/
├── cmd/
│   └── server/
│       └── main.go              # Single entry point
├── internal/
│   ├── domain/                  # Business entities
│   │   ├── user.go
│   │   ├── article.go
│   │   ├── comment.go
│   │   └── job.go
│   ├── service/                 # Business logic + worker pool
│   │   ├── import_service.go    # Import logic with embedded workers
│   │   ├── import_service_test.go
│   │   ├── export_service.go    # Export logic with embedded workers
│   │   ├── export_service_test.go
│   │   └── worker_pool.go       # Shared worker pool implementation
│   ├── repository/              # Data access
│   │   ├── user_repository.go
│   │   ├── user_repository_test.go
│   │   ├── article_repository.go
│   │   ├── article_repository_test.go
│   │   ├── comment_repository.go
│   │   ├── comment_repository_test.go
│   │   ├── job_repository.go
│   │   ├── job_repository_test.go
│   │   ├── cte_helper.go        # Shared CTE SQL builder
│   │   └── testutil/
│   │       └── postgres_container.go  # Testcontainers setup
│   ├── handler/                 # HTTP handlers
│   │   ├── import_handler.go
│   │   ├── import_handler_test.go
│   │   ├── export_handler.go
│   │   └── export_handler_test.go
│   ├── mocks/                   # Generated mocks (mockery)
│   │   ├── mock_user_repository.go
│   │   ├── mock_article_repository.go
│   │   ├── mock_comment_repository.go
│   │   ├── mock_job_repository.go
│   │   ├── mock_import_service.go
│   │   └── mock_export_service.go
│   └── infrastructure/          # External services
│       ├── database/
│       │   └── postgres.go
│       └── metrics/
│           └── prometheus.go
├── migrations/                  # SQL migrations
├── scripts/                     # Test scripts
├── monitoring/                  # Prometheus config
├── exports/                     # Async export files (gitignored)
├── testdata/                    # Test data files (gitignored)
│   ├── users_huge.csv
│   ├── articles_huge.ndjson
│   └── comments_huge.ndjson
├── .mockery.yaml                # Mockery configuration
└── docs/                        # Documentation
```

---

## Component Diagram

```
┌──────────────────────────────────────────────────────────────────┐
│                        HTTP Layer (Gin)                          │
│  ┌──────────────────┐  ┌──────────────────┐                     │
│  │  Import Handler  │  │  Export Handler  │                     │
│  └────────┬─────────┘  └────────┬─────────┘                     │
└───────────┼─────────────────────┼────────────────────────────────┘
            │                     │
            ▼                     ▼
┌──────────────────────────────────────────────────────────────────┐
│                     Service Layer                                │
│  ┌──────────────────────────┐  ┌──────────────────────────┐     │
│  │     ImportService        │  │     ExportService        │     │
│  │  ┌────────────────────┐  │  │  ┌────────────────────┐  │     │
│  │  │    Worker Pool     │  │  │  │    Worker Pool     │  │     │
│  │  │ ┌────┐┌────┐┌────┐ │  │  │  │ (streaming cursor) │  │     │
│  │  │ │ W1 ││ W2 ││ Wn │ │  │  │  └────────────────────┘  │     │
│  │  │ └────┘└────┘└────┘ │  │  │                          │     │
│  │  └────────────────────┘  │  └──────────────────────────┘     │
│  └──────────────────────────┘                                   │
└──────────────────────────────────────────────────────────────────┘
            │                     │
            ▼                     ▼
┌──────────────────────────────────────────────────────────────────┐
│                     Repository Layer                             │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐  ┌────────────┐ │
│  │   Users    │  │  Articles  │  │  Comments  │  │    Jobs    │ │
│  └────────────┘  └────────────┘  └────────────┘  └────────────┘ │
└──────────────────────────────────────────────────────────────────┘
                              │
                              ▼
                    ┌──────────────────┐
                    │   PostgreSQL 15  │
                    └──────────────────┘
```

---

## Domain Models

### Entity Structs

```go
// internal/domain/user.go
type User struct {
    ID        uuid.UUID  `json:"id"`
    Email     string     `json:"email"`
    Name      string     `json:"name"`
    Role      string     `json:"role"`
    Active    bool       `json:"active"`
    CreatedAt time.Time  `json:"created_at"`
    UpdatedAt time.Time  `json:"updated_at"`
}

// internal/domain/article.go
type Article struct {
    ID          uuid.UUID  `json:"id"`
    Slug        string     `json:"slug"`
    Title       string     `json:"title"`
    Description string     `json:"description,omitempty"`
    Body        string     `json:"body"`
    AuthorID    uuid.UUID  `json:"author_id"`
    Tags        []string   `json:"tags,omitempty"`
    PublishedAt *time.Time `json:"published_at,omitempty"`
    Status      string     `json:"status"`
    CreatedAt   time.Time  `json:"created_at"`
    UpdatedAt   time.Time  `json:"updated_at"`
}

// internal/domain/comment.go
type Comment struct {
    ID        uuid.UUID `json:"id"`
    Body      string    `json:"body"`
    ArticleID uuid.UUID `json:"article_id"`
    UserID    uuid.UUID `json:"user_id"`
    CreatedAt time.Time `json:"created_at"`
}

// internal/domain/job.go
type JobStatus string

const (
    JobStatusPending           JobStatus = "pending"
    JobStatusProcessing        JobStatus = "processing"
    JobStatusCompleted         JobStatus = "completed"
    JobStatusCompletedWithErrs JobStatus = "completed_with_errors"
    JobStatusFailed            JobStatus = "failed"
    JobStatusCancelled         JobStatus = "cancelled"
)

type ImportJob struct {
    ID               uuid.UUID  `json:"job_id"`
    IdempotencyKey   string     `json:"idempotency_key,omitempty"`
    ResourceType     string     `json:"resource_type"`
    Mode             string     `json:"mode"`
    Status           JobStatus  `json:"status"`
    TotalRecords     int        `json:"total_records"`
    ProcessedRecords int        `json:"processed_records"`
    SuccessfulRecords int       `json:"successful_records"`
    ErrorRecords     int        `json:"error_records"`
    FailureReason    string     `json:"failure_reason,omitempty"`
    StartedAt        *time.Time `json:"started_at,omitempty"`
    CompletedAt      *time.Time `json:"completed_at,omitempty"`
    CreatedAt        time.Time  `json:"created_at"`
}

type ExportJob struct {
    ID             uuid.UUID  `json:"job_id"`
    IdempotencyKey string     `json:"idempotency_key,omitempty"`
    ResourceType   string     `json:"resource_type"`
    Format         string     `json:"format"`
    Status         JobStatus  `json:"status"`
    Filters        map[string]interface{} `json:"filters,omitempty"`
    Fields         []string   `json:"fields,omitempty"`
    RecordCount    int        `json:"record_count,omitempty"`
    FilePath       string     `json:"file_path,omitempty"`
    DownloadURL    string     `json:"download_url,omitempty"`
    FailureReason  string     `json:"failure_reason,omitempty"`
    StartedAt      *time.Time `json:"started_at,omitempty"`
    CompletedAt    *time.Time `json:"completed_at,omitempty"`
    CreatedAt      time.Time  `json:"created_at"`
}

// internal/domain/errors.go
type RecordError struct {
    Row    int    `json:"row"`
    Field  string `json:"field"`
    Value  string `json:"value,omitempty"`
    Reason string `json:"reason"`
}

type ValidationError struct {
    Field  string
    Reason string
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("%s: %s", e.Field, e.Reason)
}
```

---

## Worker Pool Design

### Why Worker Pools?

Without worker pools:
```
1M records → 1M goroutines → OOM / context switch overhead
```

With worker pools:
```
1M records → 10 workers → Controlled memory, predictable performance
```

### Implementation

```go
// internal/service/worker_pool.go
type WorkerPool struct {
    workerCount int
    jobQueue    chan Job
    resultQueue chan Result
    wg          sync.WaitGroup
    ctx         context.Context
    cancel      context.CancelFunc
}

func NewWorkerPool(workerCount int) *WorkerPool {
    ctx, cancel := context.WithCancel(context.Background())
    pool := &WorkerPool{
        workerCount: workerCount,
        jobQueue:    make(chan Job, workerCount*2),
        resultQueue: make(chan Result, workerCount*2),
        ctx:         ctx,
        cancel:      cancel,
    }
    pool.start()
    return pool
}

func (p *WorkerPool) start() {
    for i := 0; i < p.workerCount; i++ {
        p.wg.Add(1)
        go p.worker(i)
    }
}

func (p *WorkerPool) worker(id int) {
    defer p.wg.Done()
    for {
        select {
        case <-p.ctx.Done():
            return
        case job, ok := <-p.jobQueue:
            if !ok {
                return
            }
            result := p.processJob(job)
            p.resultQueue <- result
        }
    }
}
```

---

## Idempotency Mechanism

### Why Idempotency?

When importing large files (potentially 1M+ records), network issues can occur:

```
Client                         Server
   |---- POST /v1/imports ------->|
   |                              | (creates job, starts processing)
   |<------ 504 Timeout --------- | (connection dropped)
   |                              |
   |  ??? Did it succeed ???      | (job continues in background!)
```

**The problem**: The client doesn't know if the import started. Without idempotency:
- Retry → duplicate import job created → data processed twice
- Don't retry → job might have failed → data not imported

**The solution**: Idempotency tokens allow safe retries.

### How It Works

```
┌─────────────────────────────────────────────────────────────────┐
│  Client generates unique token: "import-users-2026-01-28-abc"   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  POST /v1/imports                                               │
│  Idempotency-Key: import-users-2026-01-28-abc                   │
│  Content-Type: multipart/form-data                              │
│  [file data]                                                    │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  Server checks: Does job with this key exist?                   │
│                                                                 │
│  NO  → Create new job, store key in DB, queue for processing    │
│  YES → Return existing job (status may be pending/processing/   │
│        completed) - no duplicate created                        │
└─────────────────────────────────────────────────────────────────┘
```

### Implementation

```go
// internal/handler/import_handler.go

func (h *ImportHandler) Create(c *gin.Context) {
    idempotencyKey := c.GetHeader("Idempotency-Key")
    ctx := c.Request.Context()
    
    // Check for existing job with this idempotency key (fast path)
    if idempotencyKey != "" {
        existingJob, err := h.service.FindByIdempotencyKey(ctx, idempotencyKey)
        if err == nil && existingJob != nil {
            // Return existing job - safe retry (200 OK indicates existing resource)
            c.JSON(http.StatusOK, toJobResponse(existingJob))
            return
        }
    }
    
    // Parse request (multipart or JSON with remote URL)
    req, err := h.parseImportRequest(c)
    if err != nil {
        c.JSON(http.StatusBadRequest, errorResponse(err))
        return
    }
    
    // Create and queue new job
    job, err := h.service.CreateImportJob(ctx, req, idempotencyKey)
    if err != nil {
        // Handle race condition: concurrent request with same idempotency key
        // DB UNIQUE constraint prevents duplicates, so check again
        if isUniqueViolation(err) && idempotencyKey != "" {
            existingJob, findErr := h.service.FindByIdempotencyKey(ctx, idempotencyKey)
            if findErr == nil && existingJob != nil {
                c.JSON(http.StatusOK, toJobResponse(existingJob))
                return
            }
        }
        c.JSON(http.StatusInternalServerError, errorResponse(err))
        return
    }
    
    // Return immediately - processing happens in background worker
    // 202 Accepted indicates new job created and queued
    c.JSON(http.StatusAccepted, toJobResponse(job))
}

// isUniqueViolation checks if error is a PostgreSQL unique constraint violation
func isUniqueViolation(err error) bool {
    var pgErr *pgconn.PgError
    if errors.As(err, &pgErr) {
        return pgErr.Code == "23505" // unique_violation
    }
    return false
}
```

```go
// internal/service/import_service.go

func (s *ImportService) CreateImportJob(ctx context.Context, req ImportRequest, idempotencyKey string) (*domain.ImportJob, error) {
    job := &domain.ImportJob{
        ID:             uuid.New(),
        IdempotencyKey: idempotencyKey,  // May be empty (optional)
        ResourceType:   req.ResourceType,
        Status:         domain.JobStatusPending,
        CreatedAt:      time.Now(),
    }
    
    // Store job in database (idempotency_key has UNIQUE constraint)
    if err := s.jobRepo.Create(ctx, job); err != nil {
        return nil, fmt.Errorf("failed to create job: %w", err)
    }
    
    // Queue job for background processing
    s.workerPool.Submit(job)
    
    return job, nil
}

func (s *ImportService) FindByIdempotencyKey(ctx context.Context, key string) (*domain.ImportJob, error) {
    return s.jobRepo.FindByIdempotencyKey(ctx, key)
}
```

```go
// internal/repository/job_repository.go

func (r *JobRepository) FindByIdempotencyKey(ctx context.Context, key string) (*domain.ImportJob, error) {
    query := `
        SELECT id, idempotency_key, resource_type, status, 
               total_records, processed_records, successful_records, error_records,
               started_at, completed_at, created_at
        FROM import_jobs
        WHERE idempotency_key = $1
    `
    
    var job domain.ImportJob
    err := r.db.QueryRow(ctx, query, key).Scan(
        &job.ID, &job.IdempotencyKey, &job.ResourceType, &job.Status,
        &job.TotalRecords, &job.ProcessedRecords, &job.SuccessfulRecords, &job.ErrorRecords,
        &job.StartedAt, &job.CompletedAt, &job.CreatedAt,
    )
    if err == pgx.ErrNoRows {
        return nil, nil  // Not found - not an error
    }
    return &job, err
}
```

### Key Design Points

| Aspect | Decision |
|--------|----------|
| **Token generation** | Client-generated (UUID or `{resource}-{timestamp}-{hash}`) |
| **Storage** | `idempotency_key` column with UNIQUE constraint |
| **Scope** | One token per logical operation (same import = same key) |
| **Response on duplicate** | Return existing job with current status (200 OK) |
| **First request response** | 202 Accepted (job queued for background processing) |
| **Token required?** | Optional - if not provided, no idempotency protection |

### Retry Scenario

```
Timeline:
─────────────────────────────────────────────────────────────────►

Attempt 1:
  POST /v1/imports + Idempotency-Key: abc123
  → Server creates job #42 (status: pending)
  → Server queues job for background worker
  → Worker starts processing...
  → HTTP timeout after 30s (client doesn't see response)
  → Worker CONTINUES processing (independent of HTTP)

Attempt 2 (retry after timeout):
  POST /v1/imports + Idempotency-Key: abc123
  → Server finds job #42 already exists
  → Returns job #42 (status: processing, 500k of 1M done)
  → Client now knows the job ID!

Client polls for status:
  GET /v1/imports/42
  → { "status": "completed", "processed": 1000000, "errors": [...] }
```

---

## Data Flow

### Import Flow

```
HTTP Request (multipart/remote URL)
        │
        ▼
┌───────────────────┐
│  Idempotency      │ ← Check Idempotency-Key header
│  Check            │   Return existing job if found
└─────────┬─────────┘
          │ (new request)
          ▼
┌───────────────────┐
│  Parse Request    │ ← Validate format, create job record
└─────────┬─────────┘
          │
          ▼
┌───────────────────┐
│  Stream Parse     │ ← Read file in chunks (O(1) memory)
└─────────┬─────────┘
          │
          ▼
┌───────────────────┐
│  Batch Records    │ ← Group into 1000-record batches
└─────────┬─────────┘
          │
          ▼
┌─────────────────────────────────────────────────────┐
│  BEGIN TRANSACTION (single tx for entire import)    │
│                                                     │
│  ┌─────────────────────────────────────────────┐   │
│  │  FOR EACH BATCH:                            │   │
│  │  ┌───────────────────┐                      │   │
│  │  │ CTE Insert Batch  │ ← Insert 1000 recs  │   │
│  │  └─────────┬─────────┘                      │   │
│  │            │                                │   │
│  │      ┌─────┴─────┐                          │   │
│  │   Success    Fatal Error ──────────────────────► ROLLBACK ALL
│  │      │                                      │   │     │
│  │      ▼                                      │   │     ▼
│  │  Next Batch                                 │   │  Update Job
│  └─────────────────────────────────────────────┘   │  status=failed
│                                                     │
│  All batches successful → COMMIT                   │
└─────────────────────────────────────────────────────┘
          │
          ▼
┌─────────────────────────┐
│  Update Job              │ ← status=completed with errors
└─────────────────────────┘
```

**Key Principle**: Each batch runs in its **own transaction**. If a batch fails, it's rolled back and the error is recorded, but remaining batches continue processing.

### Per-Batch Transactions (Continue on Failure)

> **INTENTIONAL DESIGN DECISION**: We use per-batch transactions instead of a single 
> transaction for the entire import. This avoids huge commits/rollbacks that would:
> - Hold database locks for extended periods
> - Cause expensive rollback operations on failure
> - Risk transaction log overflow on large imports
>
> Trade-off: Partial data may exist if some batches fail. The job status and error
> details clearly indicate which batches succeeded and which failed.

```
1M records file
     │
     ▼
┌─────────────────────────────────────────────────┐
│  Batch 1 (records 1-1000)                       │
│  ┌───────────────────────────────────────────┐ │
│  │ BEGIN → INSERT → COMMIT                   │ │ ← Success
│  └───────────────────────────────────────────┘ │
│                                                 │
│  Batch 2 (records 1001-2000)                    │
│  ┌───────────────────────────────────────────┐ │
│  │ BEGIN → INSERT → COMMIT                   │ │ ← Success
│  └───────────────────────────────────────────┘ │
│                                                 │
│  Batch 3 (records 2001-3000)                    │
│  ┌───────────────────────────────────────────┐ │
│  │ BEGIN → INSERT → ROLLBACK (error)         │ │ ← Failed, logged
│  └───────────────────────────────────────────┘ │
│                                                 │
│  Batch 4 (records 3001-4000)                    │
│  ┌───────────────────────────────────────────┐ │
│  │ BEGIN → INSERT → COMMIT                   │ │ ← Continues!
│  └───────────────────────────────────────────┘ │
│  ...                                            │
└─────────────────────────────────────────────────┘
```

**Why per-batch transactions?**
- ✅ **Fast rollback**: Only failed batch is rolled back (1000 records max)
- ✅ **Short locks**: Locks released after each batch
- ✅ **No WAL bloat**: Small transactions = small WAL segments
- ✅ **Progress visible**: Committed data visible immediately

**Trade-off accepted:**
- ⚠️ **Partial data**: If batch 500 fails, batches 1-499 are already committed
- ⚠️ **Requires awareness**: Users must check job status for failed batches

```go
// internal/service/import_service.go

// INTENTIONAL DESIGN: Per-batch transactions to avoid huge commits/rollbacks.
// Trade-off: Partial data may exist if some batches fail.

type BatchResult struct {
    BatchNumber int `json:"batch_number"`
    Successful  int `json:"successful"`
    Failed      int `json:"failed"`
}

type ImportResult struct {
    TotalProcessed  int `json:"processed_records"`
    TotalSuccessful int `json:"successful_records"`
    TotalFailed     int `json:"failed_records"`
}

func (s *ImportService) ExecuteImport(ctx context.Context, job *domain.Job, records []domain.Record) (*ImportResult, error) {
    result := &ImportResult{}
    
    // Process in batches - each batch has its own transaction
    batches := s.splitIntoBatches(records, s.cfg.BatchSize)
    
    for batchNum, batch := range batches {
        // Check for cancellation
        select {
        case <-ctx.Done():
            s.updateJobCancelled(ctx, job, result)
            return result, ctx.Err()
        default:
        }
        
        // Process batch with its own transaction
        batchResult, err := s.processBatchWithTransaction(ctx, batch, batchNum)
        if err != nil {
            // Batch failed - log error and CONTINUE with remaining batches
            s.logger.Error("batch failed",
                slog.Int("batch_number", batchNum+1),
                slog.String("error", err.Error()),
            )
            result.TotalProcessed += len(batch)
            result.TotalFailed += len(batch)
            
            // Continue processing remaining batches
            continue
        }
        
        // Aggregate results
        result.TotalProcessed += len(batch)
        result.TotalSuccessful += batchResult.Successful
        result.TotalFailed += batchResult.Failed
        
        // Update progress
        s.updateJobProgress(ctx, job, result.TotalProcessed, result.TotalSuccessful)
    }
    
    // Determine final status based on failures
    if result.TotalFailed > 0 {
        s.updateJobCompletedWithErrors(ctx, job, result)
    } else {
        s.updateJobCompleted(ctx, job, result)
    }
    
    return result, nil
}

// processBatchWithTransaction handles a single batch in its own transaction
func (s *ImportService) processBatchWithTransaction(ctx context.Context, batch []domain.Record, batchNum int) (*BatchResult, error) {
    // Start transaction for this batch only
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return nil, fmt.Errorf("begin transaction: %w", err)
    }
    defer tx.Rollback(ctx)  // No-op if committed
    
    // Execute CTE insert (only UNIQUE/FK validation at DB level)
    result, err := s.repo.BulkInsertWithCTE(ctx, tx, batch)
    if err != nil {
        // Transaction rolled back by defer
        return nil, err
    }
    
    // Commit this batch
    if err := tx.Commit(ctx); err != nil {
        return nil, fmt.Errorf("commit failed: %w", err)
    }
    
    return result, nil
}

// filterValidRecords performs app-side validation before DB insert
func (s *ImportService) filterValidRecords(records []domain.Record) ([]domain.Record, []domain.RecordError) {
    var valid []domain.Record
    var errors []domain.RecordError
    
    for i, record := range records {
        if err := s.validator.Validate(record); err != nil {
            errors = append(errors, domain.RecordError{
                Row:    i + 1,
                Field:  err.Field,
                Value:  err.Value,
                Reason: err.Reason,
            })
            continue
        }
        valid = append(valid, record)
    }
    
    return valid, errors
}
```

### Repository Batch Insert

The repository receives a transaction from the service layer and executes within it:

```go
// internal/repository/user_repository.go

// BulkInsertWithCTE inserts users within the provided transaction
func (r *UserRepository) BulkInsertWithCTE(ctx context.Context, tx pgx.Tx, users []domain.User) (*domain.BatchResult, error) {
    // Execute CTE insert within the transaction
    result, err := r.executeCTEInsert(ctx, tx, users)
    if err != nil {
        return nil, fmt.Errorf("cte insert failed: %w", err)
    }
    
    return result, nil
}
```

### Failure Scenarios

| Scenario | Behavior |
|----------|----------|
| Batch 3 of 10 fails | Batch 3 rolled back, batches 1-2 committed, batches 4-10 continue, status = `completed_with_errors` |
| Record validation error | Record skipped, added to errors list, batch continues, import continues |
| All batches succeed | All committed, status = `completed` |
| Cancellation requested | Current batch completes, remaining skipped, status = `cancelled` |
| Connection lost mid-batch | Current batch rolled back, job eventually marked `failed` |
```

### Export Flow

```
HTTP Request (streaming/async)
        │
        ▼
┌───────────────────────┐
│  Idempotency Check    │ ← Check Idempotency-Key header (async only)
│  (POST only)          │   Return existing job if found
└───────────┬───────────┘
            │
            ▼
┌───────────────────────┐
│  Parse Query Params   │ ← resource, format, filters
└───────────┬───────────┘
            │
      ┌─────┴─────┐
      │           │
      ▼           ▼
  Streaming    Async
      │           │
      ▼           ▼
┌───────────┐ ┌─────────────┐
│  Cursor   │ │ Create Job  │
│  Query    │ │ Background  │
└─────┬─────┘ └──────┬──────┘
      │              │
      ▼              ▼
┌───────────┐ ┌─────────────────────────────────┐
│  Stream   │ │  Execute Export                 │
│  Response │ │  ┌───────────────────────────┐  │
└───────────┘ │  │ Query DB with cursor      │  │
              │  └─────────────┬─────────────┘  │
              │                │                │
              │          ┌─────┴─────┐          │
              │       Success      Error        │
              │          │           │          │
              │          ▼           ▼          │
              │  ┌─────────────┐ ┌───────────┐  │
              │  │ Write to    │ │ Update    │  │
              │  │ File        │ │ Job=failed│  │
              │  └──────┬──────┘ └───────────┘  │
              │         │                       │
              │         ▼                       │
              │  ┌─────────────┐                │
              │  │ Update Job  │                │
              │  │ =completed  │                │
              │  └─────────────┘                │
              └─────────────────────────────────┘
```

### Export Handler with Idempotency

```go
// internal/handler/export_handler.go

func (h *ExportHandler) CreateAsyncExport(c *gin.Context) {
    idempotencyKey := c.GetHeader("Idempotency-Key")
    ctx := c.Request.Context()
    
    // Check for existing job with this idempotency key (fast path)
    if idempotencyKey != "" {
        existingJob, err := h.service.FindExportByIdempotencyKey(ctx, idempotencyKey)
        if err == nil && existingJob != nil {
            // Return existing job - safe retry (200 OK indicates existing resource)
            c.JSON(http.StatusOK, toExportJobResponse(existingJob))
            return
        }
    }
    
    // Parse request body
    var req CreateExportRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, errorResponse(err))
        return
    }
    
    // Create and queue new export job
    job, err := h.service.CreateExportJob(ctx, req, idempotencyKey)
    if err != nil {
        // Handle race condition: concurrent request with same idempotency key
        if isUniqueViolation(err) && idempotencyKey != "" {
            existingJob, findErr := h.service.FindExportByIdempotencyKey(ctx, idempotencyKey)
            if findErr == nil && existingJob != nil {
                c.JSON(http.StatusOK, toExportJobResponse(existingJob))
                return
            }
        }
        c.JSON(http.StatusInternalServerError, errorResponse(err))
        return
    }
    
    // Return immediately - processing happens in background worker
    // 202 Accepted indicates new job created and queued
    c.JSON(http.StatusAccepted, toExportJobResponse(job))
}
```

### Async Export with Error Handling

```go
// internal/service/export_service.go
func (s *ExportService) ExecuteAsyncExport(ctx context.Context, job *domain.ExportJob) error {
    // Update job status to processing
    job.Status = domain.JobStatusProcessing
    job.StartedAt = time.Now()
    s.jobRepo.Update(ctx, job)
    
    // Create export file
    filePath := filepath.Join(s.cfg.ExportFilePath, fmt.Sprintf("%s-%s.%s", 
        job.Resource, job.ID, job.Format))
    
    file, err := os.Create(filePath)
    if err != nil {
        s.updateExportJobFailed(ctx, job, fmt.Sprintf("failed to create file: %v", err))
        return err
    }
    defer file.Close()
    
    // Query with cursor
    rows, err := s.getRepository(job.Resource).StreamAll(ctx, job.Filters)
    if err != nil {
        s.updateExportJobFailed(ctx, job, fmt.Sprintf("query failed: %v", err))
        return err
    }
    defer rows.Close()
    
    recordCount := 0
    writer := bufio.NewWriter(file)
    
    for rows.Next() {
        // Check for cancellation
        select {
        case <-ctx.Done():
            os.Remove(filePath)  // Cleanup partial file
            s.updateExportJobFailed(ctx, job, "export cancelled")
            return ctx.Err()
        default:
        }
        
        record, err := rows.Scan()
        if err != nil {
            s.updateExportJobFailed(ctx, job, fmt.Sprintf("scan error at record %d: %v", recordCount, err))
            os.Remove(filePath)
            return err
        }
        
        line, err := s.formatRecord(record, job.Format)
        if err != nil {
            s.updateExportJobFailed(ctx, job, fmt.Sprintf("format error at record %d: %v", recordCount, err))
            os.Remove(filePath)
            return err
        }
        
        if _, err := writer.WriteString(line + "\n"); err != nil {
            s.updateExportJobFailed(ctx, job, fmt.Sprintf("write error: %v", err))
            os.Remove(filePath)
            return err
        }
        
        recordCount++
    }
    
    if err := rows.Err(); err != nil {
        s.updateExportJobFailed(ctx, job, fmt.Sprintf("iteration error: %v", err))
        os.Remove(filePath)
        return err
    }
    
    if err := writer.Flush(); err != nil {
        s.updateExportJobFailed(ctx, job, fmt.Sprintf("flush error: %v", err))
        os.Remove(filePath)
        return err
    }
    
    // Success - update job
    s.updateExportJobCompleted(ctx, job, filePath, recordCount)
    return nil
}

func (s *ExportService) updateExportJobFailed(ctx context.Context, job *domain.ExportJob, reason string) {
    job.Status = domain.JobStatusFailed
    job.FailureReason = reason
    job.CompletedAt = time.Now()
    s.jobRepo.Update(ctx, job)
}

func (s *ExportService) updateExportJobCompleted(ctx context.Context, job *domain.ExportJob, filePath string, recordCount int) {
    job.Status = domain.JobStatusCompleted
    job.FilePath = filePath
    job.RecordCount = recordCount
    job.CompletedAt = time.Now()
    s.jobRepo.Update(ctx, job)
}
```

---

## Configuration

```go
// internal/config/config.go
type Config struct {
    // Server
    HTTPPort string `env:"HTTP_PORT" default:"8080"`
    
    // Database
    DatabaseURL      string        `env:"DATABASE_URL" required:"true"`
    DBMaxConns       int           `env:"DB_MAX_CONNS" default:"25"`
    DBMaxIdleConns   int           `env:"DB_MAX_IDLE_CONNS" default:"5"`
    DBConnMaxLife    time.Duration `env:"DB_CONN_MAX_LIFETIME" default:"1h"`
    
    // Worker Pool
    WorkerPoolSize    int `env:"WORKER_POOL_SIZE" default:"10"`
    BatchSize         int `env:"BATCH_SIZE" default:"1000"`
    MaxConcurrentJobs int `env:"MAX_CONCURRENT_JOBS" default:"5"`
    
    // Safety Limits
    MaxFileSizeMB    int           `env:"MAX_FILE_SIZE_MB" default:"500"`
    RemoteURLTimeout time.Duration `env:"REMOTE_URL_TIMEOUT" default:"5m"`
    
    // Async Export
    ExportFilePath string `env:"EXPORT_FILE_PATH" default:"./exports"`
    
    // Metrics
    EnableMetrics bool   `env:"ENABLE_METRICS" default:"true"`
    MetricsPath   string `env:"METRICS_PATH" default:"/metrics"`
}
```

---

## Dependency Injection

```go
// cmd/server/main.go
func main() {
    // Load configuration
    cfg := config.Load()
    
    // Initialize database with pool settings
    db := database.NewPostgres(cfg.DatabaseURL, database.PoolConfig{
        MaxConns:     cfg.DBMaxConns,
        MaxIdleConns: cfg.DBMaxIdleConns,
        MaxLifetime:  cfg.DBConnMaxLife,
    })
    
    // Initialize repositories
    userRepo := repository.NewUserRepository(db)
    articleRepo := repository.NewArticleRepository(db)
    commentRepo := repository.NewCommentRepository(db)
    jobRepo := repository.NewJobRepository(db)
    
    // Initialize services (with embedded worker pool)
    importSvc := service.NewImportService(userRepo, articleRepo, commentRepo, jobRepo, cfg)
    exportSvc := service.NewExportService(userRepo, articleRepo, commentRepo, jobRepo, cfg)
    
    // Initialize handlers
    importHandler := handler.NewImportHandler(importSvc)
    exportHandler := handler.NewExportHandler(exportSvc)
    
    // Setup router
    router := gin.Default()
    
    // Register routes
    v1 := router.Group("/v1")
    {
        // Import endpoints
        v1.POST("/imports", importHandler.Create)
        v1.GET("/imports", importHandler.List)
        v1.GET("/imports/:id", importHandler.GetStatus)
        v1.POST("/imports/:id/cancel", importHandler.Cancel)
        
        // Export endpoints
        v1.GET("/exports", exportHandler.StreamExport)
        v1.POST("/exports", exportHandler.CreateAsyncExport)
        v1.GET("/exports/:id", exportHandler.GetStatus)
        v1.GET("/exports/:id/download", exportHandler.Download)
        v1.POST("/exports/:id/cancel", exportHandler.Cancel)
    }
    
    // Health check endpoint (outside /v1 group)
    router.GET("/health", handler.HealthCheck(db))
    
    // Start server
    if err := router.Run(":" + cfg.HTTPPort); err != nil {
        log.Fatal(err)
    }
}
```

### Shutdown Behavior

When the server receives a termination signal (SIGINT/SIGTERM), shutdown is handled as follows:

1. **Services are closed first** - `Close()` is called on ImportService and ExportService
2. **Workers abort immediately** - The `stopChan` is closed, signaling all workers to exit
3. **Job queues are closed** - Pending jobs in the queue are discarded
4. **HTTP server shuts down** - A 5-second timeout is given for in-flight requests

```go
// Shutdown sequence
importSvc.Close()  // Aborts workers, closes job queue
exportSvc.Close()  // Aborts workers, closes job queue

ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
server.Shutdown(ctx)  // Allow in-flight requests to complete
```

This ensures a fast shutdown - workers don't wait for queue to drain.

---

## Request Tracing

All requests are traced using the `X-Request-ID` header for debugging and correlation.

### Middleware Implementation

```go
// internal/middleware/request_id.go

const RequestIDHeader = "X-Request-ID"
const RequestIDKey = "request_id"

func RequestIDMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        requestID := c.GetHeader(RequestIDHeader)
        if requestID == "" {
            requestID = uuid.New().String()
        }
        c.Set(RequestIDKey, requestID)
        c.Header(RequestIDHeader, requestID)
        c.Next()
    }
}

func GetRequestID(c *gin.Context) string {
    if id, exists := c.Get(RequestIDKey); exists {
        return id.(string)
    }
    return ""
}
```

### Request ID in Async Jobs

For async import/export jobs, the request ID is captured at job creation and logged throughout the job lifecycle:

```go
// Handler captures request ID
requestID := middleware.GetRequestID(c)

// Service stores it with the task
task := importTask{
    jobID:     jobID,
    requestID: requestID,
    // ...
}

// Worker logs with request ID prefix
log.Printf("[request_id=%s][job_id=%s] processing import...", task.requestID, task.jobID)
```

This enables end-to-end tracing from the initial HTTP request through async job processing.

---

## Observability Logging

> **Assignment Requirement**: "log rows/sec, error_rate, duration"

### Structured Logging Pattern

```go
// internal/service/import_service.go

type ImportMetrics struct {
    JobID             string        `json:"job_id"`
    ResourceType      string        `json:"resource_type"`
    Mode              string        `json:"mode"`  // "insert" or "upsert"
    TotalRecords      int           `json:"total_records"`
    SuccessfulRecords int           `json:"successful_records"`
    FailedRecords     int           `json:"failed_records"`
    DurationMs        int64         `json:"duration_ms"`
    RowsPerSecond     float64       `json:"rows_per_sec"`
    ErrorRate         float64       `json:"error_rate"`
    BatchCount        int           `json:"batch_count"`
}

func (s *ImportService) logImportMetrics(job *domain.Job, result *ImportResult, duration time.Duration) {
    rowsPerSec := float64(result.TotalProcessed) / duration.Seconds()
    errorRate := float64(result.TotalFailed) / float64(result.TotalProcessed)
    
    metrics := ImportMetrics{
        JobID:             job.ID.String(),
        ResourceType:      job.ResourceType,
        Mode:              job.Mode,
        TotalRecords:      result.TotalProcessed,
        SuccessfulRecords: result.TotalSuccessful,
        FailedRecords:     result.TotalFailed,
        DurationMs:        duration.Milliseconds(),
        RowsPerSecond:     rowsPerSec,
        ErrorRate:         errorRate,
        BatchCount:        result.TotalProcessed / s.cfg.BatchSize,
    }
    
    // Structured log for monitoring/alerting
    s.logger.Info("import completed",
        slog.String("job_id", metrics.JobID),
        slog.String("resource_type", metrics.ResourceType),
        slog.String("mode", metrics.Mode),
        slog.Int("total_records", metrics.TotalRecords),
        slog.Int("successful_records", metrics.SuccessfulRecords),
        slog.Int("failed_records", metrics.FailedRecords),
        slog.Int64("duration_ms", metrics.DurationMs),
        slog.Float64("rows_per_sec", metrics.RowsPerSecond),
        slog.Float64("error_rate", metrics.ErrorRate),
    )
    
    // Alert on high error rate
    if errorRate > 0.1 { // >10% errors
        s.logger.Warn("high error rate detected",
            slog.String("job_id", metrics.JobID),
            slog.Float64("error_rate", metrics.ErrorRate),
        )
    }
}
```

### Batch-Level Logging

```go
func (s *ImportService) logBatchProgress(job *domain.Job, batchNum int, totalBatches int, batchResult *BatchResult, batchDuration time.Duration) {
    batchRowsPerSec := float64(batchResult.Successful+batchResult.Failed) / batchDuration.Seconds()
    
    s.logger.Debug("batch completed",
        slog.String("job_id", job.ID.String()),
        slog.Int("batch_number", batchNum+1),
        slog.Int("total_batches", totalBatches),
        slog.Int("successful_records", batchResult.Successful),
        slog.Int("failed_records", batchResult.Failed),
        slog.Float64("rows_per_sec", batchRowsPerSec),
    )
}
```

### Export Metrics

```go
type ExportMetrics struct {
    JobID         string  `json:"job_id"`
    ResourceType  string  `json:"resource_type"`
    Format        string  `json:"format"`
    TotalRecords  int     `json:"total_records"`
    DurationMs    int64   `json:"duration_ms"`
    RowsPerSecond float64 `json:"rows_per_sec"`
}

func (s *ExportService) logExportMetrics(job *domain.Job, totalRecords int, duration time.Duration) {
    rowsPerSec := float64(totalRecords) / duration.Seconds()
    
    s.logger.Info("export completed",
        slog.String("job_id", job.ID.String()),
        slog.String("resource_type", job.ResourceType),
        slog.String("format", job.Format),
        slog.Int("total_records", totalRecords),
        slog.Int64("duration_ms", duration.Milliseconds()),
        slog.Float64("rows_per_sec", rowsPerSec),
    )
}
```

### Log Output Example

```json
{
  "time": "2024-01-15T10:30:45Z",
  "level": "INFO",
  "msg": "import completed",
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "resource_type": "users",
  "mode": "insert",
  "total_records": 100000,
  "successful_records": 99850,
  "failed_records": 150,
  "duration_ms": 12500,
  "rows_per_sec": 8000.0,
  "error_rate": 0.0015
}
```

---

## Related Documents

- [Database Schema](./database-schema.md) - Table definitions and constraints
- [Validation Strategy](./validation-strategy.md) - CTE-based validation pattern
- [API Specification](./api-specification.md) - Endpoint details
