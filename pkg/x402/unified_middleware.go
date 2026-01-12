// Package x402 provides unified payment middleware for HTTP 402 Payment Required.
// This file implements the unified middleware that supports:
// - Multiple payment rails (crypto + fiat)
// - AI agents, human users, and HTTP clients
// - Onboarding flow for payment method selection
package x402

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// ===============================================
// UNIFIED PAYMENT CONFIG
// ===============================================

// UnifiedPaymentConfig configures the unified payment middleware
type UnifiedPaymentConfig struct {
	// Basic config
	PricePerRequest int64    // Amount in smallest unit (cents, wei, etc.)
	Currency        string   // Primary currency (USD, USDC)
	Description     string   // What the payment is for
	ExemptPaths     []string // Paths that don't require payment

	// Crypto settings
	CryptoEnabled  bool          // Enable crypto payments
	CryptoPayTo    string        // Address to receive crypto payments
	CryptoAsset    string        // Token contract (USDC, etc.)
	CryptoScheme   string        // Payment scheme (exact, upto)
	CryptoNetworks []NetworkType // Supported networks

	// Fiat settings
	FiatEnabled         bool   // Enable fiat payments
	StripeSecretKey     string // Stripe API key
	StripeWebhookSecret string // Stripe webhook secret

	// Facilitator for crypto verification
	FacilitatorURL string

	// Customer/session management
	EnableSessions bool // Track customer sessions
	SessionStore   SessionStore

	// Callbacks
	OnPaymentSuccess func(ctx context.Context, payment *CompletedPayment)
	OnPaymentFailed  func(ctx context.Context, err error, req *http.Request)

	// Rail registry (uses default if nil)
	RailRegistry *RailRegistry
}

// CompletedPayment represents a successfully completed payment
type CompletedPayment struct {
	ID            string            `json:"id"`
	Rail          string            `json:"rail"`
	Type          RailType          `json:"type"`
	Amount        int64             `json:"amount"`
	Currency      string            `json:"currency"`
	Resource      string            `json:"resource"`
	Payer         string            `json:"payer,omitempty"`
	TransactionID string            `json:"transactionId,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	CompletedAt   time.Time         `json:"completedAt"`
}

// ===============================================
// CUSTOMER PAYMENT PREFERENCES
// ===============================================

// CustomerPaymentPrefs stores a customer's payment preferences
type CustomerPaymentPrefs struct {
	CustomerID       string    `json:"customerId"`
	PreferredRail    string    `json:"preferredRail"`    // stripe, evm-crypto, etc.
	PreferredNetwork string    `json:"preferredNetwork"` // For crypto
	StripeCustomerID string    `json:"stripeCustomerId,omitempty"`
	CryptoAddress    string    `json:"cryptoAddress,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// PaymentPrefsStore stores customer payment preferences
type PaymentPrefsStore interface {
	Get(ctx context.Context, customerID string) (*CustomerPaymentPrefs, error)
	Set(ctx context.Context, prefs *CustomerPaymentPrefs) error
	Delete(ctx context.Context, customerID string) error
}

// InMemoryPaymentPrefsStore is an in-memory implementation
type InMemoryPaymentPrefsStore struct {
	mu    sync.RWMutex
	prefs map[string]*CustomerPaymentPrefs
}

func NewInMemoryPaymentPrefsStore() *InMemoryPaymentPrefsStore {
	return &InMemoryPaymentPrefsStore{
		prefs: make(map[string]*CustomerPaymentPrefs),
	}
}

func (s *InMemoryPaymentPrefsStore) Get(ctx context.Context, customerID string) (*CustomerPaymentPrefs, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	prefs, ok := s.prefs[customerID]
	if !ok {
		return nil, nil
	}
	return prefs, nil
}

func (s *InMemoryPaymentPrefsStore) Set(ctx context.Context, prefs *CustomerPaymentPrefs) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prefs.UpdatedAt = time.Now()
	s.prefs[prefs.CustomerID] = prefs
	return nil
}

func (s *InMemoryPaymentPrefsStore) Delete(ctx context.Context, customerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.prefs, customerID)
	return nil
}

