#!/bin/bash

# Test script for Bulk Import/Export API
# Usage: ./scripts/test-api.sh [BASE_URL]
#
# Prerequisites:
# - Server running (make run or docker-compose up)
# - jq installed for JSON parsing
# - Test data files in testdata/ directory

set -e

# Configuration
BASE_URL="${1:-http://localhost:8080}"
API_URL="${BASE_URL}/api/v1"
TESTDATA_DIR="$(dirname "$0")/../testdata"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Helper functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[PASS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[FAIL]${NC} $1"
}

log_header() {
    echo ""
    echo -e "${YELLOW}========================================${NC}"
    echo -e "${YELLOW}$1${NC}"
    echo -e "${YELLOW}========================================${NC}"
}

check_jq() {
    if ! command -v jq &> /dev/null; then
        log_error "jq is required but not installed. Please install jq."
        exit 1
    fi
}

check_health() {
    log_info "Checking server health at ${BASE_URL}..."
    
    local response
    response=$(curl -s -w "\n%{http_code}" "${BASE_URL}/health" 2>/dev/null)
    local http_code=$(echo "$response" | tail -n1)
    local body=$(echo "$response" | sed '$d')
    
    if [ "$http_code" = "200" ]; then
        log_success "Server is healthy"
        echo "$body" | jq '.'
        return 0
    else
        log_error "Server is not healthy (HTTP $http_code)"
        echo "$body"
        return 1
    fi
}

wait_for_import_job() {
    local job_id="$1"
    local max_attempts="${2:-60}"
    local attempt=0
    
    log_info "Waiting for import job $job_id to complete..."
    
    while [ $attempt -lt $max_attempts ]; do
        local response
        response=$(curl -s "${API_URL}/imports/${job_id}")
        local status=$(echo "$response" | jq -r '.status')
        
        case "$status" in
            "completed")
                log_success "Import job completed successfully"
                echo "$response" | jq '.'
                return 0
                ;;
            "completed_with_errors")
                log_warning "Import job completed with errors"
                echo "$response" | jq '.'
                return 0
                ;;
            "failed")
                log_error "Import job failed"
                echo "$response" | jq '.'
                return 1
                ;;
            "pending"|"processing")
                local processed=$(echo "$response" | jq -r '.processed_records // 0')
                local total=$(echo "$response" | jq -r '.total_records // 0')
                printf "\r  Status: %s, Progress: %s/%s records" "$status" "$processed" "$total"
                sleep 1
                ;;
            *)
                log_error "Unknown status: $status"
                echo "$response" | jq '.'
                return 1
                ;;
        esac
        
        attempt=$((attempt + 1))
    done
    
    echo ""
    log_error "Timeout waiting for import job to complete"
    return 1
}

wait_for_export_job() {
    local job_id="$1"
    local max_attempts="${2:-60}"
    local attempt=0
    
    log_info "Waiting for export job $job_id to complete..."
    
    while [ $attempt -lt $max_attempts ]; do
        local response
        response=$(curl -s "${API_URL}/exports/${job_id}")
        local status=$(echo "$response" | jq -r '.status')
        
        case "$status" in
            "completed")
                log_success "Export job completed successfully"
                echo "$response" | jq '.'
                return 0
                ;;
            "failed")
                log_error "Export job failed"
                echo "$response" | jq '.'
                return 1
                ;;
            "pending"|"processing")
                printf "\r  Status: %s" "$status"
                sleep 1
                ;;
            *)
                log_error "Unknown status: $status"
                echo "$response" | jq '.'
                return 1
                ;;
        esac
        
        attempt=$((attempt + 1))
    done
    
    echo ""
    log_error "Timeout waiting for export job to complete"
    return 1
}

