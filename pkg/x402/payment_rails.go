// Package x402 provides multi-payment rail support for HTTP 402 Payment Required.
// This file implements the payment rail abstraction layer that supports:
// - Crypto rails: EVM (Base, Ethereum), SVM (Solana) via x402 native protocol
// - Fiat rails: Stripe, Visa Direct, ACH, etc. wrapped in x402 protocol
//
// The key insight is that x402 is PROTOCOL-AGNOSTIC - it defines HOW to negotiate
// payments over HTTP, not WHICH payment network to use. This means:
// - Stripe can be a "scheme" just like "exact" (EIP-3009) is for crypto
// - AI agents can pay via crypto OR fiat, depending on their configuration
// - Servers can accept multiple payment rails simultaneously
package x402

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ===============================================
// PAYMENT RAILS - The core abstraction
// ===============================================

// PaymentRail represents a payment processing backend
type PaymentRail interface {
	// ID returns the unique identifier for this rail
	ID() string

	// DisplayName returns a human-friendly name
	DisplayName() string

	// Type returns crypto, fiat, or hybrid
	Type() RailType

	// SupportedCurrencies returns currencies this rail accepts
	SupportedCurrencies() []string

	// CreatePaymentIntent initiates a payment (for fiat rails)
	CreatePaymentIntent(ctx context.Context, req *PaymentIntentRequest) (*PaymentIntent, error)

	// VerifyPayment verifies a payment is valid/complete
	VerifyPayment(ctx context.Context, req *VerifyPaymentRequest) (*PaymentVerification, error)

	// CapturePayment captures/settles a verified payment
	CapturePayment(ctx context.Context, req *CapturePaymentRequest) (*PaymentCapture, error)

	// RefundPayment refunds a captured payment (optional)
	RefundPayment(ctx context.Context, req *RefundPaymentRequest) (*PaymentRefund, error)

	// WebhookHandler returns an HTTP handler for payment webhooks
	WebhookHandler() http.Handler
}

// RailType categorizes payment rails
type RailType string

const (
	RailTypeCrypto RailType = "crypto" // On-chain payments (EVM, SVM)
	RailTypeFiat   RailType = "fiat"   // Traditional payment rails (Stripe, Visa)
	RailTypeHybrid RailType = "hybrid" // Supports both (e.g., Coinbase Commerce)
)

// ===============================================
// PAYMENT INTENTS - For fiat-style payments
// ===============================================

// PaymentIntentRequest is a request to create a payment intent
type PaymentIntentRequest struct {
	// Amount in smallest currency unit (cents for USD, wei for ETH)
	Amount int64 `json:"amount"`

	// Currency code (USD, EUR, USDC, ETH, etc.)
	Currency string `json:"currency"`

	// Resource being paid for
	Resource string `json:"resource"`

	// Description of the payment
	Description string `json:"description"`

	// Metadata for tracking
	Metadata map[string]string `json:"metadata,omitempty"`

	// Customer info (optional)
	CustomerID    string `json:"customerId,omitempty"`
	CustomerEmail string `json:"customerEmail,omitempty"`

	// For recurring/subscription payments
	SetupFutureUsage string `json:"setupFutureUsage,omitempty"` // "on_session", "off_session"

	// Idempotency key
	IdempotencyKey string `json:"idempotencyKey,omitempty"`
}

// PaymentIntent represents a payment intent (Stripe-style)
type PaymentIntent struct {
	// Unique identifier
	ID string `json:"id"`

	// Rail that created this intent
	Rail string `json:"rail"`

	// Amount and currency
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`

	// Status: requires_payment_method, requires_confirmation, requires_action, processing, succeeded, canceled
	Status string `json:"status"`

	// Client secret for frontend confirmation
	ClientSecret string `json:"clientSecret,omitempty"`

	// For 3D Secure or additional authentication
	NextAction *PaymentNextAction `json:"nextAction,omitempty"`

	// Timestamps
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`

	// Metadata
	Metadata map[string]string `json:"metadata,omitempty"`
}

// PaymentNextAction describes what the client needs to do next
type PaymentNextAction struct {
	Type        string `json:"type"` // "redirect_to_url", "use_stripe_sdk", "display_qr_code"
	RedirectURL string `json:"redirectUrl,omitempty"`
	QRCode      string `json:"qrCode,omitempty"`
}

// ===============================================
// VERIFICATION & CAPTURE
// ===============================================

