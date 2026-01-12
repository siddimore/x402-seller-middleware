package x402

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSchemeRegistry(t *testing.T) {
	registry := NewSchemeRegistry()

	// Test registration
	scheme := &ExactEVMScheme{}
	registry.Register(scheme)

	// Test retrieval
	retrieved, ok := registry.Get(SchemeExact)
	if !ok {
		t.Fatal("Expected to find registered scheme")
	}
	if retrieved.Type() != SchemeExact {
		t.Errorf("Expected scheme type %s, got %s", SchemeExact, retrieved.Type())
	}

	// Test listing
	types := registry.List()
	if len(types) != 1 {
		t.Errorf("Expected 1 scheme, got %d", len(types))
	}

	// Test unknown scheme
	_, ok = registry.Get(SchemeStripePayment)
	if ok {
		t.Error("Should not find unregistered scheme")
	}
}

func TestSchemeRegistry_SupportsNetwork(t *testing.T) {
	registry := NewSchemeRegistry()
	registry.Register(&ExactEVMScheme{})

	tests := []struct {
		network  NetworkType
		expected bool
	}{
		{NetworkBaseMainnet, true},
		{NetworkBaseSepolia, true},
		{NetworkEthereumMainnet, true},
		{NetworkSolanaMainnet, false}, // EVM scheme doesn't support Solana
		{NetworkStripe, false},        // EVM scheme doesn't support Stripe
	}

	for _, tt := range tests {
		t.Run(string(tt.network), func(t *testing.T) {
			if got := registry.SupportsNetwork(tt.network); got != tt.expected {
				t.Errorf("SupportsNetwork(%s) = %v, want %v", tt.network, got, tt.expected)
			}
		})
	}
}

func TestWildcardMatch(t *testing.T) {
	tests := []struct {
		pattern  NetworkType
		network  NetworkType
		expected bool
	}{
		{NetworkEVMWildcard, NetworkBaseMainnet, true},
		{NetworkEVMWildcard, NetworkBaseSepolia, true},
		{NetworkEVMWildcard, NetworkEthereumMainnet, true},
		{NetworkSolanaWildcard, NetworkSolanaMainnet, true},
		{NetworkSolanaWildcard, NetworkSolanaDevnet, true},
		{NetworkEVMWildcard, NetworkSolanaMainnet, false},
		{NetworkSolanaWildcard, NetworkBaseMainnet, false},
		{NetworkBaseMainnet, NetworkBaseMainnet, false}, // not a wildcard
	}

	for _, tt := range tests {
		name := string(tt.pattern) + "_" + string(tt.network)
		t.Run(name, func(t *testing.T) {
			if got := isWildcardMatch(tt.pattern, tt.network); got != tt.expected {
				t.Errorf("isWildcardMatch(%s, %s) = %v, want %v", tt.pattern, tt.network, got, tt.expected)
			}
		})
	}
}

func TestMultiSchemeConfig_BuildRequirements(t *testing.T) {
	config := MultiSchemeConfig{
		Config: Config{
			PayTo:             "0x1234567890abcdef",
			PricePerRequest:   1000,
			Currency:          "USDC",
			MaxTimeoutSeconds: 120,
			Description:       "Test payment",
		},
		AcceptedSchemes:  []SchemeType{SchemeExact},
		AcceptedNetworks: []NetworkType{NetworkBaseMainnet, NetworkBaseSepolia},
		PaymentAddresses: map[NetworkType]string{
			NetworkBaseMainnet:  "0xmainnet",
			NetworkBaseSepolia: "0xtestnet",
		},
	}

	requirements := config.BuildMultiSchemeRequirements("/api/test")

	if len(requirements) != 2 {
		t.Fatalf("Expected 2 requirements, got %d", len(requirements))
	}

	// Check first requirement (Base Mainnet)
	req1 := requirements[0]
	if req1.Scheme != string(SchemeExact) {
		t.Errorf("Expected scheme %s, got %s", SchemeExact, req1.Scheme)
	}
	if req1.Network != string(NetworkBaseMainnet) {
		t.Errorf("Expected network %s, got %s", NetworkBaseMainnet, req1.Network)
	}
	if req1.PayTo != "0xmainnet" {
		t.Errorf("Expected payTo 0xmainnet, got %s", req1.PayTo)
	}

	// Check second requirement (Base Sepolia)
	req2 := requirements[1]
	if req2.PayTo != "0xtestnet" {
		t.Errorf("Expected payTo 0xtestnet, got %s", req2.PayTo)
	}
}

