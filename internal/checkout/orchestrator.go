package checkout

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/appcd-dev/order-service/internal/clients"
	pb "github.com/appcd-dev/order-service/genproto/oteldemo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Request is the checkout orchestration input from POST /api/orders.
type Request struct {
	CustomerEmail          string
	TotalAmount            float64
	Status                 string
	ProductID              string
	CreditCardNumber       string
	CreditCardCVV          int32
	CreditCardExpYear      int32
	CreditCardExpMonth     int32
	PaymentAddrOverride    string
	PaymentTimeoutOverride time.Duration
	PaymentSlowDemo       bool
}

// Result captures downstream outcomes for logging and response shaping.
type Result struct {
	Product     *pb.Product
	Transaction string
}

// Orchestrator coordinates catalog → ad → payment for distributed trace demos.
type Orchestrator struct {
	Downstream *clients.Downstream
}

// Run executes the full checkout chain. Returns gRPC/HTTP-friendly errors with root causes.
func (o *Orchestrator) Run(ctx context.Context, req Request) (Result, error) {
	if o == nil || o.Downstream == nil {
		return Result{}, fmt.Errorf("checkout orchestrator not configured")
	}

	productID := req.ProductID
	if productID == "" {
		productID = envOrDefault("DEFAULT_PRODUCT_ID", "66VCHSJNUP")
	}

	product, err := o.Downstream.Catalog.GetProduct(ctx, &pb.GetProductRequest{Id: productID})
	if err != nil {
		return Result{}, fmt.Errorf("catalog get product: %w", mapGRPCError(err))
	}

	adCtx := ctx
	if len(product.Categories) > 0 {
		_, err = o.Downstream.Ad.GetAds(adCtx, &pb.AdRequest{ContextKeys: product.Categories})
		if err != nil {
			return Result{}, fmt.Errorf("ad service: %w", mapGRPCError(err))
		}
	}

	paymentClient := o.Downstream.Payment
	if req.PaymentAddrOverride != "" {
		overrideClient, conn, dialErr := clients.DialPaymentOnly(ctx, req.PaymentAddrOverride)
		if dialErr != nil {
			return Result{}, fmt.Errorf("payment dial: %w", dialErr)
		}
		defer conn.Close()
		paymentClient = overrideClient
	}

	chargeCtx := ctx
	if req.PaymentTimeoutOverride > 0 {
		var cancel context.CancelFunc
		chargeCtx, cancel = context.WithTimeout(ctx, req.PaymentTimeoutOverride)
		defer cancel()
	}

	amount := moneyFromFloat(req.TotalAmount)
	if req.PaymentSlowDemo {
		amount = &pb.Money{CurrencyCode: "USD", Units: 9999}
	}
	if req.TotalAmount == 0 && product.PriceUsd != nil && !req.PaymentSlowDemo {
		amount = product.PriceUsd
	}

	cardNumber := req.CreditCardNumber
	if cardNumber == "" {
		cardNumber = "4111111111111111"
	}

	cvv := req.CreditCardCVV
	if cvv == 0 {
		cvv = 123
	}

	expYear := req.CreditCardExpYear
	if expYear == 0 {
		expYear = int32(time.Now().Year() + 1)
	}

	expMonth := req.CreditCardExpMonth
	if expMonth == 0 {
		expMonth = 12
	}

	chargeResp, err := paymentClient.Charge(chargeCtx, &pb.ChargeRequest{
		Amount: amount,
		CreditCard: &pb.CreditCardInfo{
			CreditCardNumber:          cardNumber,
			CreditCardCvv:             cvv,
			CreditCardExpirationYear:  expYear,
			CreditCardExpirationMonth: expMonth,
		},
	})
	if err != nil {
		return Result{}, fmt.Errorf("payment charge: %w", mapGRPCError(err))
	}

	return Result{
		Product:     product,
		Transaction: chargeResp.GetTransactionId(),
	}, nil
}

func moneyFromFloat(value float64) *pb.Money {
	units := int64(value)
	nanos := int32(math.Round((value - float64(units)) * 1e9))
	return &pb.Money{
		CurrencyCode: "USD",
		Units:        units,
		Nanos:        nanos,
	}
}

func mapGRPCError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("deadline exceeded: %w", err)
	}
	st, ok := status.FromError(err)
	if !ok {
		return err
	}
	switch st.Code() {
	case codes.Unavailable:
		return fmt.Errorf("service unavailable: %s", st.Message())
	case codes.DeadlineExceeded:
		return fmt.Errorf("deadline exceeded: %s", st.Message())
	default:
		return fmt.Errorf("%s: %s", st.Code(), st.Message())
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// ParseCardFields extracts optional card fields from JSON strings for chaos traffic.
func ParseCardFields(number string, cvvRaw string) (int32, error) {
	if strings.TrimSpace(cvvRaw) == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(cvvRaw, 10, 32)
	if err != nil {
		return 0, err
	}
	return int32(parsed), nil
}
