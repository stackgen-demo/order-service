package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
)

func serviceName() string {
	if name := os.Getenv("DD_SERVICE"); name != "" {
		return name
	}
	return "order-service"
}

func envName() string {
	if env := os.Getenv("DD_ENV"); env != "" {
		return env
	}
	return "local"
}

func write(ctx context.Context, level, message string, fields map[string]any) {
	entry := map[string]any{
		"level":     level,
		"status":    level,
		"message":   message,
		"service":   serviceName(),
		"env":       envName(),
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	}
	if fields == nil {
		fields = map[string]any{}
	}
	for key, value := range fields {
		entry[key] = value
	}
	if ctx != nil {
		if span, ok := tracer.SpanFromContext(ctx); ok {
			traceCtx := span.Context()
			entry["dd.trace_id"] = traceCtx.TraceID()
			entry["dd.span_id"] = fmt.Sprintf("%d", traceCtx.SpanID())
		}
	}

	data, _ := json.Marshal(entry)
	fmt.Println(string(data))
}

// Info logs a structured info event to stdout for Datadog log collection.
func Info(message string, fields map[string]any) {
	InfoContext(context.Background(), message, fields)
}

// InfoContext logs a structured info event and attaches trace IDs from ctx when present.
func InfoContext(ctx context.Context, message string, fields map[string]any) {
	write(ctx, "info", message, fields)
}

// Error logs a structured error event to stdout for Datadog log collection.
func Error(message string, fields map[string]any) {
	ErrorContext(context.Background(), message, fields)
}

// ErrorContext logs a structured error event and attaches trace IDs from ctx when present.
func ErrorContext(ctx context.Context, message string, fields map[string]any) {
	write(ctx, "error", message, fields)
}
