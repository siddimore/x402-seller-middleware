// Package x402 provides multi-scheme payment support for HTTP 402 Payment Required.
// This file defines the extensible scheme/network architecture that supports:
// - Crypto payments (EVM chains like Base, Ethereum; SVM chains like Solana)
// - Fiat payments via Stripe (wrapped in x402 protocol)
package x402

import (
	"context"
	"fmt"
	"sync"
)

// SchemeType represents the type of payment scheme
type SchemeType string

const (
	// Crypto schemes (official x402)
	SchemeExact SchemeType = "exact" // Exact amount transfer (EIP-3009 for EVM, SPL for SVM)
	SchemeUpto  SchemeType = "upto"  // Up-to amount (metered/streaming payments)

	// Fiat scheme (Stripe wrapped in x402 protocol)
	SchemeStripePayment SchemeType = "stripe-payment" // Stripe integration
)

// NetworkType represents the payment network
type NetworkType string

const (
	// EVM Networks (CAIP-2 format)
	NetworkEthereumMainnet NetworkType = "eip155:1"
	NetworkBaseMainnet     NetworkType = "eip155:8453"
	NetworkBaseSepolia     NetworkType = "eip155:84532"
	NetworkOptimism        NetworkType = "eip155:10"
	NetworkArbitrum        NetworkType = "eip155:42161"
	NetworkPolygon         NetworkType = "eip155:137"
	NetworkEVMWildcard     NetworkType = "eip155:*" // All EVM chains

	// SVM Networks (CAIP-2 format)
	NetworkSolanaMainnet  NetworkType = "solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp"
	NetworkSolanaDevnet   NetworkType = "solana:EtWTRABZaYq6iMfeYKouRu166VU2xqa1"
	NetworkSolanaTestnet  NetworkType = "solana:4uhcVJyU9pJkvQyS88uRDiswHXSCkY3z"
	NetworkSolanaWildcard NetworkType = "solana:*" // All Solana networks

	// Stripe Networks (for fiat via Stripe)
	NetworkStripe     NetworkType = "stripe:live"
	NetworkStripeTest NetworkType = "stripe:test"
)

// PaymentScheme defines a payment mechanism
type PaymentScheme interface {
	// Type returns the scheme type identifier
	Type() SchemeType

	// SupportedNetworks returns the networks this scheme supports
	SupportedNetworks() []NetworkType

	// Verify verifies a payment payload for this scheme
	Verify(ctx context.Context, payload *PaymentPayload, requirements *PaymentRequirements) (*VerificationResult, error)

	// Settle settles a verified payment (optional, can delegate to facilitator)
	Settle(ctx context.Context, payload *PaymentPayload, requirements *PaymentRequirements) (*SettlementResult, error)
}

// PaymentPayload represents an x402 payment payload from the client
type PaymentPayload struct {
	// Core x402 fields
	Scheme    SchemeType  `json:"scheme"`
	Network   NetworkType `json:"network"`
	Payload   string      `json:"payload"`   // Scheme-specific payload (signature, token, etc.)
	Resource  string      `json:"resource"`  // Resource being paid for
	Timestamp int64       `json:"timestamp"` // Unix timestamp

	// Crypto-specific fields
	Signature string `json:"signature,omitempty"` // Payment signature (EIP-3009, etc.)
	Payer     string `json:"payer,omitempty"`     // Payer address
	Nonce     string `json:"nonce,omitempty"`     // Replay protection

	// Fiat-specific fields (future)
	CardToken       string `json:"cardToken,omitempty"`       // Tokenized card (Visa, etc.)
	PaymentIntentID string `json:"paymentIntentId,omitempty"` // Stripe payment intent
	AuthCode        string `json:"authCode,omitempty"`        // Authorization code
}

// VerificationResult contains the result of payment verification
type VerificationResult struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message,omitempty"`

	// Details
	Scheme    SchemeType  `json:"scheme"`
	Network   NetworkType `json:"network"`
	Amount    string      `json:"amount"`
	Payer     string      `json:"payer,omitempty"`
	PayTo     string      `json:"payTo"`
	Timestamp int64       `json:"timestamp"`

	// For schemes that pre-authorize
	AuthorizationID string `json:"authorizationId,omitempty"`
}