// VerifyPaymentRequest is a request to verify a payment
type VerifyPaymentRequest struct {
	// For crypto: the payment signature/payload
	PaymentPayload string `json:"paymentPayload,omitempty"`

	// For fiat: payment intent ID or token
	PaymentIntentID string `json:"paymentIntentId,omitempty"`
	PaymentToken    string `json:"paymentToken,omitempty"`

	// Expected payment details
	ExpectedAmount   int64  `json:"expectedAmount"`
	ExpectedCurrency string `json:"expectedCurrency"`
	ExpectedPayTo    string `json:"expectedPayTo"`

	// Resource being accessed
	Resource string `json:"resource"`
}

// PaymentVerification is the result of payment verification
type PaymentVerification struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message,omitempty"`

	// Payment details
	PaymentID string `json:"paymentId"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	Payer     string `json:"payer,omitempty"` // Address or customer ID

	// For capture
	RequiresCapture bool `json:"requiresCapture"`

	// Timestamps
	VerifiedAt time.Time `json:"verifiedAt"`
}

// CapturePaymentRequest is a request to capture/settle a payment
type CapturePaymentRequest struct {
	PaymentID string `json:"paymentId"`
	Amount    int64  `json:"amount,omitempty"` // For partial capture

	// For crypto: additional settlement data
	SettlementData map[string]interface{} `json:"settlementData,omitempty"`
}

// PaymentCapture is the result of payment capture
type PaymentCapture struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`

	// Transaction reference
	TransactionID  string `json:"transactionId"`
	TransactionURL string `json:"transactionUrl,omitempty"`

	// Settled amounts
	GrossAmount int64 `json:"grossAmount"`
	NetAmount   int64 `json:"netAmount"` // After fees
	FeeAmount   int64 `json:"feeAmount"`

	// Timestamps
	CapturedAt time.Time `json:"capturedAt"`
}

// RefundPaymentRequest is a request to refund a payment
type RefundPaymentRequest struct {
	PaymentID string `json:"paymentId"`
	Amount    int64  `json:"amount,omitempty"` // For partial refund
	Reason    string `json:"reason,omitempty"`
}

// PaymentRefund is the result of a refund
type PaymentRefund struct {
	Success  bool   `json:"success"`
	RefundID string `json:"refundId"`
	Amount   int64  `json:"amount"`
	Status   string `json:"status"` // pending, succeeded, failed
}

// ===============================================
// STRIPE RAIL IMPLEMENTATION
// ===============================================

// StripeRail implements PaymentRail for Stripe payments
type StripeRail struct {
	// Stripe API key
	SecretKey string

	// Webhook secret for verifying webhooks
	WebhookSecret string

	// API base URL (for testing)
	BaseURL string

	// HTTP client
	client *http.Client
}

// NewStripeRail creates a new Stripe payment rail
func NewStripeRail(secretKey, webhookSecret string) *StripeRail {
	return &StripeRail{
		SecretKey:     secretKey,
		WebhookSecret: webhookSecret,
		BaseURL:       "https://api.stripe.com/v1",
		client:        &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *StripeRail) ID() string {
	return "stripe"
}

func (s *StripeRail) DisplayName() string {
	return "Credit/Debit Card (Stripe)"
}

func (s *StripeRail) Type() RailType {
	return RailTypeFiat
}

func (s *StripeRail) SupportedCurrencies() []string {
	return []string{"USD", "EUR", "GBP", "CAD", "AUD", "JPY"} // Stripe supports 135+ currencies
}

func (s *StripeRail) CreatePaymentIntent(ctx context.Context, req *PaymentIntentRequest) (*PaymentIntent, error) {
	// Build Stripe API request
	data := fmt.Sprintf(
		"amount=%d&currency=%s&description=%s&metadata[resource]=%s",
		req.Amount,
		strings.ToLower(req.Currency),
		req.Description,
		req.Resource,
	)

	if req.CustomerID != "" {
		data += "&customer=" + req.CustomerID
	}

	if req.SetupFutureUsage != "" {
		data += "&setup_future_usage=" + req.SetupFutureUsage
	}

	// Make API request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", s.BaseURL+"/payment_intents", strings.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+s.SecretKey)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if req.IdempotencyKey != "" {
		httpReq.Header.Set("Idempotency-Key", req.IdempotencyKey)
	}

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("stripe API error: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stripe error: %s", string(body))
	}

	// Parse response
	var stripeIntent struct {
		ID           string `json:"id"`
		Amount       int64  `json:"amount"`
		Currency     string `json:"currency"`
		Status       string `json:"status"`
		ClientSecret string `json:"client_secret"`
		Created      int64  `json:"created"`
	}

	if err := json.Unmarshal(body, &stripeIntent); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &PaymentIntent{
		ID:           stripeIntent.ID,
		Rail:         s.ID(),
		Amount:       stripeIntent.Amount,
		Currency:     strings.ToUpper(stripeIntent.Currency),
		Status:       stripeIntent.Status,
		ClientSecret: stripeIntent.ClientSecret,
		CreatedAt:    time.Unix(stripeIntent.Created, 0),
		Metadata:     req.Metadata,
	}, nil
}

