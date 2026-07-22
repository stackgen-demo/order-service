package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/appcd-dev/order-service/internal/logger"
)

// ErrorResponse wraps error details with context for observability
type ErrorResponse struct {
	Error   string            `json:"error"`
	Message string            `json:"message,omitempty"`
	Code    string            `json:"code,omitempty"`
	Details map[string]string `json:"details,omitempty"`
}

// RespondError writes an error response with enhanced logging and observability
func RespondError(ctx context.Context, w http.ResponseWriter, r *http.Request, status int, errorType string, message string, details map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	response := ErrorResponse{
		Error:   errorType,
		Message: message,
		Code:    fmt.Sprintf("HTTP_%d", status),
		Details: details,
	}

	// Log error with structured context for observability
	logFields := map[string]any{
		"http": map[string]any{
			"status_code": status,
			"method":      r.Method,
			"url":         r.URL.Path,
			"query":       r.URL.RawQuery,
		},
		"error": map[string]any{
			"type":    errorType,
			"message": message,
		},
	}

	if details != nil {
		logFields["error"].(map[string]any)["details"] = details
	}

	logger.ErrorContext(ctx, "HTTP error response", logFields)
	_ = json.NewEncoder(w).Encode(response)
}

// RespondInternalError is a convenience handler for 500-level errors
func RespondInternalError(ctx context.Context, w http.ResponseWriter, r *http.Request, originalErr error, operation string) {
	details := map[string]string{
		"operation": operation,
	}
	if originalErr != nil {
		details["underlying_error"] = originalErr.Error()
	}

	RespondError(ctx, w, r, http.StatusInternalServerError, "InternalError", "Internal server error", details)
}
