// Package x402 provides HTTP 402 Payment Required middleware for Go web applications.
// It enables sellers to require payment before granting access to protected resources.
// Implements the x402 protocol: https://github.com/coinbase/x402
package x402

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	// X402Version is the current x402 protocol version
	X402Version = 1
)

// Config holds the configuration for the X402 seller middleware
type Config struct {
	// PayTo is the address to receive payments (required for x402 compliance)
	PayTo string

	// PaymentEndpoint is the URL where clients can make payments (facilitator URL)
	PaymentEndpoint string

	// AcceptedMethods lists the accepted authentication methods (e.g., "Bearer", "Token", "X402")
	AcceptedMethods []string

	// PricePerRequest is the price per request in the smallest currency unit (e.g., 1000 = $0.001 USDC)
	PricePerRequest int64

	// ExemptPaths lists paths that don't require payment
	ExemptPaths []string

	// Currency is the currency code (e.g., "USD", "USDC")
	Currency string

	// Network is the blockchain network (e.g., "base-sepolia", "base")
	Network string

	// Scheme is the payment scheme (e.g., "exact")
	Scheme string

	// Asset is the token contract address for payment
	Asset string

	// Description describes what the payment is for
	Description string

	// MaxTimeoutSeconds is the maximum time for payment verification
	MaxTimeoutSeconds int

	// PaymentVerifier is an optional custom payment verification function
	PaymentVerifier func(token string) (bool, error)
}

// PaymentRequirements defines the x402 payment requirements structure
type PaymentRequirements struct {
	Scheme            string                 `json:"scheme"`
	Network           string                 `json:"network"`
	MaxAmountRequired string                 `json:"maxAmountRequired"`
	Resource          string                 `json:"resource"`
	Description       string                 `json:"description"`
	MimeType          string                 `json:"mimeType,omitempty"`
	PayTo             string                 `json:"payTo"`
	MaxTimeoutSeconds int                    `json:"maxTimeoutSeconds"`
	Asset             string                 `json:"asset,omitempty"`
	OutputSchema      interface{}            `json:"outputSchema"`
	Extra             map[string]interface{} `json:"extra,omitempty"`
}

// PaymentRequiredResponse is the x402 402 response body
type PaymentRequiredResponse struct {
	X402Version int                   `json:"x402Version"`
	Accepts     []PaymentRequirements `json:"accepts"`
	Error       string                `json:"error,omitempty"`
}

// PaymentInfo contains legacy payment info (for backward compatibility)
type PaymentInfo struct {
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
	PaymentURL  string `json:"payment_url"`
	Description string `json:"description"`
}

// Middleware creates a middleware that implements HTTP 402 Payment Required
func Middleware(next http.Handler, config Config) http.Handler {
	// Set default currency if not provided
	if config.Currency == "" {
		config.Currency = "USD"
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if path is exempt from payment
		if isExemptPath(r.URL.Path, config.ExemptPaths) {
			next.ServeHTTP(w, r)
			return
		}

		// Extract payment token from request
		token := extractPaymentToken(r, config.AcceptedMethods)

		if token == "" {
			// No payment token provided, return 402
			sendPaymentRequired(w, config, r)
			return
		}

		// Verify payment token
		valid, err := verifyPaymentToken(token, config)
		if err != nil || !valid {
			// Invalid or expired payment token
			sendPaymentRequired(w, config, r)
			return
		}

		// Payment verified, allow access
		// Add payment metadata to response headers
		w.Header().Set("X-Payment-Verified", "true")
		w.Header().Set("X-Payment-Timestamp", time.Now().Format(time.RFC3339))

		next.ServeHTTP(w, r)
	})
}

// isExemptPath checks if the requested path is exempt from payment.
// It uses prefix matching, so "/api/public" will match "/api/public",
// "/api/public/foo", and "/api/publicXYZ". Use trailing slashes for
// directory-style matching: "/api/public/" only matches paths starting
// with that exact prefix.
func isExemptPath(path string, exemptPaths []string) bool {
	for _, exemptPath := range exemptPaths {
		if strings.HasPrefix(path, exemptPath) {
			return true
		}
	}
	return false
}