// ===============================================
// UNIFIED PAYMENT MIDDLEWARE
// ===============================================

// UnifiedPaymentMiddleware creates middleware that accepts multiple payment rails
func UnifiedPaymentMiddleware(next http.Handler, config UnifiedPaymentConfig) http.Handler {
	// Set defaults
	if config.Currency == "" {
		config.Currency = "USD"
	}
	if config.CryptoScheme == "" {
		config.CryptoScheme = "exact"
	}

	// Get or create rail registry
	registry := config.RailRegistry
	if registry == nil {
		registry = NewRailRegistry()

		// Register Stripe if enabled
		if config.FiatEnabled && config.StripeSecretKey != "" {
			registry.Register(NewStripeRail(config.StripeSecretKey, config.StripeWebhookSecret))
		}

		// Register EVM crypto if enabled
		if config.CryptoEnabled && config.FacilitatorURL != "" {
			registry.Register(NewEVMCryptoRail(config.FacilitatorURL, config.CryptoNetworks))
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if path is exempt
		if isExemptPath(r.URL.Path, config.ExemptPaths) {
			next.ServeHTTP(w, r)
			return
		}

		// Check for payment proof in headers
		paymentProof := extractPaymentProof(r)

		if paymentProof == nil {
			// No payment - return 402 with options
			sendPaymentOptions(w, r, config, registry)
			return
		}

		// Get the appropriate rail
		rail, ok := registry.Get(paymentProof.Rail)
		if !ok {
			sendPaymentOptions(w, r, config, registry)
			return
		}

		// Build resource URL
		resource := r.URL.Path
		if r.URL.RawQuery != "" {
			resource += "?" + r.URL.RawQuery
		}

		// Verify payment
		verification, err := rail.VerifyPayment(r.Context(), &VerifyPaymentRequest{
			PaymentPayload:   paymentProof.Payload,
			PaymentIntentID:  paymentProof.PaymentIntentID,
			PaymentToken:     paymentProof.Token,
			ExpectedAmount:   config.PricePerRequest,
			ExpectedCurrency: config.Currency,
			ExpectedPayTo:    config.CryptoPayTo,
			Resource:         resource,
		})

		if err != nil || !verification.Valid {
			if config.OnPaymentFailed != nil {
				config.OnPaymentFailed(r.Context(), err, r)
			}
			sendPaymentOptions(w, r, config, registry)
			return
		}

		// Capture payment if needed
		if verification.RequiresCapture {
			capture, err := rail.CapturePayment(r.Context(), &CapturePaymentRequest{
				PaymentID: verification.PaymentID,
				Amount:    config.PricePerRequest,
			})

			if err != nil || !capture.Success {
				if config.OnPaymentFailed != nil {
					config.OnPaymentFailed(r.Context(), err, r)
				}
				sendPaymentOptions(w, r, config, registry)
				return
			}

			// Call success callback
			if config.OnPaymentSuccess != nil {
				config.OnPaymentSuccess(r.Context(), &CompletedPayment{
					ID:            verification.PaymentID,
					Rail:          rail.ID(),
					Type:          rail.Type(),
					Amount:        capture.GrossAmount,
					Currency:      verification.Currency,
					Resource:      resource,
					Payer:         verification.Payer,
					TransactionID: capture.TransactionID,
					CompletedAt:   time.Now(),
				})
			}
		}

		// Payment verified - add headers and continue
		w.Header().Set("X-Payment-Verified", "true")
		w.Header().Set("X-Payment-Rail", rail.ID())
		w.Header().Set("X-Payment-ID", verification.PaymentID)
		w.Header().Set("X-Payment-Timestamp", time.Now().Format(time.RFC3339))

		next.ServeHTTP(w, r)
	})
}

// PaymentProof contains proof of payment from the client
type PaymentProof struct {
	// Which rail was used
	Rail string `json:"rail"`

	// For crypto: payment payload/signature
	Payload string `json:"payload,omitempty"`

	// For fiat: payment intent ID or token
	PaymentIntentID string `json:"paymentIntentId,omitempty"`
	Token           string `json:"token,omitempty"`
}

