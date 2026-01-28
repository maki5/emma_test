# API Specification

## Base URL

```
http://localhost:8080/v1
```

## Authentication

> Note: Authentication is not implemented in this version. All endpoints are publicly accessible for development purposes.

---

## Request Tracing

All endpoints support the `X-Request-ID` header for distributed tracing and debugging.

**Behavior:**
- If a request includes an `X-Request-ID` header, that value is used for tracing
- If no header is provided, the server generates a UUID automatically
- The request ID is included in response headers and server logs
- For async jobs (imports/exports), the request ID is logged throughout the job lifecycle

**Example:**
```http
POST /v1/imports HTTP/1.1
X-Request-ID: req-12345-abcde
Content-Type: multipart/form-data
```

**Response Header:**
```http
HTTP/1.1 202 Accepted
X-Request-ID: req-12345-abcde
```

---

## Health Check

### GET /health

Health check endpoint for monitoring and load balancers.

**Request:**
```http
GET /health HTTP/1.1
```

**Response (Healthy):**
```json
{
    "status": "healthy",
    "version": "1.0.0",
    "timestamp": "2026-01-28T10:30:00Z",
    "checks": {
        "database": "ok",
        "disk_space": "ok"
    }
}
```

**Response (Unhealthy):**
```json
{
    "status": "unhealthy",
    "version": "1.0.0",
    "timestamp": "2026-01-28T10:30:00Z",
    "checks": {
        "database": "connection refused",
        "disk_space": "ok"
    }
}
```

| Status Code | Condition |
|-------------|-----------|
| 200 OK | All checks pass |
| 503 Service Unavailable | Any critical check fails |

---

## Import Order Dependency

**IMPORTANT**: Due to foreign key constraints, imports must be performed in the following order:

```
1. Users      → No dependencies
2. Articles   → Depends on Users (author_id)
3. Comments   → Depends on Articles (article_id) and Users (user_id)
```

If you attempt to import articles before users, or comments before articles/users, the records will fail with `invalid_author_id`, `invalid_article_id`, or `invalid_user_id` errors.

**Recommended Import Workflow:**
1. Import all users first, wait for completion
2. Import all articles, wait for completion
3. Import all comments

---

## Import Endpoints

### POST /v1/imports (Multipart)

Upload a file directly for import.

**Request:**
```http
POST /v1/imports HTTP/1.1
Content-Type: multipart/form-data
Idempotency-Key: unique-key-123

file: <binary file data>
resource: users
```

**Form Fields:**
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `file` | file | Yes | The file to import (CSV or NDJSON) |
| `resource` | string | Yes | Resource type: `users`, `articles`, `comments` |
| `mode` | string | No | Import mode: `insert` (default) or `upsert` |

### ID and Upsert Behavior

All import records should include an `id` field (UUID). This ID is used:
- **Insert mode**: As the primary key for new records
- **Upsert mode**: To identify existing records for update

| Resource | Upsert Key | Behavior |
|----------|------------|----------|
| Users | `id` or `email` | Updates existing user by email if found, otherwise inserts with provided id |
| Articles | `id` or `slug` | Updates existing article by slug if found, otherwise inserts with provided id |
| Comments | `id` | Updates existing comment by id if found, otherwise inserts |

**Response:**
```json
{
    "job_id": "550e8400-e29b-41d4-a716-446655440000",
    "status": "pending",
    "message": "Import job created successfully"
}
```

---

### POST /v1/imports (JSON - Remote URL)

Import a file from a remote URL.

**Request:**
```http
POST /v1/imports HTTP/1.1
Content-Type: application/json
Idempotency-Key: unique-key-456

{
    "resource": "comments",
    "file_url": "https://example.com/data/comments.ndjson",
    "format": "ndjson"
}
```

**Body Fields:**
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `resource` | string | Yes | Resource type: `users`, `articles`, `comments` |
| `file_url` | string | Yes | URL to download the file from |
| `format` | string | No | File format: `csv`, `ndjson` (auto-detected if not provided) |
| `mode` | string | No | Import mode: `insert` (default) or `upsert` |