// extractPaymentToken extracts the payment token from the request
// Supports x402 protocol headers (X-PAYMENT, PAYMENT-SIGNATURE) and legacy methods
func extractPaymentToken(r *http.Request, acceptedMethods []string) string {
	// x402 v2: Check PAYMENT-SIGNATURE header first
	if paymentSig := r.Header.Get("PAYMENT-SIGNATURE"); paymentSig != "" {
		return paymentSig
	}

	// x402 v1: Check X-PAYMENT header (base64-encoded payment payload)
	if xPayment := r.Header.Get("X-PAYMENT"); xPayment != "" {
		return xPayment
	}

	// Legacy: Check Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		for _, method := range acceptedMethods {
			prefix := method + " "
			if strings.HasPrefix(authHeader, prefix) {
				return strings.TrimPrefix(authHeader, prefix)
			}
		}
	}

	// Legacy: Check X-Payment-Token header
	paymentToken := r.Header.Get("X-Payment-Token")
	if paymentToken != "" {
		return paymentToken
	}

	// Legacy: Check query parameter
	return r.URL.Query().Get("payment_token")
}

// verifyPaymentToken verifies the payment token
func verifyPaymentToken(token string, config Config) (bool, error) {
	// Use custom verifier if provided
	if config.PaymentVerifier != nil {
		return config.PaymentVerifier(token)
	}

	// Default: accept tokens that start with "valid_" (for testing)
	if strings.HasPrefix(token, "valid_") {
		return true, nil
	}

	return false, nil
}

// sendPaymentRequired sends a 402 Payment Required response compliant with x402 protocol
func sendPaymentRequired(w http.ResponseWriter, config Config, r *http.Request) {
	// Build resource URL
	resource := r.URL.Path
	if r.URL.RawQuery != "" {
		resource += "?" + r.URL.RawQuery
	}

	// Set defaults
	scheme := config.Scheme
	if scheme == "" {
		scheme = "exact"
	}
	network := config.Network
	if network == "" {
		network = "base-sepolia"
	}
	maxTimeout := config.MaxTimeoutSeconds
	if maxTimeout == 0 {
		maxTimeout = 60
	}
	description := config.Description
	if description == "" {
		description = fmt.Sprintf("Payment of %d %s required", config.PricePerRequest, config.Currency)
	}

	// Build x402 PaymentRequirements
	requirements := PaymentRequirements{
		Scheme:            scheme,
		Network:           network,
		MaxAmountRequired: fmt.Sprintf("%d", config.PricePerRequest),
		Resource:          resource,
		Description:       description,
		PayTo:             config.PayTo,
		MaxTimeoutSeconds: maxTimeout,
		Asset:             config.Asset,
		OutputSchema:      nil,
	}

	// Build x402 response
	response := PaymentRequiredResponse{
		X402Version: X402Version,
		Accepts:     []PaymentRequirements{requirements},
		Error:       "X-PAYMENT header is required",
	}

	// Encode response as base64 for PAYMENT-REQUIRED header (v2 protocol)
	responseJSON, _ := json.Marshal(response)
	paymentRequiredHeader := base64.StdEncoding.EncodeToString(responseJSON)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("PAYMENT-REQUIRED", paymentRequiredHeader) // x402 v2 header

	w.WriteHeader(http.StatusPaymentRequired) // 402

	// Write JSON body (x402 v1 style, also useful for debugging)
	_ = json.NewEncoder(w).Encode(response)
}