// extractPaymentProof extracts payment proof from request headers
func extractPaymentProof(r *http.Request) *PaymentProof {
	// Check X-PAYMENT-PROOF header (unified format)
	if proofHeader := r.Header.Get("X-PAYMENT-PROOF"); proofHeader != "" {
		decoded, err := base64.StdEncoding.DecodeString(proofHeader)
		if err == nil {
			var proof PaymentProof
			if json.Unmarshal(decoded, &proof) == nil {
				return &proof
			}
		}
	}

	// Check PAYMENT-SIGNATURE header (x402 crypto format)
	if paymentSig := r.Header.Get("PAYMENT-SIGNATURE"); paymentSig != "" {
		return &PaymentProof{
			Rail:    "evm-crypto",
			Payload: paymentSig,
		}
	}

	// Check X-PAYMENT header (x402 v1 format)
	if xPayment := r.Header.Get("X-PAYMENT"); xPayment != "" {
		return &PaymentProof{
			Rail:    "evm-crypto",
			Payload: xPayment,
		}
	}

	// Check X-STRIPE-PAYMENT-INTENT header (Stripe format)
	if stripePI := r.Header.Get("X-STRIPE-PAYMENT-INTENT"); stripePI != "" {
		return &PaymentProof{
			Rail:            "stripe",
			PaymentIntentID: stripePI,
		}
	}

	// Check query parameters (for redirects from Stripe checkout)
	if pi := r.URL.Query().Get("payment_intent"); pi != "" {
		return &PaymentProof{
			Rail:            "stripe",
			PaymentIntentID: pi,
		}
	}

	return nil
}

// sendPaymentOptions sends a 402 response with all available payment options
func sendPaymentOptions(w http.ResponseWriter, r *http.Request, config UnifiedPaymentConfig, registry *RailRegistry) {
	resource := r.URL.Path
	if r.URL.RawQuery != "" {
		resource += "?" + r.URL.RawQuery
	}

	var options []PaymentOption
	var accepts []PaymentRequirements

	// Add crypto options
	if config.CryptoEnabled {
		for _, network := range config.CryptoNetworks {
			option := PaymentOption{
				Rail:         "evm-crypto",
				DisplayName:  fmt.Sprintf("Pay with Crypto (%s)", networkDisplayName(network)),
				Type:         RailTypeCrypto,
				Scheme:       config.CryptoScheme,
				Network:      string(network),
				Amount:       config.PricePerRequest,
				Currency:     config.Currency,
				PayTo:        config.CryptoPayTo,
				Asset:        config.CryptoAsset,
				EstimatedFee: 0, // Gas paid by sender
			}
			options = append(options, option)

			// Legacy x402 format
			accepts = append(accepts, PaymentRequirements{
				Scheme:            config.CryptoScheme,
				Network:           string(network),
				MaxAmountRequired: fmt.Sprintf("%d", config.PricePerRequest),
				Resource:          resource,
				Description:       config.Description,
				PayTo:             config.CryptoPayTo,
				MaxTimeoutSeconds: 60,
				Asset:             config.CryptoAsset,
			})
		}
	}

	// Add Stripe option
	if config.FiatEnabled && config.StripeSecretKey != "" {
		stripeRail := NewStripeRail(config.StripeSecretKey, config.StripeWebhookSecret)

		// Create payment intent
		intent, err := stripeRail.CreatePaymentIntent(r.Context(), &PaymentIntentRequest{
			Amount:      config.PricePerRequest,
			Currency:    config.Currency,
			Resource:    resource,
			Description: config.Description,
			Metadata: map[string]string{
				"resource": resource,
			},
		})

		if err == nil {
			// Calculate estimated Stripe fee (2.9% + $0.30)
			estimatedFee := int64(float64(config.PricePerRequest)*0.029) + 30

			option := PaymentOption{
				Rail:         "stripe",
				DisplayName:  "Pay with Card (Visa, Mastercard)",
				Type:         RailTypeFiat,
				Amount:       config.PricePerRequest,
				Currency:     config.Currency,
				ClientSecret: intent.ClientSecret,
				EstimatedFee: estimatedFee,
			}
			options = append(options, option)
		}
	}

	// Build response
	response := PaymentOptionsResponse{
		X402Version: X402Version,
		Options:     options,
		Accepts:     accepts,
		Resource:    resource,
		Description: config.Description,
		Error:       "Payment required - select a payment method",
	}

	// Encode for PAYMENT-REQUIRED header
	responseJSON, _ := json.Marshal(response)
	paymentRequiredHeader := base64.StdEncoding.EncodeToString(responseJSON)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("PAYMENT-REQUIRED", paymentRequiredHeader)

	// Add CORS headers for browser clients
	w.Header().Set("Access-Control-Expose-Headers", "PAYMENT-REQUIRED")

	w.WriteHeader(http.StatusPaymentRequired)
	_ = json.NewEncoder(w).Encode(response)
}