// SettlementResult contains the result of payment settlement
type SettlementResult struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`

	// Transaction details
	TransactionID  string `json:"transactionId,omitempty"`  // Blockchain tx hash or payment ID
	TransactionURL string `json:"transactionUrl,omitempty"` // Explorer link
	SettledAmount  string `json:"settledAmount,omitempty"`
	SettledAt      int64  `json:"settledAt,omitempty"`

	// Network-specific
	BlockNumber   uint64 `json:"blockNumber,omitempty"` // For crypto
	Confirmations int    `json:"confirmations,omitempty"`
}

// SchemeRegistry manages registered payment schemes
type SchemeRegistry struct {
	mu      sync.RWMutex
	schemes map[SchemeType]PaymentScheme
}

// NewSchemeRegistry creates a new scheme registry
func NewSchemeRegistry() *SchemeRegistry {
	return &SchemeRegistry{
		schemes: make(map[SchemeType]PaymentScheme),
	}
}

// Register registers a payment scheme
func (r *SchemeRegistry) Register(scheme PaymentScheme) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.schemes[scheme.Type()] = scheme
}

// Get retrieves a payment scheme by type
func (r *SchemeRegistry) Get(schemeType SchemeType) (PaymentScheme, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	scheme, ok := r.schemes[schemeType]
	return scheme, ok
}

// List returns all registered scheme types
func (r *SchemeRegistry) List() []SchemeType {
	r.mu.RLock()
	defer r.mu.RUnlock()
	types := make([]SchemeType, 0, len(r.schemes))
	for t := range r.schemes {
		types = append(types, t)
	}
	return types
}

// SupportsNetwork checks if any registered scheme supports the given network
func (r *SchemeRegistry) SupportsNetwork(network NetworkType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, scheme := range r.schemes {
		for _, n := range scheme.SupportedNetworks() {
			if n == network || isWildcardMatch(n, network) {
				return true
			}
		}
	}
	return false
}

// isWildcardMatch checks if a wildcard network matches a specific network
func isWildcardMatch(pattern, network NetworkType) bool {
	// eip155:* matches eip155:8453
	// solana:* matches solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp
	if len(pattern) < 2 || pattern[len(pattern)-1] != '*' {
		return false
	}
	prefix := pattern[:len(pattern)-1] // "eip155:" or "solana:"
	return len(network) > len(prefix) && string(network[:len(prefix)]) == string(prefix)
}

// DefaultRegistry is the global scheme registry
var DefaultRegistry = NewSchemeRegistry()

// MultiSchemeConfig extends Config to support multiple payment schemes
type MultiSchemeConfig struct {
	Config

	// AcceptedSchemes lists the payment schemes this endpoint accepts
	// If empty, defaults to [SchemeExact]
	AcceptedSchemes []SchemeType

	// AcceptedNetworks lists the networks this endpoint accepts
	// If empty, defaults to the Network field in Config
	AcceptedNetworks []NetworkType

	// PaymentAddresses maps networks to payment addresses
	// Allows different addresses for different chains/payment methods
	PaymentAddresses map[NetworkType]string

	// FacilitatorURLs maps networks to their facilitator endpoints
	FacilitatorURLs map[NetworkType]string

	// SchemeRegistry is the registry of payment schemes (uses DefaultRegistry if nil)
	SchemeRegistry *SchemeRegistry
}

// BuildMultiSchemeRequirements generates PaymentRequirements for all accepted schemes/networks
func (c *MultiSchemeConfig) BuildMultiSchemeRequirements(resource string) []PaymentRequirements {
	var requirements []PaymentRequirements

	schemes := c.AcceptedSchemes
	if len(schemes) == 0 {
		schemes = []SchemeType{SchemeExact}
	}

	networks := c.AcceptedNetworks
	if len(networks) == 0 && c.Network != "" {
		networks = []NetworkType{NetworkType(c.Network)}
	}

	maxTimeout := c.MaxTimeoutSeconds
	if maxTimeout == 0 {
		maxTimeout = 60
	}

	description := c.Description
	if description == "" {
		description = fmt.Sprintf("Payment of %d %s required", c.PricePerRequest, c.Currency)
	}

	for _, scheme := range schemes {
		for _, network := range networks {
			// Get payment address for this network (or use default)
			payTo := c.PayTo
			if addr, ok := c.PaymentAddresses[network]; ok {
				payTo = addr
			}

			req := PaymentRequirements{
				Scheme:            string(scheme),
				Network:           string(network),
				MaxAmountRequired: fmt.Sprintf("%d", c.PricePerRequest),
				Resource:          resource,
				Description:       description,
				PayTo:             payTo,
				MaxTimeoutSeconds: maxTimeout,
				Asset:             c.Asset,
				OutputSchema:      nil,
			}

			// Add facilitator URL if configured
			if facilitatorURL, ok := c.FacilitatorURLs[network]; ok {
				if req.Extra == nil {
					req.Extra = make(map[string]interface{})
				}
				req.Extra["facilitatorUrl"] = facilitatorURL
			}

			requirements = append(requirements, req)
		}
	}

	return requirements
}

// Example scheme implementations (stubs for future)

// ExactEVMScheme implements the exact payment scheme for EVM chains
type ExactEVMScheme struct {
	// RPC endpoints for verification
	RPCEndpoints map[NetworkType]string
}

func (s *ExactEVMScheme) Type() SchemeType {
	return SchemeExact
}

func (s *ExactEVMScheme) SupportedNetworks() []NetworkType {
	return []NetworkType{
		NetworkEthereumMainnet,
		NetworkBaseMainnet,
		NetworkBaseSepolia,
		NetworkOptimism,
		NetworkArbitrum,
		NetworkPolygon,
		NetworkEVMWildcard,
	}
}

func (s *ExactEVMScheme) Verify(ctx context.Context, payload *PaymentPayload, requirements *PaymentRequirements) (*VerificationResult, error) {
	// TODO: Implement EIP-3009 signature verification
	// This would verify the transferWithAuthorization signature
	return &VerificationResult{
		Valid:   true,
		Message: "EVM verification delegated to facilitator",
		Scheme:  SchemeExact,
		Network: payload.Network,
	}, nil
}

func (s *ExactEVMScheme) Settle(ctx context.Context, payload *PaymentPayload, requirements *PaymentRequirements) (*SettlementResult, error) {
	// TODO: Implement on-chain settlement or delegate to facilitator
	return &SettlementResult{
		Success: true,
		Message: "Settlement delegated to facilitator",
	}, nil
}

// StripeScheme implements the Stripe payment scheme (fiat via Stripe wrapped in x402)
type StripeScheme struct {
	SecretKey string
	Sandbox   bool
}

func (s *StripeScheme) Type() SchemeType {
	return SchemeStripePayment
}

func (s *StripeScheme) SupportedNetworks() []NetworkType {
	if s.Sandbox {
		return []NetworkType{NetworkStripeTest}
	}
	return []NetworkType{NetworkStripe}
}

func (s *StripeScheme) Verify(ctx context.Context, payload *PaymentPayload, requirements *PaymentRequirements) (*VerificationResult, error) {
	// Stripe verification is handled by the StripeRail in payment_rails.go
	// This scheme is for compatibility with the scheme registry
	return nil, fmt.Errorf("use StripeRail for Stripe payment verification")
}

func (s *StripeScheme) Settle(ctx context.Context, payload *PaymentPayload, requirements *PaymentRequirements) (*SettlementResult, error) {
	// Stripe capture is handled by the StripeRail in payment_rails.go
	return nil, fmt.Errorf("use StripeRail for Stripe payment capture")
}

// RegisterDefaultSchemes registers the default payment schemes
func RegisterDefaultSchemes() {
	DefaultRegistry.Register(&ExactEVMScheme{})
}

func init() {
	RegisterDefaultSchemes()
}
