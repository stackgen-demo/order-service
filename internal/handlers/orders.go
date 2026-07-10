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
		logOrderFault(r, http.StatusInternalServerError, "Order creation failed", h.databaseErrorFields(err), map[string]any{
			"customer_email": req.CustomerEmail,
			"total_amount":   req.TotalAmount,
			"status":         status,
		})

		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "internal server error",
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
	if h.Checkout == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "checkout not configured"})
		return
	}

	checkoutResult, err := h.Checkout.Run(r.Context(), h.checkoutRequest(req, status))
	if err != nil {
		logOrderFault(r, http.StatusBadGateway, "Order creation failed: checkout saga error", map[string]any{
			"kind":       "CheckoutSagaFailure",
			"message":    err.Error(),
			"root_cause": "Downstream catalog, ad, or payment step failed during healthy checkout",
		}, map[string]any{
			"customer_email": req.CustomerEmail,
			"total_amount":   req.TotalAmount,
			"product_id":     req.ProductID,
			"demo_fault":     fault.ModeHealthy,
		})
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "checkout failed"})
		return
	}

	result, err := h.DB.Exec(
		`INSERT INTO orders (customer_email, total_amount, status) VALUES (?, ?, ?)`,
		req.CustomerEmail,
		req.TotalAmount,
		status,
	)
	if err != nil {
		logOrderFault(r, http.StatusInternalServerError, "Order creation failed", h.databaseErrorFields(err), map[string]any{
			"customer_email": req.CustomerEmail,
			"total_amount":   req.TotalAmount,
			"status":         status,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	id, _ := result.LastInsertId()
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{
		"id":             id,
		"customer_email": req.CustomerEmail,
		"total_amount":   req.TotalAmount,
		"status":         status,
		"product_id":     checkoutResult.Product.GetId(),
		"transaction_id": checkoutResult.Transaction,
	}})
}

func (h OrdersCreateHandler) respondDependencyFailure(w http.ResponseWriter, r *http.Request, req createOrderRequest, status string) {
	if h.Checkout == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "checkout not configured"})
		return
	}

	checkoutReq := h.checkoutRequest(req, status)
	checkoutReq.PaymentAddrOverride = clients.DependencyFailPaymentAddr()

	_, err := h.Checkout.Run(r.Context(), checkoutReq)
	if err == nil {
		writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"status": status}})
		return
	}

	logOrderFault(r, http.StatusBadGateway, "Order creation failed: downstream payment unavailable", map[string]any{
		"kind":       "DownstreamPaymentFailure",
		"message":    err.Error(),
		"root_cause": "Real gRPC charge to unreachable payment endpoint; order-service cannot authorize charge",
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
	if h.Checkout == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "checkout not configured"})
		return
	}

	checkoutReq := h.checkoutRequest(req, status)
	checkoutReq.PaymentTimeoutOverride = clients.PaymentTimeout()
	checkoutReq.PaymentSlowDemo = true

	_, err := h.Checkout.Run(r.Context(), checkoutReq)
	if err == nil {
		writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"status": status}})
		return
	}

	logOrderFault(r, http.StatusGatewayTimeout, "Order creation failed: downstream payment timeout", map[string]any{
		"kind":       "DownstreamPaymentTimeout",
		"message":    err.Error(),
		"root_cause": "Payment charge exceeded 500ms deadline (enable PAYMENT_DEMO_FAULT=slow on payment-service for reliable timeout)",
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

// databaseErrorFields classifies SQLite schema drift so Datadog monitors and SRE investigations
// can match DatabaseSchemaMismatch on the healthy checkout path (no X-Demo-Fault header).
func (h OrdersCreateHandler) databaseErrorFields(err error) map[string]any {
	message := err.Error()
	if strings.Contains(message, "no such column") {
		return map[string]any{
			"kind":       "DatabaseSchemaMismatch",
			"message":    message,
			"root_cause": "Application INSERT references columns missing from orders table — update cmd/initdb/main.go schema",
		}
	}

	return map[string]any{
		"kind":    "DatabaseError",
		"message": message,
	}
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