func (s *StripeRail) VerifyPayment(ctx context.Context, req *VerifyPaymentRequest) (*PaymentVerification, error) {
	// Retrieve payment intent from Stripe
	httpReq, err := http.NewRequestWithContext(ctx, "GET", s.BaseURL+"/payment_intents/"+req.PaymentIntentID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+s.SecretKey)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("stripe API error: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stripe error: %s", string(body))
	}

	var stripeIntent struct {
		ID       string `json:"id"`
		Amount   int64  `json:"amount"`
		Currency string `json:"currency"`
		Status   string `json:"status"`
		Customer string `json:"customer"`
	}

	if err := json.Unmarshal(body, &stripeIntent); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Verify amount matches
	valid := stripeIntent.Status == "succeeded" &&
		stripeIntent.Amount >= req.ExpectedAmount &&
		strings.EqualFold(stripeIntent.Currency, req.ExpectedCurrency)

	return &PaymentVerification{
		Valid:           valid,
		Message:         fmt.Sprintf("Payment status: %s", stripeIntent.Status),
		PaymentID:       stripeIntent.ID,
		Amount:          stripeIntent.Amount,
		Currency:        strings.ToUpper(stripeIntent.Currency),
		Payer:           stripeIntent.Customer,
		RequiresCapture: stripeIntent.Status == "requires_capture",
		VerifiedAt:      time.Now(),
	}, nil
}

func (s *StripeRail) CapturePayment(ctx context.Context, req *CapturePaymentRequest) (*PaymentCapture, error) {
	// Capture the payment intent
	url := fmt.Sprintf("%s/payment_intents/%s/capture", s.BaseURL, req.PaymentID)

	data := ""
	if req.Amount > 0 {
		data = fmt.Sprintf("amount_to_capture=%d", req.Amount)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+s.SecretKey)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("stripe API error: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stripe error: %s", string(body))
	}

	var stripeIntent struct {
		ID     string `json:"id"`
		Amount int64  `json:"amount"`
		Status string `json:"status"`
	}

	if err := json.Unmarshal(body, &stripeIntent); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Stripe typically charges 2.9% + $0.30
	feeAmount := int64(float64(stripeIntent.Amount)*0.029) + 30

	return &PaymentCapture{
		Success:       stripeIntent.Status == "succeeded",
		TransactionID: stripeIntent.ID,
		GrossAmount:   stripeIntent.Amount,
		NetAmount:     stripeIntent.Amount - feeAmount,
		FeeAmount:     feeAmount,
		CapturedAt:    time.Now(),
	}, nil
}

func (s *StripeRail) RefundPayment(ctx context.Context, req *RefundPaymentRequest) (*PaymentRefund, error) {
	data := "payment_intent=" + req.PaymentID
	if req.Amount > 0 {
		data += fmt.Sprintf("&amount=%d", req.Amount)
	}
	if req.Reason != "" {
		data += "&reason=" + req.Reason
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", s.BaseURL+"/refunds", strings.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+s.SecretKey)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("stripe API error: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stripe error: %s", string(body))
	}

	var stripeRefund struct {
		ID     string `json:"id"`
		Amount int64  `json:"amount"`
		Status string `json:"status"`
	}

	if err := json.Unmarshal(body, &stripeRefund); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &PaymentRefund{
		Success:  stripeRefund.Status == "succeeded",
		RefundID: stripeRefund.ID,
		Amount:   stripeRefund.Amount,
		Status:   stripeRefund.Status,
	}, nil
}

