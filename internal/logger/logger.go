package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

func serviceName() string {
	if name := os.Getenv("DD_SERVICE"); name != "" {
		return name
	}
	return "order-service"
}

func write(level, message string, fields map[string]any) {
	entry := map[string]any{
		"level":     level,
		"message":   message,
		"service":   serviceName(),
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	}
	for key, value := range fields {
		entry[key] = value
	}

	data, _ := json.Marshal(entry)
	line := string(data)

	if level == "error" {
		fmt.Fprintln(os.Stderr, line)
		return
	}
	fmt.Println(line)
}

func Info(message string, fields map[string]any) {
	write("info", message, fields)
}

func Error(message string, fields map[string]any) {
	write("error", message, fields)
}
