package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/appcd-dev/order-service/internal/checkout"
	"github.com/appcd-dev/order-service/internal/clients"
	"github.com/appcd-dev/order-service/internal/fault"
	"github.com/appcd-dev/order-service/internal/logger"
)

type OrdersListHandler struct {
	DB *sql.DB
}

func (h OrdersListHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(`SELECT id, amount, status, created_at FROM orders ORDER BY id`)
	if err != nil {
		RespondInternalError(r.Context(), w, r, err, "orders_list_query")
		return
	}
	defer rows.Close()

	orders := make([]map[string]any, 0)
	for rows.Next() {
		var id int
		var amount float64
		var status, createdAt string
		if err := rows.Scan(&id, &amount, &status, &createdAt); err != nil {
			RespondInternalError(r.Context(), w, r, err, "orders_scan_row")
			return
		}
		orders = append(orders, map[string]any{
			"id":         id,
			"amount":     amount,
			"status":     status,
			"created_at": createdAt,
		})
	}

	if err := rows.Err(); err != nil {
		RespondInternalError(r.Context(), w, r, err, "orders_iteration")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": orders})
}

type OrdersCreateHandler struct {
	DB           *sql.DB
	Checkout     *checkout.Orchestrator
}

type createOrderRequest struct {
	CustomerEmail      string  `json:"customer_email"`
	TotalAmount        float64 `json:"total_amount"`
	Status             string  `json:"status"`
	ProductID          string  `json:"product_id"`
	CreditCardNumber   string  `json:"credit_card_number"`
	CreditCardCVV      int32   `json:"credit_card_cvv"`
	CreditCardExpYear  int32   `json:"credit_card_expiration_year"`
	CreditCardExpMonth int32   `json:"credit_card_expiration_month"`
}

// ServeHTTP handles POST /api/orders.
func (h OrdersCreateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mode, err := fault.ModeFromRequest(r)
	if err != nil {
		RespondError(r.Context(), w, r, http.StatusBadRequest, "InvalidRequest", "Unknown demo fault header", map[string]string{
			"reason": "invalid fault mode",
		})
		return
	}

	var req createOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(r.Context(), w, r, http.StatusBadRequest, "InvalidRequest", "Invalid request body", map[string]string{
			"reason": "malformed JSON",
		})
		return
	}

	if strings.TrimSpace(req.CustomerEmail) == "" {
		RespondError(r.Context(), w, r, http.StatusBadRequest, "ValidationError", "Missing required fields", map[string]string{
			"required_fields": "customer_email, total_amount",
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

func (h OrdersCreateHandler) checkoutRequest(req createOrderRequest, status string) checkout.Request {
	return checkout.Request{
		CustomerEmail:      req.CustomerEmail,
		TotalAmount:        req.TotalAmount,
		Status:             status,
		ProductID:          req.ProductID,
		CreditCardNumber:   req.CreditCardNumber,
		CreditCardCVV:      req.CreditCardCVV,
		CreditCardExpYear:  req.CreditCardExpYear,
		CreditCardExpMonth: req.CreditCardExpMonth,
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
		RespondInternalError(r.Context(), w, r, err, "orders_insert_schema_mismatch")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"customer_email": req.CustomerEmail,
		"total_amount":   req.TotalAmount,
		"status":         status,
	})
}

func (h OrdersCreateHandler) createHealthyOrder(w http.ResponseWriter, r *http.Request, req createOrderRequest, status string) {
	coRes, err := h.Checkout.ExecuteSync(r.Context(), h.checkoutRequest(req, status))
	if err != nil {
		RespondInternalError(r.Context(), w, r, err, "checkout_execution")
		return
	}

	_, err = h.DB.Exec(
		`INSERT INTO orders (amount, status) VALUES (?, ?)`,
		req.TotalAmount,
		status,
	)

	if err != nil {
		RespondInternalError(r.Context(), w, r, err, "orders_insert_healthy")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"customer_email": req.CustomerEmail,
		"total_amount":   req.TotalAmount,
		"status":         status,
		"correlation_id": coRes.CorrelationID,
	})
}

func (h OrdersCreateHandler) respondDependencyFailure(w http.ResponseWriter, r *http.Request, req createOrderRequest, status string) {
	logger.ErrorContext(r.Context(), "Dependency failure simulated", map[string]any{
		"http": map[string]any{
			"status_code": http.StatusServiceUnavailable,
			"method":      r.Method,
			"url":         r.URL.Path,
		},
		"error": map[string]any{
			"kind":       "DependencyFailure",
			"message":    "Payment processor unavailable",
			"root_cause": "Simulated payment service failure",
		},
		"customer_email": req.CustomerEmail,
		"total_amount":   req.TotalAmount,
		"status":         status,
		"demo_fault":     fault.ModeDependency,
	})

	writeJSON(w, http.StatusServiceUnavailable, map[string]any{
		"error": "payment service unavailable",
	})
}

func (h OrdersCreateHandler) respondDownstreamTimeout(w http.ResponseWriter, r *http.Request, req createOrderRequest, status string) {
	logger.ErrorContext(r.Context(), "Downstream timeout simulated", map[string]any{
		"http": map[string]any{
			"status_code": http.StatusGatewayTimeout,
			"method":      r.Method,
			"url":         r.URL.Path,
		},
		"error": map[string]any{
			"kind":       "TimeoutError",
			"message":    "Payment processing timeout",
			"root_cause": "Payment charge exceeded 500ms deadline (enable PAYMENT_DEMO_FAULT=slow on payment-service for reliable timeout)",
		},
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

	defer close(release)

	if err != nil {
		RespondInternalError(r.Context(), w, r, err, "orders_insert_locked")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"customer_email": req.CustomerEmail,
		"total_amount":   req.TotalAmount,
		"status":         status,
	})
}