// MultiSchemeMiddleware creates a middleware that accepts multiple payment schemes
// This supports both crypto (EVM, SVM) and future fiat rails (Visa, Stripe)
func MultiSchemeMiddleware(next http.Handler, config MultiSchemeConfig) http.Handler {
	// Set default currency if not provided
	if config.Currency == "" {
		config.Currency = "USD"
	}

	registry := config.SchemeRegistry
	if registry == nil {
		registry = DefaultRegistry
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if path is exempt from payment
		if isExemptPath(r.URL.Path, config.ExemptPaths) {
			next.ServeHTTP(w, r)
			return
		}

		// Extract payment token from request
		token := extractPaymentToken(r, config.AcceptedMethods)

		if token == "" {
			// No payment token provided, return 402 with multi-scheme requirements
			sendMultiSchemePaymentRequired(w, config, r)
			return
		}

		// Parse payment payload to determine scheme
		payload, err := parsePaymentPayload(token)
		if err != nil {
			// Invalid payload format
			sendMultiSchemePaymentRequired(w, config, r)
			return
		}

		// Get the appropriate scheme handler
		scheme, ok := registry.Get(payload.Scheme)
		if !ok {
			// Unsupported scheme, return 402 with supported schemes
			sendMultiSchemePaymentRequired(w, config, r)
			return
		}

		// Build requirements for verification
		resource := r.URL.Path
		if r.URL.RawQuery != "" {
			resource += "?" + r.URL.RawQuery
		}

		requirements := &PaymentRequirements{
			Scheme:            string(payload.Scheme),
			Network:           string(payload.Network),
			MaxAmountRequired: fmt.Sprintf("%d", config.PricePerRequest),
			Resource:          resource,
			PayTo:             config.PayTo,
			MaxTimeoutSeconds: config.MaxTimeoutSeconds,
			Asset:             config.Asset,
		}

		// Verify payment using the scheme handler
		result, err := scheme.Verify(r.Context(), payload, requirements)
		if err != nil || !result.Valid {
			sendMultiSchemePaymentRequired(w, config, r)
			return
		}

		// Payment verified, allow access
		w.Header().Set("X-Payment-Verified", "true")
		w.Header().Set("X-Payment-Scheme", string(payload.Scheme))
		w.Header().Set("X-Payment-Network", string(payload.Network))
		w.Header().Set("X-Payment-Timestamp", fmt.Sprintf("%d", payload.Timestamp))

		next.ServeHTTP(w, r)
	})
}

// sendMultiSchemePaymentRequired sends a 402 response with all accepted schemes
func sendMultiSchemePaymentRequired(w http.ResponseWriter, config MultiSchemeConfig, r *http.Request) {
	// Build resource URL
	resource := r.URL.Path
	if r.URL.RawQuery != "" {
		resource += "?" + r.URL.RawQuery
	}

	// Generate requirements for all accepted schemes/networks
	requirements := config.BuildMultiSchemeRequirements(resource)

	// If no multi-scheme config, fall back to single scheme
	if len(requirements) == 0 {
		requirements = []PaymentRequirements{{
			Scheme:            "exact",
			Network:           "base-sepolia",
			MaxAmountRequired: fmt.Sprintf("%d", config.PricePerRequest),
			Resource:          resource,
			Description:       config.Description,
			PayTo:             config.PayTo,
			MaxTimeoutSeconds: 60,
			Asset:             config.Asset,
		}}
	}

	// Build x402 response
	response := PaymentRequiredResponse{
		X402Version: X402Version,
		Accepts:     requirements,
		Error:       "Payment required - select a supported scheme and network",
	}

	// Encode response as base64 for PAYMENT-REQUIRED header (v2 protocol)
	responseJSON, _ := json.Marshal(response)
	paymentRequiredHeader := base64.StdEncoding.EncodeToString(responseJSON)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("PAYMENT-REQUIRED", paymentRequiredHeader)

	w.WriteHeader(http.StatusPaymentRequired) // 402

	_ = json.NewEncoder(w).Encode(response)
}

// parsePaymentPayload parses a base64-encoded payment payload
func parsePaymentPayload(token string) (*PaymentPayload, error) {
	// Try base64 decode first
	decoded, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		// Try URL-safe base64
		decoded, err = base64.URLEncoding.DecodeString(token)
		if err != nil {
			// Assume it's raw JSON
			decoded = []byte(token)
		}
	}

	var payload PaymentPayload
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return nil, fmt.Errorf("invalid payment payload: %w", err)
	}

	return &payload, nil
}
