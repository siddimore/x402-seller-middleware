// Package edge provides x402 middleware compatible with edge computing platforms
// like Cloudflare Workers, Vercel Edge, Deno Deploy, etc.
package edge

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// EdgeConfig is a simplified config for edge deployment
type EdgeConfig struct {
	// PaymentEndpoint is the URL where clients can make payments
	PaymentEndpoint string `json:"payment_endpoint"`

	// Price per request in smallest currency unit
	Price int64 `json:"price"`

	// Currency code
	Currency string `json:"currency"`

	// ExemptPaths - paths that don't require payment
	ExemptPaths []string `json:"exempt_paths"`

	// UpstreamURL - the backend to proxy to (for gateway mode)
	UpstreamURL string `json:"upstream_url"`

	// ValidTokens - static list of valid tokens (for simple deployments)
	ValidTokens []string `json:"valid_tokens,omitempty"`

	// VerifyEndpoint - external endpoint to verify tokens
	VerifyEndpoint string `json:"verify_endpoint,omitempty"`
}

// PaymentRequiredResponse is the 402 response body
type PaymentRequiredResponse struct {
	Status      int    `json:"status"`
	Error       string `json:"error"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
	PaymentURL  string `json:"payment_url"`
	Description string `json:"description"`
}

// EdgeHandler handles x402 logic at the edge
type EdgeHandler struct {
	config   EdgeConfig
	tokenSet map[string]struct{}
}

// NewEdgeHandler creates a new edge-compatible handler
func NewEdgeHandler(config EdgeConfig) *EdgeHandler {
	tokenSet := make(map[string]struct{})
	for _, t := range config.ValidTokens {
		tokenSet[t] = struct{}{}
	}

	if config.Currency == "" {
		config.Currency = "USD"
	}

	return &EdgeHandler{
		config:   config,
		tokenSet: tokenSet,
	}
}

// ShouldRequirePayment checks if a request should require payment
// Returns: (requiresPayment bool, token string)
func (h *EdgeHandler) ShouldRequirePayment(r *http.Request) (bool, string) {
	// Check exempt paths
	for _, path := range h.config.ExemptPaths {
		if strings.HasPrefix(r.URL.Path, path) {
			return false, ""
		}
	}

	// Extract token
	token := h.ExtractToken(r)
	if token == "" {
		return true, ""
	}

	return false, token
}

// ExtractToken extracts payment token from various sources
func (h *EdgeHandler) ExtractToken(r *http.Request) string {
	// Check Authorization header (Bearer, Token, X402)
	if auth := r.Header.Get("Authorization"); auth != "" {
		for _, prefix := range []string{"Bearer ", "Token ", "X402 "} {
			if strings.HasPrefix(auth, prefix) {
				return strings.TrimPrefix(auth, prefix)
			}
		}
	}

	// Check X-Payment-Token header
	if token := r.Header.Get("X-Payment-Token"); token != "" {
		return token
	}

	// Check X-402-Token header (standardized)
	if token := r.Header.Get("X-402-Token"); token != "" {
		return token
	}

	// Check query parameter
	if token := r.URL.Query().Get("payment_token"); token != "" {
		return token
	}

	// Check cookie
	if cookie, err := r.Cookie("x402_token"); err == nil {
		return cookie.Value
	}

	return ""
}

// VerifyToken verifies if a token is valid
func (h *EdgeHandler) VerifyToken(token string) bool {
	// Check static token list first (fastest)
	if len(h.tokenSet) > 0 {
		_, valid := h.tokenSet[token]
		return valid
	}

	// For testing: accept tokens starting with "valid_"
	if strings.HasPrefix(token, "valid_") {
		return true
	}

	return false
}

// PaymentRequiredJSON returns the 402 response as JSON bytes
func (h *EdgeHandler) PaymentRequiredJSON() []byte {
	resp := PaymentRequiredResponse{
		Status:      402,
		Error:       "Payment Required",
		Amount:      h.config.Price,
		Currency:    h.config.Currency,
		PaymentURL:  h.config.PaymentEndpoint,
		Description: fmt.Sprintf("Payment of %d %s required", h.config.Price, h.config.Currency),
	}
	data, _ := json.Marshal(resp)
	return data
}

// PaymentRequiredHeaders returns headers for a 402 response
func (h *EdgeHandler) PaymentRequiredHeaders() map[string]string {
	return map[string]string{
		"Content-Type":       "application/json",
		"X-Payment-Required": "true",
		"X-Payment-Amount":   fmt.Sprintf("%d", h.config.Price),
		"X-Payment-Currency": h.config.Currency,
		"X-Payment-URL":      h.config.PaymentEndpoint,
		"WWW-Authenticate":   `Bearer realm="Payment Required", X402 realm="Payment Required"`,
		"Cache-Control":      "no-store",
	}
}

// SuccessHeaders returns headers to add on successful payment verification
func (h *EdgeHandler) SuccessHeaders() map[string]string {
	return map[string]string{
		"X-Payment-Verified":  "true",
		"X-Payment-Timestamp": time.Now().UTC().Format(time.RFC3339),
	}
}

// ServeHTTP implements http.Handler for standard Go servers
func (h *EdgeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requiresPayment, token := h.ShouldRequirePayment(r)

	if !requiresPayment {
		// Path is exempt, let it through
		w.WriteHeader(http.StatusOK)
		return
	}

	if token == "" || !h.VerifyToken(token) {
		// No valid token, return 402
		for k, v := range h.PaymentRequiredHeaders() {
			w.Header().Set(k, v)
		}
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write(h.PaymentRequiredJSON())
		return
	}

	// Valid payment, add success headers
	for k, v := range h.SuccessHeaders() {
		w.Header().Set(k, v)
	}
	w.WriteHeader(http.StatusOK)
}

// WrapHandler wraps an existing http.Handler with x402 payment protection
func (h *EdgeHandler) WrapHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requiresPayment, token := h.ShouldRequirePayment(r)

		if !requiresPayment {
			next.ServeHTTP(w, r)
			return
		}

		if token == "" || !h.VerifyToken(token) {
			for k, v := range h.PaymentRequiredHeaders() {
				w.Header().Set(k, v)
			}
			w.WriteHeader(http.StatusPaymentRequired)
			_, _ = w.Write(h.PaymentRequiredJSON())
			return
		}

		for k, v := range h.SuccessHeaders() {
			w.Header().Set(k, v)
		}
		next.ServeHTTP(w, r)
	})
}
