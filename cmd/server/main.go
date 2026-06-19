package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	httptrace "github.com/DataDog/dd-trace-go/contrib/net/http/v2"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"github.com/appcd-dev/order-service/internal/db"
	"github.com/appcd-dev/order-service/internal/handlers"
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
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	handler := httptrace.WrapHandler(handlers.NewRouter(database), service, "")

	port := envOrDefault("PORT", "3000")
	addr := fmt.Sprintf(":%s", port)

	log.Printf("%s listening on http://localhost:%s", service, port)
	log.Printf("POST /api/orders is configured to fail with HTTP 500 (schema mismatch)")

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