func (s *StripeRail) WebhookHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}

		// Verify webhook signature
		sigHeader := r.Header.Get("Stripe-Signature")
		if !s.verifyWebhookSignature(body, sigHeader) {
			http.Error(w, "Invalid signature", http.StatusBadRequest)
			return
		}

		// Parse event
		var event struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}

		if err := json.Unmarshal(body, &event); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Handle event types
		switch event.Type {
		case "payment_intent.succeeded":
			// Payment successful - grant access
		case "payment_intent.payment_failed":
			// Payment failed - deny access
		case "charge.refunded":
			// Handle refund
		}

		w.WriteHeader(http.StatusOK)
	})
}

func (s *StripeRail) verifyWebhookSignature(payload []byte, sigHeader string) bool {
	if s.WebhookSecret == "" {
		return true // Skip verification if no secret configured
	}

	// Parse signature header: t=timestamp,v1=signature
	parts := strings.Split(sigHeader, ",")
	var timestamp, signature string
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			switch kv[0] {
			case "t":
				timestamp = kv[1]
			case "v1":
				signature = kv[1]
			}
		}
	}

	// Compute expected signature
	signedPayload := timestamp + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(s.WebhookSecret))
	mac.Write([]byte(signedPayload))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expectedSig))
}

// ===============================================
// CRYPTO RAIL IMPLEMENTATION (EVM)
// ===============================================

// EVMCryptoRail implements PaymentRail for EVM-based crypto payments
type EVMCryptoRail struct {
	// RPC endpoints by network
	RPCEndpoints map[NetworkType]string

	// Facilitator URL for verification/settlement
	FacilitatorURL string

	// Supported networks
	Networks []NetworkType

	client *http.Client
}

// NewEVMCryptoRail creates a new EVM crypto payment rail
func NewEVMCryptoRail(facilitatorURL string, networks []NetworkType) *EVMCryptoRail {
	return &EVMCryptoRail{
		FacilitatorURL: facilitatorURL,
		Networks:       networks,
		RPCEndpoints:   make(map[NetworkType]string),
		client:         &http.Client{Timeout: 30 * time.Second},
	}
}

func (e *EVMCryptoRail) ID() string {
	return "evm-crypto"
}

func (e *EVMCryptoRail) DisplayName() string {
	return "Cryptocurrency (Base, Ethereum)"
}

func (e *EVMCryptoRail) Type() RailType {
	return RailTypeCrypto
}

func (e *EVMCryptoRail) SupportedCurrencies() []string {
	return []string{"USDC", "USDT", "ETH", "WETH", "DAI"}
}

func (e *EVMCryptoRail) CreatePaymentIntent(ctx context.Context, req *PaymentIntentRequest) (*PaymentIntent, error) {
	// For crypto, we don't create a payment intent in the traditional sense
	// Instead, we return the payment requirements that the client needs to fulfill
	return &PaymentIntent{
		ID:        fmt.Sprintf("crypto_%d", time.Now().UnixNano()),
		Rail:      e.ID(),
		Amount:    req.Amount,
		Currency:  req.Currency,
		Status:    "requires_payment_method",
		CreatedAt: time.Now(),
		NextAction: &PaymentNextAction{
			Type: "crypto_signature",
		},
		Metadata: req.Metadata,
	}, nil
}