# ============================================
# Test: Import Users (CSV)
# ============================================
test_import_users() {
    log_header "Test: Import Users (CSV)"
    
    local file="${TESTDATA_DIR}/users_huge.csv"
    if [ ! -f "$file" ]; then
        log_error "Test file not found: $file"
        return 1
    fi
    
    local idempotency_token=$(uuidgen | tr '[:upper:]' '[:lower:]')
    
    log_info "Uploading users CSV file..."
    local response
    response=$(curl -s -X POST "${API_URL}/imports" \
        -H "X-Request-ID: test-import-users-$(date +%s)" \
        -F "resource_type=users" \
        -F "idempotency_token=${idempotency_token}" \
        -F "file=@${file}")
    
    local job_id=$(echo "$response" | jq -r '.id')
    
    if [ "$job_id" = "null" ] || [ -z "$job_id" ]; then
        log_error "Failed to create import job"
        echo "$response" | jq '.'
        return 1
    fi
    
    log_success "Import job created: $job_id"
    echo "$response" | jq '.'
    
    # Wait for completion
    wait_for_import_job "$job_id" 120
    
    return $?
}

# ============================================
# Test: Import Articles (NDJSON)
# ============================================
test_import_articles() {
    log_header "Test: Import Articles (NDJSON)"
    
    local file="${TESTDATA_DIR}/articles_huge.ndjson"
    if [ ! -f "$file" ]; then
        log_error "Test file not found: $file"
        return 1
    fi
    
    local idempotency_token=$(uuidgen | tr '[:upper:]' '[:lower:]')
    
    log_info "Uploading articles NDJSON file..."
    local response
    response=$(curl -s -X POST "${API_URL}/imports" \
        -H "X-Request-ID: test-import-articles-$(date +%s)" \
        -F "resource_type=articles" \
        -F "idempotency_token=${idempotency_token}" \
        -F "file=@${file}")
    
    local job_id=$(echo "$response" | jq -r '.id')
    
    if [ "$job_id" = "null" ] || [ -z "$job_id" ]; then
        log_error "Failed to create import job"
        echo "$response" | jq '.'
        return 1
    fi
    
    log_success "Import job created: $job_id"
    echo "$response" | jq '.'
    
    # Wait for completion
    wait_for_import_job "$job_id" 180
    
    return $?
}

# ============================================
# Test: Import Comments (NDJSON)
# ============================================
test_import_comments() {
    log_header "Test: Import Comments (NDJSON)"
    
    local file="${TESTDATA_DIR}/comments_huge.ndjson"
    if [ ! -f "$file" ]; then
        log_error "Test file not found: $file"
        return 1
    fi
    
    local idempotency_token=$(uuidgen | tr '[:upper:]' '[:lower:]')
    
    log_info "Uploading comments NDJSON file..."
    local response
    response=$(curl -s -X POST "${API_URL}/imports" \
        -H "X-Request-ID: test-import-comments-$(date +%s)" \
        -F "resource_type=comments" \
        -F "idempotency_token=${idempotency_token}" \
        -F "file=@${file}")
    
    local job_id=$(echo "$response" | jq -r '.id')
    
    if [ "$job_id" = "null" ] || [ -z "$job_id" ]; then
        log_error "Failed to create import job"
        echo "$response" | jq '.'
        return 1
    fi
    
    log_success "Import job created: $job_id"
    echo "$response" | jq '.'
    
    # Wait for completion
    wait_for_import_job "$job_id" 240
    
    return $?
}

# ============================================
# Test: Async Export (NDJSON)
# ============================================
test_async_export_ndjson() {
    log_header "Test: Async Export Users (NDJSON)"
    
    local idempotency_token=$(uuidgen | tr '[:upper:]' '[:lower:]')
    
    log_info "Creating async export job for users (NDJSON)..."
    local response
    response=$(curl -s -X POST "${API_URL}/exports" \
        -H "Content-Type: application/json" \
        -H "X-Request-ID: test-export-users-ndjson-$(date +%s)" \
        -d "{
            \"resource_type\": \"users\",
            \"format\": \"ndjson\",
            \"idempotency_token\": \"${idempotency_token}\"
        }")
    
    local job_id=$(echo "$response" | jq -r '.id')
    
    if [ "$job_id" = "null" ] || [ -z "$job_id" ]; then
        log_error "Failed to create export job"
        echo "$response" | jq '.'
        return 1
    fi
    
    log_success "Export job created: $job_id"
    echo "$response" | jq '.'
    
    # Wait for completion
    if ! wait_for_export_job "$job_id" 120; then
        return 1
    fi
    
    log_success "Export job completed successfully"
    
    return 0
}