> **Note**: See [ID and Upsert Behavior](#id-and-upsert-behavior) above for details on how records are identified.

**Response:**
```json
{
    "job_id": "550e8400-e29b-41d4-a716-446655440001",
    "status": "pending",
    "message": "Import job created successfully"
}
```

---

### GET /v1/imports/:id

Get the status of an import job.

**Request:**
```http
GET /v1/imports/550e8400-e29b-41d4-a716-446655440000 HTTP/1.1
```

**Response (Processing):**
```json
{
    "job_id": "550e8400-e29b-41d4-a716-446655440000",
    "resource_type": "users",
    "status": "processing",
    "total_records": 10000,
    "processed_records": 5500,
    "successful_records": 5485,
    "error_records": 15,
    "started_at": "2024-01-15T10:30:00Z"
}
```

**Response (Completed):**
```json
{
    "job_id": "550e8400-e29b-41d4-a716-446655440000",
    "resource_type": "users",
    "status": "completed",
    "total_records": 10000,
    "processed_records": 10000,
    "successful_records": 9985,
    "error_records": 15,
    "errors": [
        {
            "row": 42,
            "field": "email",
            "value": "duplicate@example.com",
            "reason": "duplicate_email"
        },
        {
            "row": 156,
            "field": "email",
            "value": "invalid-email",
            "reason": "invalid_email_format"
        }
    ],
    "started_at": "2024-01-15T10:30:00Z",
    "completed_at": "2024-01-15T10:30:45Z"
}
```

**Response (Completed with Errors):**
```json
{
    "job_id": "550e8400-e29b-41d4-a716-446655440000",
    "resource_type": "users",
    "status": "completed_with_errors",
    "total_records": 10000,
    "processed_records": 10000,
    "successful_records": 7985,
    "error_records": 2015,
    "started_at": "2024-01-15T10:30:00Z",
    "completed_at": "2024-01-15T10:30:45Z"
}
```

> **Note**: Status `completed_with_errors` means:
> - Some records failed validation but processing continued
> - Successfully processed records have their data persisted
> - `error_records` shows the count of records that failed

**Response (Failed):**
```json
{
    "job_id": "550e8400-e29b-41d4-a716-446655440000",
    "resource_type": "users",
    "status": "failed",
    "total_records": 10000,
    "processed_records": 0,
    "successful_records": 0,
    "error_records": 0,
    "failure_reason": "failed to download file: connection refused",
    "started_at": "2024-01-15T10:30:00Z",
    "completed_at": "2024-01-15T10:30:02Z"
}
```

> **Note**: Status `failed` is used when the import cannot start or continue at all (e.g., file download failed, invalid format, all batches failed). This is different from `completed_with_errors` where some batches succeeded.

---

### GET /v1/imports

List all import jobs.

**Request:**
```http
GET /v1/imports HTTP/1.1
```

**Response:**
```json
{
    "items": [
        {
            "job_id": "550e8400-e29b-41d4-a716-446655440000",
            "resource_type": "users",
            "status": "processing",
            "total_records": 10000,
            "processed_records": 5500,
            "started_at": "2024-01-15T10:30:00Z"
        },
        {
            "job_id": "550e8400-e29b-41d4-a716-446655440001",
            "resource_type": "articles",
            "status": "pending",
            "started_at": "2024-01-15T10:31:00Z"
        }
    ],
    "total": 2
}
```

---

### POST /v1/imports/:id/cancel

Cancel a running import job. Only jobs with status `pending` or `processing` can be cancelled.

**Request:**
```http
POST /v1/imports/550e8400-e29b-41d4-a716-446655440000/cancel HTTP/1.1
```

**Response (Success):**
```json
{
    "job_id": "550e8400-e29b-41d4-a716-446655440000",
    "status": "cancelled",
    "message": "Import job cancelled successfully",
    "processed_records": 5500,
    "successful_records": 5485,
    "error_records": 15,
    "cancelled_at": "2024-01-15T10:32:00Z"
}
```

**Response (Cannot Cancel - Already Completed):**
```json
{
    "error": "invalid_state",
    "message": "Cannot cancel job with status 'completed'",
    "job_id": "550e8400-e29b-41d4-a716-446655440000",
    "current_status": "completed"
}
```

**Cancellation Behavior:**
- The current batch will complete (transactions are atomic per batch)
- Remaining batches will be skipped
- Already inserted records remain in the database
- Job status changes to `cancelled`

---

## Export Endpoints

### GET /v1/exports (Streaming)

Stream export data directly in the response.

**Request:**
```http
GET /v1/exports?resource=users&format=ndjson HTTP/1.1
```

**Query Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `resource` | string | Yes | Resource type: `users`, `articles`, `comments` |
| `format` | string | No | Output format: `ndjson` (default), `csv` |
| `fields` | string | No | Comma-separated list of fields to include |
| `filter[field]` | string | No | Filter by field value |

**Response:**
```http
HTTP/1.1 200 OK
Content-Type: application/x-ndjson
Transfer-Encoding: chunked

{"id":"550e8400-e29b-41d4-a716-446655440000","email":"user1@example.com","name":"User One"}
{"id":"550e8400-e29b-41d4-a716-446655440001","email":"user2@example.com","name":"User Two"}
{"id":"550e8400-e29b-41d4-a716-446655440002","email":"user3@example.com","name":"User Three"}
...
```

---

### POST /v1/exports (Async)

Create an async export job that generates a downloadable file.

**Request:**
```http
POST /v1/exports HTTP/1.1
Content-Type: application/json

{
    "resource": "users",
    "format": "ndjson",
    "filters": {
        "role": "admin"
    },
    "fields": ["id", "email", "name", "role"]
}
```

**Body Fields:**
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `resource` | string | Yes | Resource type: `users`, `articles`, `comments` |
| `format` | string | No | Output format: `ndjson` (default), `csv`, `json` |
| `filters` | object | No | Key-value pairs for filtering |
| `fields` | array | No | List of fields to include |

**Response:**
```json
{
    "job_id": "550e8400-e29b-41d4-a716-446655440100",
    "status": "pending",
    "message": "Export job created successfully"
}
```

---

### GET /v1/exports/:id

Get the status of an async export job.

**Request:**
```http
GET /v1/exports/550e8400-e29b-41d4-a716-446655440100 HTTP/1.1
```

**Response (Processing):**
```json
{
    "job_id": "550e8400-e29b-41d4-a716-446655440100",
    "resource_type": "users",
    "format": "ndjson",
    "status": "processing",
    "started_at": "2024-01-15T10:35:00Z"
}
```

**Response (Completed):**
```json
{
    "job_id": "550e8400-e29b-41d4-a716-446655440100",
    "resource_type": "users",
    "format": "ndjson",
    "status": "completed",
    "record_count": 150,
    "file_path": "./exports/users-export-2024-01-15-550e8400.ndjson",
    "download_url": "/v1/exports/550e8400-e29b-41d4-a716-446655440100/download",
    "started_at": "2024-01-15T10:35:00Z",
    "completed_at": "2024-01-15T10:35:02Z"
}
```

**Response (Failed):**
```json
{
    "job_id": "550e8400-e29b-41d4-a716-446655440100",
    "resource_type": "users",
    "format": "ndjson",
    "status": "failed",
    "failure_reason": "query failed: connection timeout after 30s",
    "started_at": "2024-01-15T10:35:00Z",
    "completed_at": "2024-01-15T10:35:30Z"
}
```

> **Note**: When export fails, no partial file is left behind - it is cleaned up automatically.

> **Note**: Export files are stored at `EXPORT_FILE_PATH` (default: `./exports`) and are retained permanently.

---

### POST /v1/exports/:id/cancel

Cancel a running async export job.

**Request:**
```http
POST /v1/exports/550e8400-e29b-41d4-a716-446655440100/cancel HTTP/1.1
```

**Response (Success):**
```json
{
    "job_id": "550e8400-e29b-41d4-a716-446655440100",
    "status": "cancelled",
    "message": "Export job cancelled successfully",
    "cancelled_at": "2024-01-15T10:35:30Z"
}
```

---

### GET /v1/exports/:id/download

Download the generated export file.

**Request:**
```http
GET /v1/exports/550e8400-e29b-41d4-a716-446655440100/download HTTP/1.1
```

**Response:**
```http
HTTP/1.1 200 OK
Content-Type: application/x-ndjson
Content-Disposition: attachment; filename="users-export-2024-01-15.ndjson"

{"id":"550e8400-e29b-41d4-a716-446655440000","email":"admin1@example.com","name":"Admin One","role":"admin"}
{"id":"550e8400-e29b-41d4-a716-446655440001","email":"admin2@example.com","name":"Admin Two","role":"admin"}
...
```

---

## Idempotency

Both import and async export endpoints support idempotency via the `Idempotency-Key` header.

**Supported Endpoints:**
- `POST /v1/imports` - Bulk import jobs
- `POST /v1/exports` - Async export jobs

**Behavior:**
- First request: Creates a new job, stores the key → **202 Accepted**
- Duplicate request (same key): Returns existing job → **200 OK**

**Status Code Rationale:**
| Response | Status | Meaning |
|----------|--------|--------|
| New job created | 202 Accepted | Resource created, processing in background |
| Existing job found | 200 OK | Resource already exists, returning current state |

**Import Example:**
```http
POST /v1/imports HTTP/1.1
Idempotency-Key: import-users-batch-001
Content-Type: multipart/form-data

file: <binary data>
resource: users
```

**Export Example:**
```http
POST /v1/exports HTTP/1.1
Content-Type: application/json
Idempotency-Key: export-articles-2026-01-28

{
    "resource": "articles",
    "format": "ndjson"
}
```

**Race Condition Handling:**

If two concurrent requests arrive with the same idempotency key:
1. First request wins (creates job)
2. Second request hits UNIQUE constraint
3. Server detects constraint violation and returns existing job

This ensures exactly-once semantics even under concurrent retries.

---

## Error Responses

### 400 Bad Request

```json
{
    "error": "validation_error",
    "message": "Invalid resource type",
    "details": {
        "field": "resource",
        "value": "invalid",
        "allowed": ["users", "articles", "comments"]
    }
}
```

### 404 Not Found

```json
{
    "error": "not_found",
    "message": "Job not found",
    "job_id": "550e8400-e29b-41d4-a716-446655440999"
}
```

### 409 Conflict (Idempotency)

```json
{
    "error": "idempotency_conflict",
    "message": "Request with this idempotency key is already being processed",
    "existing_job_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

### 500 Internal Server Error

```json
{
    "error": "internal_error",
    "message": "An unexpected error occurred",
    "request_id": "req-12345"
}
```

---

## Data Formats

### Users CSV

```csv
id,email,name,role,active,created_at,updated_at
550e8400-e29b-41d4-a716-446655440000,user@example.com,John Doe,user,true,2024-01-15T10:00:00Z,2024-01-15T10:00:00Z
```

### Users NDJSON

```json
{"id":"550e8400-e29b-41d4-a716-446655440000","email":"user@example.com","name":"John Doe","role":"user","active":true,"created_at":"2024-01-15T10:00:00Z","updated_at":"2024-01-15T10:00:00Z"}
```

### Articles NDJSON

```json
{"id":"550e8400-e29b-41d4-a716-446655440000","slug":"my-article","title":"My Article","description":"A description","body":"Article content here","author_id":"550e8400-e29b-41d4-a716-446655440001","tags":["go","tutorial"],"published_at":"2024-01-15T12:00:00Z","status":"published","created_at":"2024-01-15T10:00:00Z","updated_at":"2024-01-15T10:00:00Z"}
```

### Comments NDJSON

```json
{"id":"550e8400-e29b-41d4-a716-446655440000","body":"This is a comment","article_id":"550e8400-e29b-41d4-a716-446655440001","user_id":"550e8400-e29b-41d4-a716-446655440002","created_at":"2024-01-15T10:00:00Z"}
```

---

## Swagger Documentation

When the server is running, access Swagger UI at:

```
http://localhost:8080/swagger/index.html
```

---

## Related Documents

- [Testing Strategy](./testing-strategy.md) - Curl examples and test scripts
- [Validation Strategy](./validation-strategy.md) - Error response details
- [Development Setup](./development-setup.md) - Running the server
