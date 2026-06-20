package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/appcd-dev/order-service/internal/logger"
)

type UsersListHandler struct {
	DB *sql.DB
}

func (h UsersListHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(`SELECT id, name, email, created_at FROM users ORDER BY id`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	defer rows.Close()

	users := make([]map[string]any, 0)
	for rows.Next() {
		var id int
		var name, email, createdAt string
		if err := rows.Scan(&id, &name, &email, &createdAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		users = append(users, map[string]any{
			"id":         id,
			"name":       name,
			"email":      email,
			"created_at": createdAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": users})
}

type UsersCreateHandler struct {
	DB *sql.DB
}

type createUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func (h UsersCreateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Email) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and email are required"})
		return
	}

	result, err := h.DB.Exec(`INSERT INTO users (name, email) VALUES (?, ?)`, req.Name, req.Email)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "email already exists"})
			return
		}

		logHandlerError(r, http.StatusInternalServerError, "User creation failed", err, map[string]any{
			"context": map[string]string{"name": req.Name, "email": req.Email},
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	id, _ := result.LastInsertId()
	var name, email, createdAt string
	err = h.DB.QueryRow(
		`SELECT name, email, created_at FROM users WHERE id = ?`,
		id,
	).Scan(&name, &email, &createdAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"id":         id,
			"name":       name,
			"email":      email,
			"created_at": createdAt,
		},
	})
}

func logHandlerError(r *http.Request, status int, message string, err error, extra map[string]any) {
	fields := map[string]any{
		"http": map[string]any{
			"status_code": status,
			"method":      r.Method,
			"url":         r.URL.Path,
		},
		"error": map[string]any{
			"kind":    errorKind(err),
			"message": err.Error(),
		},
	}
	for key, value := range extra {
		fields[key] = value
	}
	logger.ErrorContext(r.Context(), message, fields)
}

func errorKind(err error) string {
	if errors.Is(err, sql.ErrNoRows) {
		return "SqlNoRows"
	}
	return "Error"
}