// networkDisplayName returns a human-friendly name for a network
func networkDisplayName(network NetworkType) string {
	switch network {
	case NetworkBaseMainnet:
		return "Base"
	case NetworkBaseSepolia:
		return "Base Sepolia"
	case NetworkEthereumMainnet:
		return "Ethereum"
	case NetworkOptimism:
		return "Optimism"
	case NetworkArbitrum:
		return "Arbitrum"
	case NetworkPolygon:
		return "Polygon"
	case NetworkSolanaMainnet:
		return "Solana"
	default:
		return string(network)
	}
}

// ===============================================
// AI AGENT PAYMENT SUPPORT
// ===============================================

// AIAgentPaymentConfig configures AI agent payment handling
type AIAgentPaymentConfig struct {
	// Allow agents to use crypto
	AllowCrypto bool

	// Allow agents to use fiat (via pre-authorized cards)
	AllowFiat bool

	// Budget limits for agents
	MaxRequestBudget int64 // Max per request
	MaxSessionBudget int64 // Max per session
	MaxDailyBudget   int64 // Max per day

	// Pre-authorized payment methods
	PreAuthStore PreAuthStore
}

// AIAgentPaymentMiddleware adds AI agent payment support to the unified middleware
func AIAgentPaymentMiddleware(next http.Handler, config UnifiedPaymentConfig, agentConfig AIAgentPaymentConfig) http.Handler {
	unified := UnifiedPaymentMiddleware(next, config)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if this is an AI agent
		if !isAIAgent(r) {
			unified.ServeHTTP(w, r)
			return
		}

		// Parse agent budget header
		agentInfo := ParseAIAgentHeaders(r)

		// Check for pre-authorized budget using agent task ID or agent header
		agentID := agentInfo.AgentTaskID
		if agentID == "" {
			agentID = r.Header.Get("X-Agent-ID")
		}

		if agentConfig.PreAuthStore != nil && agentID != "" {
			preAuth, err := agentConfig.PreAuthStore.GetByAgentID(agentID)
			if err == nil && preAuth != nil {
				// Check if agent has sufficient pre-auth budget
				if preAuth.Remaining >= config.PricePerRequest {
					// Deduct from pre-auth
					err := agentConfig.PreAuthStore.Deduct(preAuth.ID, config.PricePerRequest)
					if err == nil {
						// Payment covered by pre-auth - get updated budget
						updatedPreAuth, _ := agentConfig.PreAuthStore.Get(preAuth.ID)
						remaining := int64(0)
						if updatedPreAuth != nil {
							remaining = updatedPreAuth.Remaining
						}

						// Payment covered by pre-auth
						w.Header().Set("X-Payment-Verified", "true")
						w.Header().Set("X-Payment-Method", "pre-auth")
						w.Header().Set("X-Remaining-Budget", fmt.Sprintf("%d", remaining))
						next.ServeHTTP(w, r)
						return
					}
				}
			}
		}

		// Check agent budget constraints
		if agentInfo.AgentBudget > 0 && agentInfo.AgentBudget < config.PricePerRequest {
			// Agent budget is insufficient
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusPaymentRequired)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error":           "Insufficient agent budget",
				"required":        config.PricePerRequest,
				"agentBudget":     agentInfo.AgentBudget,
				"currency":        config.Currency,
				"suggestedAction": "Increase budget or use different payment method",
			})
			return
		}

		// Fall back to standard payment flow
		unified.ServeHTTP(w, r)
	})
}

