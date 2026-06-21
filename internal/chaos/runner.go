package chaos

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/appcd-dev/order-service/internal/clients"
	"github.com/appcd-dev/order-service/internal/logger"
	pb "github.com/appcd-dev/order-service/genproto/oteldemo"
)

// Runner executes random checkout sagas and isolated leaf faults.
type Runner struct {
	HTTPClient *http.Client
	Downstream *clients.Downstream
	Config     Config
	Rand       *rand.Rand
}

// Config controls chaos monkey timing and targets.
type Config struct {
	Enabled       bool
	MinInterval   time.Duration
	MaxInterval   time.Duration
	BurstSize     int
	OrderURL      string
	PaymentAddr   string
	CatalogAddr   string
	AdAddr        string
}

// LoadConfigFromEnv reads chaos monkey settings from the environment.
func LoadConfigFromEnv() Config {
	minSec := envInt("CHAOS_MIN_INTERVAL_SEC", 45)
	maxSec := envInt("CHAOS_MAX_INTERVAL_SEC", 480)
	return Config{
		Enabled:     envBool("CHAOS_ENABLED", true),
		MinInterval: time.Duration(minSec) * time.Second,
		MaxInterval: time.Duration(maxSec) * time.Second,
		BurstSize:   envInt("CHAOS_BURST_SIZE", 1),
		OrderURL:    envOrDefault("ORDER_SERVICE_URL", "http://aiden-demo"),
		PaymentAddr: envOrDefault("PAYMENT_GRPC_ADDR", "payment-service:50051"),
		CatalogAddr: envOrDefault("CATALOG_GRPC_ADDR", "product-catalog-service:3550"),
		AdAddr:      envOrDefault("AD_GRPC_ADDR", "ad-service:8080"),
	}
}

// RunLoop sleeps randomly and fires weighted faults until ctx is cancelled.
func (r *Runner) RunLoop(ctx context.Context) {
	if r == nil || !r.Config.Enabled {
		logger.Info("chaos monkey disabled", nil)
		return
	}

	logger.Info("chaos monkey started", map[string]any{
		"min_interval_sec": int(r.Config.MinInterval.Seconds()),
		"max_interval_sec": int(r.Config.MaxInterval.Seconds()),
	})

	for {
		sleep := r.randomInterval()
		select {
		case <-ctx.Done():
			logger.Info("chaos monkey stopped", nil)
			return
		case <-time.After(sleep):
		}

		burst := r.Config.BurstSize
		if r.Rand.Intn(5) == 0 {
			burst = 3 + r.Rand.Intn(3)
		}

		for i := 0; i < burst; i++ {
			r.executeOnce(ctx)
		}
	}
}

func (r *Runner) executeOnce(ctx context.Context) {
	mode := "checkout_saga"
	if r.Rand.Intn(100) >= 60 {
		mode = "isolated"
	}

	var target, fault, result string
	var err error

	switch mode {
	case "checkout_saga":
		target = "order-service"
		fault = r.pick([]string{"healthy", "dependency", "timeout", "schema"})
		result, err = r.postOrder(ctx, fault)
	default:
		target = r.pick([]string{"payment-service", "product-catalog-service", "ad-service"})
		fault, result, err = r.isolatedFault(ctx, target)
	}

	fields := map[string]any{
		"chaos_mode":   mode,
		"chaos_target": target,
		"chaos_fault":  fault,
		"chaos_result": result,
	}
	if err != nil {
		fields["error"] = map[string]any{"kind": "ChaosActionError", "message": err.Error()}
		logger.Error("chaos action completed with error", fields)
		return
	}
	logger.Info("chaos action completed", fields)
}

func (r *Runner) postOrder(ctx context.Context, faultMode string) (string, error) {
	body := map[string]any{
		"customer_email": fmt.Sprintf("chaos-%d@example.com", r.Rand.Intn(100000)),
		"total_amount":   42.5 + float64(r.Rand.Intn(100)),
		"product_id":     r.pick([]string{"66VCHSJNUP", "1YMWWN1N4O", "OLJCESPC7Z"}),
	}
	payload, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.Config.OrderURL+"/api/orders", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Demo-Fault", faultMode)

	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return fmt.Sprintf("http_%d", resp.StatusCode), nil
}

func (r *Runner) isolatedFault(ctx context.Context, target string) (string, string, error) {
	switch target {
	case "payment-service":
		fault := r.pick([]string{"invalid_card", "expired_card", "random"})
		result, err := r.chargePayment(ctx, fault)
		return fault, result, err
	case "product-catalog-service":
		fault := r.pick([]string{"not_found", "feature_product", "rank_panic"})
		result, err := r.getCatalogProduct(ctx, fault)
		return fault, result, err
	default:
		fault := r.pick([]string{"happy", "empty_context"})
		result, err := r.getAds(ctx, fault)
		return fault, result, err
	}
}

func (r *Runner) chargePayment(ctx context.Context, fault string) (string, error) {
	if r.Downstream == nil {
		return "", fmt.Errorf("downstream not configured")
	}

	card := "4111111111111111"
	year := int32(time.Now().Year() + 1)
	month := int32(12)
	switch fault {
	case "invalid_card":
		card = "0000000000000001"
	case "expired_card":
		year = int32(time.Now().Year() - 1)
	}

	_, err := r.Downstream.Payment.Charge(ctx, &pb.ChargeRequest{
		Amount: &pb.Money{CurrencyCode: "USD", Units: 10},
		CreditCard: &pb.CreditCardInfo{
			CreditCardNumber:          card,
			CreditCardCvv:             123,
			CreditCardExpirationYear:  year,
			CreditCardExpirationMonth: month,
		},
	})
	if err != nil {
		return "grpc_error", nil
	}
	return "grpc_ok", nil
}

func (r *Runner) getCatalogProduct(ctx context.Context, fault string) (string, error) {
	if r.Downstream == nil {
		return "", fmt.Errorf("downstream not configured")
	}

	productID := "66VCHSJNUP"
	switch fault {
	case "not_found":
		productID = "DOES-NOT-EXIST"
	case "feature_product", "rank_panic":
		productID = "OLJCESPC7Z"
	}

	_, err := r.Downstream.Catalog.GetProduct(ctx, &pb.GetProductRequest{Id: productID})
	if err != nil {
		return "grpc_error", nil
	}
	return "grpc_ok", nil
}

func (r *Runner) getAds(ctx context.Context, fault string) (string, error) {
	if r.Downstream == nil {
		return "", fmt.Errorf("downstream not configured")
	}

	req := &pb.AdRequest{ContextKeys: []string{"telescopes"}}
	if fault == "empty_context" {
		req = &pb.AdRequest{}
	}

	_, err := r.Downstream.Ad.GetAds(ctx, req)
	if err != nil {
		return "grpc_error", nil
	}
	return "grpc_ok", nil
}

func (r *Runner) randomInterval() time.Duration {
	min := int(r.Config.MinInterval.Seconds())
	max := int(r.Config.MaxInterval.Seconds())
	if max <= min {
		return r.Config.MinInterval
	}
	return time.Duration(min+r.Rand.Intn(max-min+1)) * time.Second
}

func (r *Runner) pick(values []string) string {
	return values[r.Rand.Intn(len(values))]
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return parsed
}
