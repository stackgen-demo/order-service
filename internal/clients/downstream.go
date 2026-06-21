package clients

import (
	"context"
	"fmt"
	"os"
	"time"

	grpctrace "github.com/DataDog/dd-trace-go/contrib/google.golang.org/grpc/v2"
	pb "github.com/appcd-dev/order-service/genproto/oteldemo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Downstream holds gRPC clients for the checkout orchestration path.
type Downstream struct {
	Catalog pb.ProductCatalogServiceClient
	Ad      pb.AdServiceClient
	Payment pb.PaymentServiceClient
	conns   []*grpc.ClientConn
}

// Config addresses for downstream microservices.
type Config struct {
	CatalogAddr string
	AdAddr      string
	PaymentAddr string
}

// Dial opens traced gRPC connections to catalog, ad, and payment services.
func Dial(ctx context.Context, cfg Config) (*Downstream, error) {
	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(grpctrace.UnaryClientInterceptor()),
	}

	catalogConn, err := grpc.DialContext(ctx, cfg.CatalogAddr, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("dial catalog: %w", err)
	}

	adConn, err := grpc.DialContext(ctx, cfg.AdAddr, dialOpts...)
	if err != nil {
		_ = catalogConn.Close()
		return nil, fmt.Errorf("dial ad: %w", err)
	}

	paymentConn, err := grpc.DialContext(ctx, cfg.PaymentAddr, dialOpts...)
	if err != nil {
		_ = catalogConn.Close()
		_ = adConn.Close()
		return nil, fmt.Errorf("dial payment: %w", err)
	}

	return &Downstream{
		Catalog: pb.NewProductCatalogServiceClient(catalogConn),
		Ad:      pb.NewAdServiceClient(adConn),
		Payment: pb.NewPaymentServiceClient(paymentConn),
		conns:   []*grpc.ClientConn{catalogConn, adConn, paymentConn},
	}, nil
}

// Close releases gRPC connections.
func (d *Downstream) Close() error {
	if d == nil {
		return nil
	}
	var first error
	for _, conn := range d.conns {
		if conn == nil {
			continue
		}
		if err := conn.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// ConfigFromEnv loads downstream addresses from environment variables.
func ConfigFromEnv() Config {
	return Config{
		CatalogAddr: envOrDefault("CATALOG_GRPC_ADDR", "product-catalog-service:3550"),
		AdAddr:      envOrDefault("AD_GRPC_ADDR", "ad-service:8080"),
		PaymentAddr: envOrDefault("PAYMENT_GRPC_ADDR", "payment-service:50051"),
	}
}

// DependencyFailPaymentAddr returns a closed-port target for dependency fault demos.
func DependencyFailPaymentAddr() string {
	return envOrDefault("PAYMENT_DEPENDENCY_FAIL_ADDR", "payment-service:9")
}

// DialPaymentOnly opens a traced client to an alternate payment address (fault injection).
func DialPaymentOnly(ctx context.Context, addr string) (pb.PaymentServiceClient, *grpc.ClientConn, error) {
	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(grpctrace.UnaryClientInterceptor()),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("dial payment at %s: %w", addr, err)
	}
	return pb.NewPaymentServiceClient(conn), conn, nil
}

// PaymentTimeout returns the deadline used for timeout fault mode.
func PaymentTimeout() time.Duration {
	return 500 * time.Millisecond
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
