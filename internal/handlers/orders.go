package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

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
// INTENTIONAL BUG: this handler inserts into customer_email and total_amount,
// but scripts/init-db creates the orders table with only amount and status.
// An agent fix should align cmd/initdb/main.go with this INSERT, or update
// this handler to match the existing schema.
func (h OrdersCreateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	_, err := h.DB.Exec(
		`INSERT INTO orders (customer_email, total_amount, status) VALUES (?, ?, ?)`,
		req.CustomerEmail,
		req.TotalAmount,
		status,
	)
	if err != nil {
		logger.Error("Order creation failed: database schema mismatch", map[string]any{
			"http": map[string]any{
				"status_code": http.StatusInternalServerError,
				"method":      r.Method,
				"url":         r.URL.Path,
			},
			"error": map[string]any{
				"kind":    "DatabaseSchemaMismatch",
				"message": err.Error(),
				"root_cause": "Application expects orders.customer_email and orders.total_amount " +
					"but DB schema only has amount and status",
			},
			"context": map[string]any{
				"customer_email": req.CustomerEmail,
				"total_amount":   req.TotalAmount,
				"status":         status,
			},
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
