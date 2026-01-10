package x402

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSessionStore_CreateAndRetrieve(t *testing.T) {
	store := NewInMemorySessionStore()

	session := &Session{
		PayerAddress: "0x1234567890abcdef",
		ExpiresAt:    time.Now().Add(time.Hour),
		SessionType:  SessionTypeTime,
		AmountPaid:   1000,
		Currency:     "USDC",
	}

	err := store.CreateSession(session)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	if session.ID == "" {
		t.Error("Session ID was not generated")
	}

	// Retrieve
	retrieved, err := store.GetSession(session.ID)
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	if retrieved.PayerAddress != session.PayerAddress {
		t.Errorf("Payer address mismatch: got %s, want %s", retrieved.PayerAddress, session.PayerAddress)
	}
}

func TestSessionStore_ListByPayer(t *testing.T) {
	store := NewInMemorySessionStore()

	// Create sessions for different payers
	store.CreateSession(&Session{PayerAddress: "wallet_a", ExpiresAt: time.Now().Add(time.Hour)})
	store.CreateSession(&Session{PayerAddress: "wallet_a", ExpiresAt: time.Now().Add(time.Hour)})
	store.CreateSession(&Session{PayerAddress: "wallet_b", ExpiresAt: time.Now().Add(time.Hour)})

	sessions, err := store.ListSessionsByPayer("wallet_a")
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}

	if len(sessions) != 2 {
		t.Errorf("Expected 2 sessions for wallet_a, got %d", len(sessions))
	}
}

func TestSessionStore_CleanExpired(t *testing.T) {
	store := NewInMemorySessionStore()

	// Create expired session
	store.CreateSession(&Session{
		PayerAddress: "wallet_a",
		ExpiresAt:    time.Now().Add(-time.Hour), // Already expired
	})

	// Create valid session
	store.CreateSession(&Session{
		PayerAddress: "wallet_b",
		ExpiresAt:    time.Now().Add(time.Hour),
	})

	err := store.CleanExpired()
	if err != nil {
		t.Fatalf("Failed to clean expired: %v", err)
	}

	// Only wallet_b should remain
	sessionsA, _ := store.ListSessionsByPayer("wallet_a")
	sessionsB, _ := store.ListSessionsByPayer("wallet_b")

	if len(sessionsA) != 0 {
		t.Errorf("Expected 0 sessions for wallet_a after cleanup, got %d", len(sessionsA))
	}
	if len(sessionsB) != 1 {
		t.Errorf("Expected 1 session for wallet_b after cleanup, got %d", len(sessionsB))
	}
}

func TestValidateSession_Expired(t *testing.T) {
	session := &Session{
		Active:    true,
		ExpiresAt: time.Now().Add(-time.Hour),
	}

	err := validateSession(session, "/api/test")
	if err == nil {
		t.Error("Expected error for expired session")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("Expected expiration error, got: %v", err)
	}
}

func TestValidateSession_RequestLimit(t *testing.T) {
	session := &Session{
		Active:       true,
		ExpiresAt:    time.Now().Add(time.Hour),
		SessionType:  SessionTypeRequests,
		MaxRequests:  10,
		UsedRequests: 10, // Already used all requests
	}

	err := validateSession(session, "/api/test")
	if err == nil {
		t.Error("Expected error for exceeded request limit")
	}
	if !strings.Contains(err.Error(), "limit exceeded") {
		t.Errorf("Expected limit error, got: %v", err)
	}
}

func TestValidateSession_EndpointRestriction(t *testing.T) {
	session := &Session{
		Active:           true,
		ExpiresAt:        time.Now().Add(time.Hour),
		SessionType:      SessionTypeTime,
		AllowedEndpoints: []string{"/api/allowed", "/api/other/*"},
	}

	// Allowed endpoint
	if err := validateSession(session, "/api/allowed"); err != nil {
		t.Errorf("Expected /api/allowed to be valid: %v", err)
	}

	// Wildcard match
	if err := validateSession(session, "/api/other/foo"); err != nil {
		t.Errorf("Expected /api/other/foo to be valid: %v", err)
	}

	// Not allowed
	if err := validateSession(session, "/api/forbidden"); err == nil {
		t.Error("Expected /api/forbidden to be rejected")
	}
}

func TestSessionMiddleware_ValidSession(t *testing.T) {
	store := NewInMemorySessionStore()
	session := &Session{
		PayerAddress: "wallet_123",
		ExpiresAt:    time.Now().Add(time.Hour),
		SessionType:  SessionTypeRequests,
		MaxRequests:  100,
	}
	store.CreateSession(session)

	handler := SessionMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		SessionConfig{Store: store},
	)

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Session-ID", session.ID)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	// Check session remaining header
	if remaining := rr.Header().Get("X-Session-Remaining"); remaining == "" {
		t.Error("Expected X-Session-Remaining header")
	}
}

func TestSessionMiddleware_InvalidSession(t *testing.T) {
	store := NewInMemorySessionStore()

	handler := SessionMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		SessionConfig{Store: store},
	)

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Session-ID", "invalid_session_id")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 for invalid session, got %d", rr.Code)
	}
}

func TestSessionHandler_CreateSession(t *testing.T) {
	store := NewInMemorySessionStore()
	config := SessionConfig{
		Store:              store,
		DefaultDuration:    time.Hour,
		DefaultMaxRequests: 100,
		Currency:           "USDC",
	}

	handler := SessionHandler(store, config)

	body := `{"payerAddress": "0x123", "sessionType": "requests", "maxRequests": 50}`
	req := httptest.NewRequest("POST", "/sessions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", rr.Code)
	}

	var resp SessionCreateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.SessionID == "" {
		t.Error("Expected session ID in response")
	}
	if resp.MaxRequests != 50 {
		t.Errorf("Expected max requests 50, got %d", resp.MaxRequests)
	}
}

func TestEncodeDecodeSessionToken(t *testing.T) {
	original := &Session{
		ID:           "sess_test123",
		PayerAddress: "0xabcdef",
		SessionType:  SessionTypeTime,
		ExpiresAt:    time.Now().Add(time.Hour),
	}

	token := EncodeSessionToken(original)
	if token == "" {
		t.Fatal("Expected non-empty token")
	}

	decoded, err := DecodeSessionToken(token)
	if err != nil {
		t.Fatalf("Failed to decode token: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID mismatch: got %s, want %s", decoded.ID, original.ID)
	}
	if decoded.PayerAddress != original.PayerAddress {
		t.Errorf("Payer mismatch: got %s, want %s", decoded.PayerAddress, original.PayerAddress)
	}
}

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		path    string
		pattern string
		want    bool
	}{
		{"/api/test", "/api/test", true},
		{"/api/test", "/api/other", false},
		{"/api/test/foo", "/api/test/*", true},
		{"/api/test", "/api/test/*", true}, // Pattern prefix matches
		{"/anything", "*", true},
		{"/anything", "/*", true},
	}

	for _, tt := range tests {
		got := matchesPattern(tt.path, tt.pattern)
		if got != tt.want {
			t.Errorf("matchesPattern(%q, %q) = %v, want %v", tt.path, tt.pattern, got, tt.want)
		}
	}
}
