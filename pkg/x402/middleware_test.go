package x402

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddleware_NoToken(t *testing.T) {
	handler := createTestHandler()
	wrapped := Middleware(handler, testConfig())

	req := httptest.NewRequest("GET", "/api/protected", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPaymentRequired {
		t.Errorf("Expected status 402, got %d", resp.StatusCode)
	}

	// Verify PAYMENT-REQUIRED header exists (x402 v2)
	paymentRequiredHeader := resp.Header.Get("PAYMENT-REQUIRED")
	if paymentRequiredHeader == "" {
		t.Error("Expected PAYMENT-REQUIRED header")
	}

	// Verify response body (x402 format)
	var x402Response PaymentRequiredResponse
	if err := json.NewDecoder(resp.Body).Decode(&x402Response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if x402Response.X402Version != X402Version {
		t.Errorf("Expected x402Version %d, got %d", X402Version, x402Response.X402Version)
	}

	if len(x402Response.Accepts) == 0 {
		t.Fatal("Expected at least one payment requirement")
	}

	if x402Response.Accepts[0].MaxAmountRequired != "100" {
		t.Errorf("Expected maxAmountRequired '100', got '%s'", x402Response.Accepts[0].MaxAmountRequired)
	}
}

func TestMiddleware_ValidToken(t *testing.T) {
	handler := createTestHandler()
	wrapped := Middleware(handler, testConfig())

	req := httptest.NewRequest("GET", "/api/protected", nil)
	req.Header.Set("Authorization", "Bearer valid_token123")
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status 200, got %d. Body: %s", resp.StatusCode, string(body))
	}

	if resp.Header.Get("X-Payment-Verified") != "true" {
		t.Error("Expected X-Payment-Verified header")
	}
}

func TestMiddleware_InvalidToken(t *testing.T) {
	handler := createTestHandler()
	wrapped := Middleware(handler, testConfig())

	req := httptest.NewRequest("GET", "/api/protected", nil)
	req.Header.Set("Authorization", "Bearer invalid_token")
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPaymentRequired {
		t.Errorf("Expected status 402, got %d", resp.StatusCode)
	}
}

func TestMiddleware_ExemptPath(t *testing.T) {
	handler := createTestHandler()
	wrapped := Middleware(handler, testConfig())

	req := httptest.NewRequest("GET", "/public/resource", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for exempt path, got %d", resp.StatusCode)
	}
}

func TestMiddleware_XPaymentTokenHeader(t *testing.T) {
	handler := createTestHandler()
	wrapped := Middleware(handler, testConfig())

	req := httptest.NewRequest("GET", "/api/protected", nil)
	req.Header.Set("X-Payment-Token", "valid_token456")
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestMiddleware_QueryParam(t *testing.T) {
	handler := createTestHandler()
	wrapped := Middleware(handler, testConfig())

	req := httptest.NewRequest("GET", "/api/protected?payment_token=valid_token789", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestMiddleware_X402PaymentHeader(t *testing.T) {
	handler := createTestHandler()
	wrapped := Middleware(handler, testConfig())

	// x402 protocol uses X-PAYMENT header with base64-encoded payment payload
	// For testing, we just use a simple token that passes the default verifier
	req := httptest.NewRequest("GET", "/api/protected", nil)
	req.Header.Set("X-PAYMENT", "valid_x402_payment")
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestMiddleware_PaymentSignatureHeader(t *testing.T) {
	handler := createTestHandler()
	wrapped := Middleware(handler, testConfig())

	// x402 v2 protocol uses PAYMENT-SIGNATURE header
	req := httptest.NewRequest("GET", "/api/protected", nil)
	req.Header.Set("PAYMENT-SIGNATURE", "valid_signature")
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestMiddleware_CustomVerifier(t *testing.T) {
	handler := createTestHandler()
	config := testConfig()
	config.PaymentVerifier = func(token string) (bool, error) {
		return token == "custom_valid", nil
	}
	wrapped := Middleware(handler, config)

	// Test with custom valid token
	req := httptest.NewRequest("GET", "/api/protected", nil)
	req.Header.Set("Authorization", "Bearer custom_valid")
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

// Helper functions

func createTestHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Success"))
	})
}

func testConfig() Config {
	return Config{
		PayTo:           "0x1234567890123456789012345678901234567890",
		PaymentEndpoint: "https://payment.example.com",
		AcceptedMethods: []string{"Bearer"},
		PricePerRequest: 100,
		Currency:        "USD",
		Network:         "base-sepolia",
		Scheme:          "exact",
		ExemptPaths:     []string{"/public"},
	}
}