// ===============================================
// PAYMENT ONBOARDING HANDLERS
// ===============================================

// OnboardingHandler returns HTTP handlers for payment method onboarding
type OnboardingHandler struct {
	config   UnifiedPaymentConfig
	registry *RailRegistry
	prefs    PaymentPrefsStore
}

// NewOnboardingHandler creates a new onboarding handler
func NewOnboardingHandler(config UnifiedPaymentConfig, prefs PaymentPrefsStore) *OnboardingHandler {
	registry := config.RailRegistry
	if registry == nil {
		registry = NewRailRegistry()
		if config.FiatEnabled && config.StripeSecretKey != "" {
			registry.Register(NewStripeRail(config.StripeSecretKey, config.StripeWebhookSecret))
		}
		if config.CryptoEnabled {
			registry.Register(NewEVMCryptoRail(config.FacilitatorURL, config.CryptoNetworks))
		}
	}

	return &OnboardingHandler{
		config:   config,
		registry: registry,
		prefs:    prefs,
	}
}

// ListPaymentMethods returns available payment methods
func (h *OnboardingHandler) ListPaymentMethods(w http.ResponseWriter, r *http.Request) {
	methods := make([]map[string]interface{}, 0)

	for _, rail := range h.registry.List() {
		methods = append(methods, map[string]interface{}{
			"id":          rail.ID(),
			"displayName": rail.DisplayName(),
			"type":        rail.Type(),
			"currencies":  rail.SupportedCurrencies(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"paymentMethods": methods,
	})
}

// SetPreferredMethod sets a customer's preferred payment method
func (h *OnboardingHandler) SetPreferredMethod(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CustomerID string `json:"customerId"`
		Rail       string `json:"rail"`
		Network    string `json:"network,omitempty"`
		CryptoAddr string `json:"cryptoAddress,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Validate rail exists
	if _, ok := h.registry.Get(req.Rail); !ok {
		http.Error(w, "Unknown payment rail", http.StatusBadRequest)
		return
	}

	prefs := &CustomerPaymentPrefs{
		CustomerID:       req.CustomerID,
		PreferredRail:    req.Rail,
		PreferredNetwork: req.Network,
		CryptoAddress:    req.CryptoAddr,
		CreatedAt:        time.Now(),
	}

	if err := h.prefs.Set(r.Context(), prefs); err != nil {
		http.Error(w, "Failed to save preferences", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"preferences": prefs,
	})
}

// CreateStripeSetupIntent creates a Stripe SetupIntent for saving card
func (h *OnboardingHandler) CreateStripeSetupIntent(w http.ResponseWriter, r *http.Request) {
	if !h.config.FiatEnabled || h.config.StripeSecretKey == "" {
		http.Error(w, "Stripe not enabled", http.StatusBadRequest)
		return
	}

	var req struct {
		CustomerID string `json:"customerId"`
		Email      string `json:"email,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// In production, you'd create a Stripe Customer first if needed
	// Then create a SetupIntent for saving the payment method

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"clientSecret": "seti_xxx_secret_xxx", // Would come from Stripe API
		"instructions": "Use Stripe.js to collect and save payment method",
	})
}

// GetPreferences gets a customer's payment preferences
func (h *OnboardingHandler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	customerID := r.URL.Query().Get("customerId")
	if customerID == "" {
		http.Error(w, "customerId required", http.StatusBadRequest)
		return
	}

	prefs, err := h.prefs.Get(r.Context(), customerID)
	if err != nil {
		http.Error(w, "Failed to get preferences", http.StatusInternalServerError)
		return
	}

	if prefs == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"hasPreferences": false,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"hasPreferences": true,
		"preferences":    prefs,
	})
}