# ============================================
# Test: Async Export (CSV)
# ============================================
test_async_export_csv() {
    log_header "Test: Async Export Users (CSV)"
    
    local idempotency_token=$(uuidgen | tr '[:upper:]' '[:lower:]')
    
    log_info "Creating async export job for users (CSV)..."
    local response
    response=$(curl -s -X POST "${API_URL}/exports" \
        -H "Content-Type: application/json" \
        -H "X-Request-ID: test-export-users-csv-$(date +%s)" \
        -d "{
            \"resource_type\": \"users\",
            \"format\": \"csv\",
            \"idempotency_token\": \"${idempotency_token}\"
        }")
    
    local job_id=$(echo "$response" | jq -r '.id')
    
    if [ "$job_id" = "null" ] || [ -z "$job_id" ]; then
        log_error "Failed to create export job"
        echo "$response" | jq '.'
        return 1
    fi
    
    log_success "Export job created: $job_id"
    echo "$response" | jq '.'
    
    # Wait for completion
    if ! wait_for_export_job "$job_id" 120; then
        return 1
    fi
    
    log_success "Export job completed successfully"
    
    return 0
}

# ============================================
# Test: Export Articles (verifies streaming internally)
# ============================================
test_export_articles() {
    log_header "Test: Export Articles (NDJSON)"
    
    local idempotency_token=$(uuidgen | tr '[:upper:]' '[:lower:]')
    
    log_info "Creating async export job for articles (NDJSON)..."
    local response
    response=$(curl -s -X POST "${API_URL}/exports" \
        -H "Content-Type: application/json" \
        -H "X-Request-ID: test-export-articles-$(date +%s)" \
        -d "{
            \"resource_type\": \"articles\",
            \"format\": \"ndjson\",
            \"idempotency_token\": \"${idempotency_token}\"
        }")
    
    local job_id=$(echo "$response" | jq -r '.id')
    
    if [ "$job_id" = "null" ] || [ -z "$job_id" ]; then
        log_error "Failed to create export job"
        echo "$response" | jq '.'
        return 1
    fi
    
    log_success "Export job created: $job_id"
    
    # Wait for completion
    if ! wait_for_export_job "$job_id" 120; then
        return 1
    fi
    
    log_success "Export job completed successfully"
    
    return 0
}

# ============================================
# Test: Export Comments (verifies streaming internally)
# ============================================
test_export_comments() {
    log_header "Test: Export Comments (CSV)"
    
    local idempotency_token=$(uuidgen | tr '[:upper:]' '[:lower:]')
    
    log_info "Creating async export job for comments (CSV)..."
    local response
    response=$(curl -s -X POST "${API_URL}/exports" \
        -H "Content-Type: application/json" \
        -H "X-Request-ID: test-export-comments-$(date +%s)" \
        -d "{
            \"resource_type\": \"comments\",
            \"format\": \"csv\",
            \"idempotency_token\": \"${idempotency_token}\"
        }")
    
    local job_id=$(echo "$response" | jq -r '.id')
    
    if [ "$job_id" = "null" ] || [ -z "$job_id" ]; then
        log_error "Failed to create export job"
        echo "$response" | jq '.'
        return 1
    fi
    
    log_success "Export job created: $job_id"
    
    # Wait for completion
    if ! wait_for_export_job "$job_id" 120; then
        return 1
    fi
    
    log_success "Export job completed successfully"
    
    return 0
}

