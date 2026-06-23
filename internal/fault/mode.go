package fault

import (
	"fmt"
	"net/http"
	"strings"
)

// HeaderName selects the demo fault injected on POST /api/orders.
const HeaderName = "X-Demo-Fault"

// Mode is a header-driven fault scenario for demo and RCA workflows.
type Mode string

const (
	ModeSchema     Mode = "schema"
	ModeDependency Mode = "dependency"
	ModeTimeout    Mode = "timeout"
	ModePanic      Mode = "panic"
	ModeLocked     Mode = "locked"
	ModeHealthy    Mode = "healthy"
)

// ModeFromRequest reads X-Demo-Fault. Missing or empty header defaults to healthy checkout.
func ModeFromRequest(r *http.Request) (Mode, error) {
	value := strings.TrimSpace(r.Header.Get(HeaderName))
	if value == "" {
		return ModeHealthy, nil
	}

	mode := Mode(strings.ToLower(value))
	switch mode {
	case ModeSchema, ModeDependency, ModeTimeout, ModePanic, ModeLocked, ModeHealthy:
		return mode, nil
	default:
		return "", fmt.Errorf("unknown demo fault %q", value)
	}
}
