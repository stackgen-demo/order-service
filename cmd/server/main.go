package main

import (
	"fmt"
	"net/http"
	"os"

	httptrace "github.com/DataDog/dd-trace-go/contrib/net/http/v2"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"github.com/appcd-dev/order-service/internal/db"
	"github.com/appcd-dev/order-service/internal/handlers"
	"github.com/appcd-dev/order-service/internal/logger"
)

func main() {
	service := envOrDefault("DD_SERVICE", "order-service")
	tracer.Start(
		tracer.WithService(service),
		tracer.WithEnv(envOrDefault("DD_ENV", "local")),
		tracer.WithServiceVersion(envOrDefault("DD_VERSION", "1.0.0")),
	)
	defer tracer.Stop()

	database, err := db.Open()
	if err != nil {
		logger.Error("failed to open database", map[string]any{"error": err.Error()})
		os.Exit(1)
	}
	defer database.Close()

	handler := httptrace.WrapHandler(handlers.NewRouter(database), service, "")

	port := envOrDefault("PORT", "3000")
	addr := fmt.Sprintf(":%s", port)

	logger.Info(service+" listening", map[string]any{
		"port": port,
		"addr": addr,
	})
	logger.Info("POST /api/orders default fault mode is schema mismatch (HTTP 500)", nil)

	if err := http.ListenAndServe(addr, handler); err != nil {
		logger.Error("server failed", map[string]any{"error": err.Error()})
		os.Exit(1)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