# ============================================
# Test: Streaming Export Users (NDJSON)
# ============================================
test_streaming_export_users_ndjson() {
    log_header "Test: Streaming Export Users (NDJSON)"
    
    log_info "Streaming users export in NDJSON format..."
    local response
    response=$(curl -s -w "\n%{http_code}" "${API_URL}/exports?resource=users&format=ndjson" \
        -H "X-Request-ID: test-stream-users-ndjson-$(date +%s)")
    
    local http_code=$(echo "$response" | tail -n1)
    local content=$(echo "$response" | sed '$d')
    
    if [ "$http_code" = "200" ]; then
        local line_count=$(echo "$content" | wc -l | tr -d ' ')
        log_success "Streamed $line_count users in NDJSON format"
        echo "First 3 records:"
        echo "$content" | head -3 | jq '.'
        return 0
    else
        log_error "Streaming export failed (HTTP $http_code)"
        echo "$content"
        return 1
    fi
}

# ============================================
# Test: Streaming Export Users (CSV)
# ============================================
test_streaming_export_users_csv() {
    log_header "Test: Streaming Export Users (CSV)"
    
    log_info "Streaming users export in CSV format..."
    local response
    response=$(curl -s -w "\n%{http_code}" "${API_URL}/exports?resource=users&format=csv" \
        -H "X-Request-ID: test-stream-users-csv-$(date +%s)")
    
    local http_code=$(echo "$response" | tail -n1)
    local content=$(echo "$response" | sed '$d')
    
    if [ "$http_code" = "200" ]; then
        local line_count=$(echo "$content" | wc -l | tr -d ' ')
        log_success "Streamed $line_count lines (including header) in CSV format"
        echo "First 5 lines:"
        echo "$content" | head -5
        return 0
    else
        log_error "Streaming export failed (HTTP $http_code)"
        echo "$content"
        return 1
    fi
}

# ============================================
# Test: Streaming Export Articles (NDJSON)
# ============================================
test_streaming_export_articles() {
    log_header "Test: Streaming Export Articles (NDJSON)"
    
    log_info "Streaming articles export in NDJSON format..."
    local response
    response=$(curl -s -w "\n%{http_code}" "${API_URL}/exports?resource=articles&format=ndjson" \
        -H "X-Request-ID: test-stream-articles-$(date +%s)")
    
    local http_code=$(echo "$response" | tail -n1)
    local content=$(echo "$response" | sed '$d')
    
    if [ "$http_code" = "200" ]; then
        local line_count=$(echo "$content" | wc -l | tr -d ' ')
        log_success "Streamed $line_count articles in NDJSON format"
        echo "First 2 records:"
        echo "$content" | head -2 | jq '.'
        return 0
    else
        log_error "Streaming export failed (HTTP $http_code)"
        echo "$content"
        return 1
    fi
}

# ============================================
# Test: Streaming Export Comments (NDJSON)
# ============================================
test_streaming_export_comments() {
    log_header "Test: Streaming Export Comments (NDJSON)"
    
    log_info "Streaming comments export in NDJSON format..."
    local response
    response=$(curl -s -w "\n%{http_code}" "${API_URL}/exports?resource=comments&format=ndjson" \
        -H "X-Request-ID: test-stream-comments-$(date +%s)")
    
    local http_code=$(echo "$response" | tail -n1)
    local content=$(echo "$response" | sed '$d')
    
    if [ "$http_code" = "200" ]; then
        local line_count=$(echo "$content" | wc -l | tr -d ' ')
        log_success "Streamed $line_count comments in NDJSON format"
        echo "First 2 records:"
        echo "$content" | head -2 | jq '.'
        return 0
    else
        log_error "Streaming export failed (HTTP $http_code)"
        echo "$content"
        return 1
    fi
}