func TestMultiSchemeMiddleware_NoPayment(t *testing.T) {
	config := MultiSchemeConfig{
		Config: Config{
			PayTo:           "0x1234567890abcdef",
			PricePerRequest: 1000,
			Currency:        "USDC",
		},
		AcceptedSchemes:  []SchemeType{SchemeExact},
		AcceptedNetworks: []NetworkType{NetworkBaseMainnet, NetworkBaseSepolia},
	}

	handler := MultiSchemeMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), config)

	req := httptest.NewRequest("GET", "/api/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusPaymentRequired {
		t.Errorf("Expected status %d, got %d", http.StatusPaymentRequired, rr.Code)
	}

	// Check PAYMENT-REQUIRED header exists
	paymentHeader := rr.Header().Get("PAYMENT-REQUIRED")
	if paymentHeader == "" {
		t.Error("Expected PAYMENT-REQUIRED header")
	}

	// Decode and verify response
	decoded, err := base64.StdEncoding.DecodeString(paymentHeader)
	if err != nil {
		t.Fatalf("Failed to decode PAYMENT-REQUIRED header: %v", err)
	}

	var response PaymentRequiredResponse
	if err := json.Unmarshal(decoded, &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if len(response.Accepts) != 2 {
		t.Errorf("Expected 2 accepted schemes, got %d", len(response.Accepts))
	}
}

func TestMultiSchemeMiddleware_ExemptPath(t *testing.T) {
	config := MultiSchemeConfig{
		Config: Config{
			PayTo:           "0x1234567890abcdef",
			PricePerRequest: 1000,
			ExemptPaths:     []string{"/health", "/public/"},
		},
	}

	called := false
	handler := MultiSchemeMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}), config)

	// Test exempt path
	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d for exempt path, got %d", http.StatusOK, rr.Code)
	}
	if !called {
		t.Error("Expected handler to be called for exempt path")
	}
}

func TestExactEVMScheme_Verify(t *testing.T) {
	scheme := &ExactEVMScheme{}

	payload := &PaymentPayload{
		Scheme:    SchemeExact,
		Network:   NetworkBaseMainnet,
		Signature: "0xtest",
		Payer:     "0xpayer",
	}

	requirements := &PaymentRequirements{
		Scheme:            string(SchemeExact),
		Network:           string(NetworkBaseMainnet),
		MaxAmountRequired: "1000",
		PayTo:             "0xreceiver",
	}

	result, err := scheme.Verify(context.Background(), payload, requirements)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !result.Valid {
		t.Error("Expected valid verification result")
	}
}

func TestStripeScheme_ReturnsError(t *testing.T) {
	scheme := &StripeScheme{SecretKey: "test", Sandbox: true}

	if scheme.Type() != SchemeStripePayment {
		t.Errorf("Expected type %s, got %s", SchemeStripePayment, scheme.Type())
	}

	networks := scheme.SupportedNetworks()
	if len(networks) != 1 || networks[0] != NetworkStripeTest {
		t.Error("Expected Stripe sandbox scheme to support stripe:test network")
	}

	// Verify should return error (use StripeRail instead)
	_, err := scheme.Verify(context.Background(), &PaymentPayload{}, &PaymentRequirements{})
	if err == nil {
		t.Error("Expected error directing to use StripeRail")
	}
}

func TestParsePaymentPayload(t *testing.T) {
	payload := PaymentPayload{
		Scheme:    SchemeExact,
		Network:   NetworkBaseMainnet,
		Signature: "0xtest",
		Payer:     "0xpayer",
		Timestamp: 1234567890,
	}

	jsonBytes, _ := json.Marshal(payload)
	encoded := base64.StdEncoding.EncodeToString(jsonBytes)

	parsed, err := parsePaymentPayload(encoded)
	if err != nil {
		t.Fatalf("Failed to parse payload: %v", err)
	}

	if parsed.Scheme != SchemeExact {
		t.Errorf("Expected scheme %s, got %s", SchemeExact, parsed.Scheme)
	}
	if parsed.Network != NetworkBaseMainnet {
		t.Errorf("Expected network %s, got %s", NetworkBaseMainnet, parsed.Network)
	}
	if parsed.Payer != "0xpayer" {
		t.Errorf("Expected payer 0xpayer, got %s", parsed.Payer)
	}
}

func TestDefaultRegistry_HasEVMScheme(t *testing.T) {
	scheme, ok := DefaultRegistry.Get(SchemeExact)
	if !ok {
		t.Fatal("DefaultRegistry should have exact scheme registered")
	}

	networks := scheme.SupportedNetworks()
	hasBase := false
	for _, n := range networks {
		if n == NetworkBaseMainnet || n == NetworkEVMWildcard {
			hasBase = true
			break
		}
	}

	if !hasBase {
		t.Error("Exact scheme should support Base network")
	}
}
