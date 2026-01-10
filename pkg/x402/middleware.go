// Package x402 provides HTTP 402 Payment Required middleware for Go web applications.
// It enables sellers to require payment before granting access to protected resources.
package x402

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Config holds the configuration for the X402 seller middleware
type Config struct {
	// PaymentEndpoint is the URL where clients can make payments
	PaymentEndpoint string

	// AcceptedMethods lists the accepted authentication methods (e.g., "Bearer", "Token")
	AcceptedMethods []string

	// PricePerRequest is the price per request in the smallest currency unit
	PricePerRequest int64

	// ExemptPaths lists paths that don't require payment
	ExemptPaths []string

	// Currency is the currency code (e.g., "USD", "BTC", "ETH")
	Currency string

	// PaymentVerifier is an optional custom payment verification function
	PaymentVerifier func(token string) (bool, error)
}

// PaymentInfo contains information about required payment returned in 402 responses
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
			sendPaymentRequired(w, config)
			return
		}

		// Verify payment token
		valid, err := verifyPaymentToken(token, config)
		if err != nil || !valid {
			// Invalid or expired payment token
			sendPaymentRequired(w, config)
			return
		}

		// Payment verified, allow access
		// Add payment metadata to response headers
		w.Header().Set("X-Payment-Verified", "true")
		w.Header().Set("X-Payment-Timestamp", time.Now().Format(time.RFC3339))

		next.ServeHTTP(w, r)
	})
}

// isExemptPath checks if the requested path is exempt from payment
func isExemptPath(path string, exemptPaths []string) bool {
	for _, exemptPath := range exemptPaths {
		if strings.HasPrefix(path, exemptPath) {
			return true
		}
	}
	return false
}

// extractPaymentToken extracts the payment token from the request
func extractPaymentToken(r *http.Request, acceptedMethods []string) string {
	// Check Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		for _, method := range acceptedMethods {
			prefix := method + " "
			if strings.HasPrefix(authHeader, prefix) {
				return strings.TrimPrefix(authHeader, prefix)
			}
		}
	}

	// Check X-Payment-Token header
	paymentToken := r.Header.Get("X-Payment-Token")
	if paymentToken != "" {
		return paymentToken
	}

	// Check query parameter
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

// sendPaymentRequired sends a 402 Payment Required response
func sendPaymentRequired(w http.ResponseWriter, config Config) {
	paymentInfo := PaymentInfo{
		Amount:      config.PricePerRequest,
		Currency:    config.Currency,
		PaymentURL:  config.PaymentEndpoint,
		Description: fmt.Sprintf("Payment of %d %s required to access this resource", config.PricePerRequest, config.Currency),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", fmt.Sprintf("%s realm=\"Payment Required\"", strings.Join(config.AcceptedMethods, ", ")))
	w.Header().Set("X-Payment-Required", "true")
	w.Header().Set("X-Payment-Amount", fmt.Sprintf("%d", paymentInfo.Amount))
	w.Header().Set("X-Payment-Currency", paymentInfo.Currency)

	w.WriteHeader(http.StatusPaymentRequired) // 402

	json.NewEncoder(w).Encode(paymentInfo)
}
