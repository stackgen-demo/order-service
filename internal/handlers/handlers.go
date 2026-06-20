package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"time"

	"github.com/appcd-dev/order-service/internal/checkout"
	"github.com/appcd-dev/order-service/internal/fault"
	"github.com/appcd-dev/order-service/internal/logger"
)

type HealthHandler struct{}

func (h HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	service := os.Getenv("DD_SERVICE")
	if service == "" {
		service = "order-service"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":          "ok",
		"service":         service,
		"uptime_seconds":  int(time.Since(startTime).Seconds()),
		"runtime":         "go",
	})
}

var startTime = time.Now()

type APIInfoHandler struct{}

func (h APIInfoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	service := os.Getenv("DD_SERVICE")
	if service == "" {
		service = "order-service"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"service": service,
		"runtime": "go",
		"endpoints": map[string]any{
			"health": "GET /health",
			"users": map[string]string{
				"list":   "GET /api/users",
				"create": "POST /api/users",
			},
			"orders": map[string]string{
				"list":   "GET /api/orders",
				"create": "POST /api/orders (X-Demo-Fault header selects fault mode)",
			},
			"demo_fault_header": fault.HeaderName,
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func logRequest(ctx context.Context, r *http.Request, status int, duration time.Duration) {
	fields := map[string]any{
		"http": map[string]any{
			"status_code": status,
			"method":      r.Method,
			"url":         r.URL.Path,
		},
		"duration_ms": duration.Milliseconds(),
	}

	if status >= 500 {
		logger.ErrorContext(ctx, "Request completed with server error", fields)
		return
	}
	logger.InfoContext(ctx, "Request completed", fields)
}

func withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}

			stack := string(debug.Stack())
			logger.ErrorContext(r.Context(), "Request panicked", map[string]any{
				"http": map[string]any{
					"status_code": http.StatusInternalServerError,
					"method":      r.Method,
					"url":         r.URL.Path,
				},
				"error": map[string]any{
					"kind":       "UnhandledPanic",
					"message":    fmt.Sprint(recovered),
					"root_cause": "Simulated unhandled panic in order creation path",
					"stack":      stack,
				},
			})

			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
		}()

		next.ServeHTTP(w, r)
	})
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		logRequest(r.Context(), r, recorder.status, time.Since(start))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func NewRouter(database *sql.DB, orchestrator *checkout.Orchestrator) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /health", withLogging(HealthHandler{}))
	mux.Handle("GET /api", withLogging(APIInfoHandler{}))
	mux.Handle("GET /api/users", withLogging(UsersListHandler{DB: database}))
	mux.Handle("POST /api/users", withLogging(UsersCreateHandler{DB: database}))
	mux.Handle("GET /api/orders", withLogging(OrdersListHandler{DB: database}))
	mux.Handle("POST /api/orders", withRecovery(withLogging(OrdersCreateHandler{DB: database, Checkout: orchestrator})))
	mux.Handle("/", withLogging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	})))

	return mux
}