# ============================================
# Test: Idempotency
# ============================================
test_idempotency() {
    log_header "Test: Idempotency (duplicate import request)"
    
    local idempotency_token=$(uuidgen | tr '[:upper:]' '[:lower:]')
    
    # Create a small test file
    local temp_file=$(mktemp)
    echo 'id,email,name,role,active,created_at,updated_at' > "$temp_file"
    echo "$(uuidgen | tr '[:upper:]' '[:lower:]'),test@example.com,Test User,user,true,2024-01-01T00:00:00Z,2024-01-01T00:00:00Z" >> "$temp_file"
    
    log_info "Sending first import request..."
    local response1
    response1=$(curl -s -X POST "${API_URL}/imports" \
        -F "resource_type=users" \
        -F "idempotency_token=${idempotency_token}" \
        -F "file=@${temp_file}")
    
    local job_id1=$(echo "$response1" | jq -r '.id')
    log_info "First request job_id: $job_id1"
    
    log_info "Sending duplicate import request with same idempotency token..."
    local response2
    response2=$(curl -s -X POST "${API_URL}/imports" \
        -F "resource_type=users" \
        -F "idempotency_token=${idempotency_token}" \
        -F "file=@${temp_file}")
    
    local job_id2=$(echo "$response2" | jq -r '.id')
    log_info "Second request job_id: $job_id2"
    
    rm -f "$temp_file"
    
    if [ "$job_id1" = "$job_id2" ]; then
        log_success "Idempotency works correctly - same job_id returned: $job_id1"
        return 0
    else
        log_error "Idempotency failed - different job_ids returned: $job_id1 vs $job_id2"
        return 1
    fi
}

# ============================================
# Test: Validation Errors
# ============================================
test_validation_errors() {
    log_header "Test: Validation Errors"
    
    log_info "Testing missing resource_type..."
    local response
    response=$(curl -s -X POST "${API_URL}/imports" \
        -F "file=@${TESTDATA_DIR}/users_huge.csv")
    
    local error=$(echo "$response" | jq -r '.error')
    if [ "$error" != "null" ] && [ -n "$error" ]; then
        log_success "Got expected error: $error"
    else
        log_error "Expected validation error but got: $response"
        return 1
    fi
    
    log_info "Testing invalid resource_type..."
    response=$(curl -s -X POST "${API_URL}/imports" \
        -F "resource_type=invalid" \
        -F "file=@${TESTDATA_DIR}/users_huge.csv")
    
    error=$(echo "$response" | jq -r '.error')
    if [ "$error" != "null" ] && [ -n "$error" ]; then
        log_success "Got expected error: $error"
    else
        log_error "Expected validation error but got: $response"
        return 1
    fi
    
    return 0
}

# ============================================
# Main Execution
# ============================================
main() {
    echo ""
    echo -e "${BLUE}╔════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║       Bulk Import/Export API Test Suite                    ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo "Base URL: ${BASE_URL}"
    echo "API URL:  ${API_URL}"
    echo "Test Data: ${TESTDATA_DIR}"
    echo ""
    
    check_jq
    
    if ! check_health; then
        log_error "Server is not available. Please start the server first."
        exit 1
    fi
    
    local failed=0
    
    # Test validation errors first (doesn't require data)
    test_validation_errors || ((failed++))
    
    # Test idempotency
    test_idempotency || ((failed++))
    
    # Import tests (order matters due to FK constraints!)
    test_import_users || ((failed++))
    test_import_articles || ((failed++))
    test_import_comments || ((failed++))
    
    # Export tests (async exports with internal streaming)
    test_async_export_ndjson || ((failed++))
    test_async_export_csv || ((failed++))
    test_export_articles || ((failed++))
    test_export_comments || ((failed++))
    
    # Streaming export tests (direct HTTP streaming)
    test_streaming_export_users_ndjson || ((failed++))
    test_streaming_export_users_csv || ((failed++))
    test_streaming_export_articles || ((failed++))
    test_streaming_export_comments || ((failed++))
    
    # Summary
    log_header "Test Summary"
    
    if [ $failed -eq 0 ]; then
        log_success "All tests passed!"
        exit 0
    else
        log_error "$failed test(s) failed"
        exit 1
    fi
}

# Run main function
main "$@"
