package main

import (
	"context"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/appcd-dev/order-service/internal/chaos"
	"github.com/appcd-dev/order-service/internal/clients"
	"github.com/appcd-dev/order-service/internal/logger"
)

func main() {
	cfg := chaos.LoadConfigFromEnv()

	downstream, err := clients.Dial(context.Background(), clients.ConfigFromEnv())
	if err != nil {
		logger.Error("failed to dial downstream services", map[string]any{"error": err.Error()})
		os.Exit(1)
	}
	defer downstream.Close()

	runner := &chaos.Runner{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Downstream: downstream,
		Config:     cfg,
		Rand:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runner.RunLoop(ctx)
}