func (e *EVMCryptoRail) VerifyPayment(ctx context.Context, req *VerifyPaymentRequest) (*PaymentVerification, error) {
	// Call facilitator to verify the payment signature
	verifyReq := map[string]interface{}{
		"paymentPayload": req.PaymentPayload,
		"paymentRequirements": map[string]interface{}{
			"maxAmountRequired": fmt.Sprintf("%d", req.ExpectedAmount),
			"payTo":             req.ExpectedPayTo,
			"resource":          req.Resource,
		},
	}

	jsonBody, _ := json.Marshal(verifyReq)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", e.FacilitatorURL+"/verify", strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("facilitator API error: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var verifyResp struct {
		Valid   bool   `json:"valid"`
		Message string `json:"message"`
		Payer   string `json:"payer"`
	}

	if err := json.Unmarshal(body, &verifyResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &PaymentVerification{
		Valid:           verifyResp.Valid,
		Message:         verifyResp.Message,
		PaymentID:       req.PaymentPayload[:16], // Use first 16 chars as ID
		Amount:          req.ExpectedAmount,
		Currency:        req.ExpectedCurrency,
		Payer:           verifyResp.Payer,
		RequiresCapture: true, // Crypto payments need to be settled
		VerifiedAt:      time.Now(),
	}, nil
}

func (e *EVMCryptoRail) CapturePayment(ctx context.Context, req *CapturePaymentRequest) (*PaymentCapture, error) {
	// Call facilitator to settle the payment on-chain
	settleReq := map[string]interface{}{
		"paymentId":      req.PaymentID,
		"settlementData": req.SettlementData,
	}

	jsonBody, _ := json.Marshal(settleReq)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", e.FacilitatorURL+"/settle", strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("facilitator API error: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var settleResp struct {
		Success       bool   `json:"success"`
		TransactionID string `json:"transactionId"`
		BlockNumber   uint64 `json:"blockNumber"`
	}

	if err := json.Unmarshal(body, &settleResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &PaymentCapture{
		Success:       settleResp.Success,
		TransactionID: settleResp.TransactionID,
		GrossAmount:   req.Amount,
		NetAmount:     req.Amount, // No fees for on-chain settlement (gas paid separately)
		FeeAmount:     0,
		CapturedAt:    time.Now(),
	}, nil
}

func (e *EVMCryptoRail) RefundPayment(ctx context.Context, req *RefundPaymentRequest) (*PaymentRefund, error) {
	// Crypto refunds require a new on-chain transaction
	// This would need to be handled differently than fiat refunds
	return nil, fmt.Errorf("crypto refunds require manual on-chain transaction")
}

func (e *EVMCryptoRail) WebhookHandler() http.Handler {
	// Crypto payments don't typically use webhooks
	// Instead, we monitor the blockchain or use the facilitator's callback
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// ===============================================
// PAYMENT RAIL REGISTRY
// ===============================================

// RailRegistry manages registered payment rails
type RailRegistry struct {
	rails map[string]PaymentRail
}

// NewRailRegistry creates a new payment rail registry
func NewRailRegistry() *RailRegistry {
	return &RailRegistry{
		rails: make(map[string]PaymentRail),
	}
}

// Register registers a payment rail
func (r *RailRegistry) Register(rail PaymentRail) {
	r.rails[rail.ID()] = rail
}

// Get retrieves a payment rail by ID
func (r *RailRegistry) Get(id string) (PaymentRail, bool) {
	rail, ok := r.rails[id]
	return rail, ok
}

// List returns all registered rails
func (r *RailRegistry) List() []PaymentRail {
	rails := make([]PaymentRail, 0, len(r.rails))
	for _, rail := range r.rails {
		rails = append(rails, rail)
	}
	return rails
}

// ListByType returns rails of a specific type
func (r *RailRegistry) ListByType(railType RailType) []PaymentRail {
	rails := make([]PaymentRail, 0)
	for _, rail := range r.rails {
		if rail.Type() == railType {
			rails = append(rails, rail)
		}
	}
	return rails
}

// DefaultRailRegistry is the global rail registry
var DefaultRailRegistry = NewRailRegistry()

// ===============================================
// X402 PAYMENT OPTIONS RESPONSE
// ===============================================

// PaymentOption represents a single payment option for the client
type PaymentOption struct {
	// Rail identifier
	Rail string `json:"rail"`

	// Display name for UI
	DisplayName string `json:"displayName"`

	// Type: crypto or fiat
	Type RailType `json:"type"`

	// For crypto: scheme and network
	Scheme  string `json:"scheme,omitempty"`
	Network string `json:"network,omitempty"`

	// Payment details
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`

	// For fiat: client secret to complete payment
	ClientSecret string `json:"clientSecret,omitempty"`

	// For crypto: payment requirements
	PayTo string `json:"payTo,omitempty"`
	Asset string `json:"asset,omitempty"`

	// Estimated fees (for display)
	EstimatedFee int64 `json:"estimatedFee,omitempty"`
}

// PaymentOptionsResponse is the enhanced 402 response with multiple payment options
type PaymentOptionsResponse struct {
	X402Version int `json:"x402Version"`

	// All available payment options
	Options []PaymentOption `json:"options"`

	// Legacy x402 format for backwards compatibility
	Accepts []PaymentRequirements `json:"accepts"`

	// Resource info
	Resource    string `json:"resource"`
	Description string `json:"description"`

	// Error message
	Error string `json:"error,omitempty"`
}
