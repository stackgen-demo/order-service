package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/appcd-dev/order-service/internal/fault"
	"github.com/appcd-dev/order-service/internal/logger"
)

type OrdersListHandler struct {
	DB *sql.DB
}

func (h OrdersListHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(`SELECT id, amount, status, created_at FROM orders ORDER BY id`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	defer rows.Close()

	orders := make([]map[string]any, 0)
	for rows.Next() {
		var id int
		var amount float64
		var status, createdAt string
		if err := rows.Scan(&id, &amount, &status, &createdAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		orders = append(orders, map[string]any{
			"id":         id,
			"amount":     amount,
			"status":     status,
			"created_at": createdAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": orders})
}

type OrdersCreateHandler struct {
	DB *sql.DB
}

type createOrderRequest struct {
	CustomerEmail string  `json:"customer_email"`
	TotalAmount   float64 `json:"total_amount"`
	Status        string  `json:"status"`
}

// ServeHTTP handles POST /api/orders.
//
// Default (no X-Demo-Fault header): schema mismatch 500 for RCA demos.
// Other modes are selected via X-Demo-Fault on the request (see internal/fault).
func (h OrdersCreateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mode, err := fault.ModeFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown demo fault"})
		return
	}

	var req createOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	if strings.TrimSpace(req.CustomerEmail) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "customer_email and total_amount are required",
		})
		return
	}

	status := req.Status
	if status == "" {
		status = "pending"
	}

	switch mode {
	case fault.ModeDependency:
		h.respondDependencyFailure(w, r, req, status)
		return
	case fault.ModeTimeout:
		h.respondDownstreamTimeout(w, r, req, status)
		return
	case fault.ModePanic:
		panic("demo payment processor panic")
	case fault.ModeLocked:
		h.createWithLockedContention(w, r, req, status)
		return
	case fault.ModeHealthy:
		h.createHealthyOrder(w, r, req, status)
		return
	default:
		h.createSchemaMismatchOrder(w, r, req, status)
	}
}

func (h OrdersCreateHandler) createSchemaMismatchOrder(w http.ResponseWriter, r *http.Request, req createOrderRequest, status string) {
	_, err := h.DB.Exec(
		`INSERT INTO orders (customer_email, total_amount, status) VALUES (?, ?, ?)`,
		req.CustomerEmail,
		req.TotalAmount,
		status,
	)
	if err != nil {
		logOrderFault(r, http.StatusInternalServerError, "Order creation failed: database schema mismatch", map[string]any{
			"kind":       "DatabaseSchemaMismatch",
			"message":    err.Error(),
			"root_cause": "Application expects orders.customer_email and orders.total_amount but DB schema only has amount and status",
		}, map[string]any{
			"customer_email": req.CustomerEmail,
			"total_amount":   req.TotalAmount,
			"status":         status,
			"demo_fault":     fault.ModeSchema,
		})

		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "internal server error",
			"hint":  "orders table schema does not match application expectations",
		})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{
		"customer_email": req.CustomerEmail,
		"total_amount":   req.TotalAmount,
		"status":         status,
	}})
}

func (h OrdersCreateHandler) createHealthyOrder(w http.ResponseWriter, r *http.Request, req createOrderRequest, status string) {
	result, err := h.DB.Exec(
		`INSERT INTO orders (amount, status) VALUES (?, ?)`,
		req.TotalAmount,
		status,
	)
	if err != nil {
		logOrderFault(r, http.StatusInternalServerError, "Order creation failed", map[string]any{
			"kind":    "DatabaseError",
			"message": err.Error(),
		}, map[string]any{"demo_fault": fault.ModeHealthy})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	id, _ := result.LastInsertId()
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{
		"id":             id,
		"customer_email": req.CustomerEmail,
		"total_amount":   req.TotalAmount,
		"status":         status,
	}})
}

func (h OrdersCreateHandler) respondDependencyFailure(w http.ResponseWriter, r *http.Request, req createOrderRequest, status string) {
	time.Sleep(150 * time.Millisecond)

	logOrderFault(r, http.StatusBadGateway, "Order creation failed: downstream payment unavailable", map[string]any{
		"kind":       "DownstreamPaymentFailure",
		"message":    "payment-service returned connection refused",
		"root_cause": "Simulated payment-service outage; order-service cannot authorize charge",
	}, map[string]any{
		"customer_email": req.CustomerEmail,
		"total_amount":   req.TotalAmount,
		"status":         status,
		"demo_fault":     fault.ModeDependency,
	})

	writeJSON(w, http.StatusBadGateway, map[string]any{
		"error": "payment service unavailable",
	})
}

func (h OrdersCreateHandler) respondDownstreamTimeout(w http.ResponseWriter, r *http.Request, req createOrderRequest, status string) {
	time.Sleep(2 * time.Second)

	logOrderFault(r, http.StatusGatewayTimeout, "Order creation failed: downstream payment timeout", map[string]any{
		"kind":       "DownstreamPaymentTimeout",
		"message":    "payment-service did not respond within 2000ms",
		"root_cause": "Simulated slow payment-service; upstream deadline exceeded",
	}, map[string]any{
		"customer_email": req.CustomerEmail,
		"total_amount":   req.TotalAmount,
		"status":         status,
		"demo_fault":     fault.ModeTimeout,
	})

	writeJSON(w, http.StatusGatewayTimeout, map[string]any{
		"error": "payment service timeout",
	})
}

func (h OrdersCreateHandler) createWithLockedContention(w http.ResponseWriter, r *http.Request, req createOrderRequest, status string) {
	ready := make(chan struct{})
	release := make(chan struct{})
	go func() {
		tx, err := h.DB.Begin()
		if err != nil {
			close(ready)
			return
		}
		if _, err := tx.Exec(`INSERT INTO orders (amount, status) VALUES (999.0, 'contention-hold')`); err != nil {
			_ = tx.Rollback()
			close(ready)
			return
		}
		close(ready)
		select {
		case <-release:
		case <-time.After(2 * time.Second):
		}
		_ = tx.Rollback()
	}()

	<-ready
	time.Sleep(50 * time.Millisecond)

	_, err := h.DB.Exec(
		`INSERT INTO orders (amount, status) VALUES (?, ?)`,
		req.TotalAmount,
		status,
	)
	close(release)
	if err == nil {
		writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{
			"customer_email": req.CustomerEmail,
			"total_amount":   req.TotalAmount,
			"status":         status,
		}})
		return
	}

	logOrderFault(r, http.StatusInternalServerError, "Order creation failed: database locked", map[string]any{
		"kind":       "DatabaseLocked",
		"message":    err.Error(),
		"root_cause": "Concurrent writers contended for SQLite; another transaction held the write lock",
	}, map[string]any{
		"customer_email": req.CustomerEmail,
		"total_amount":   req.TotalAmount,
		"status":         status,
		"demo_fault":     fault.ModeLocked,
	})

	writeJSON(w, http.StatusInternalServerError, map[string]any{
		"error": "internal server error",
		"hint":  "database is locked",
	})
}

func logOrderFault(r *http.Request, status int, message string, errFields map[string]any, context map[string]any) {
	fields := map[string]any{
		"http": map[string]any{
			"status_code": status,
			"method":      r.Method,
			"url":         r.URL.Path,
		},
		"error":   errFields,
		"context": context,
	}
	logger.ErrorContext(r.Context(), message, fields)
}
