// Package x402 - Session & Subscription Payments
// Pay once, use many times. Supports time-based and request-count-based sessions.
package x402

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"
)

// SessionType defines the type of session
type SessionType string

const (
	// SessionTypeTime is a time-based session (e.g., 1 hour access)
	SessionTypeTime SessionType = "time"
	// SessionTypeRequests is a request-count-based session (e.g., 100 requests)
	SessionTypeRequests SessionType = "requests"
	// SessionTypeUnlimited is an unlimited session until expiry
	SessionTypeUnlimited SessionType = "unlimited"
)

// Session represents a payment session
type Session struct {
	ID               string            `json:"id"`
	PayerAddress     string            `json:"payerAddress"`
	CreatedAt        time.Time         `json:"createdAt"`
	ExpiresAt        time.Time         `json:"expiresAt"`
	SessionType      SessionType       `json:"sessionType"`
	MaxRequests      int64             `json:"maxRequests,omitempty"` // For request-based sessions
	UsedRequests     int64             `json:"usedRequests"`
	AmountPaid       int64             `json:"amountPaid"` // Total amount paid for session
	Currency         string            `json:"currency"`
	AllowedEndpoints []string          `json:"allowedEndpoints,omitempty"` // Empty = all endpoints
	Metadata         map[string]string `json:"metadata,omitempty"`
	Active           bool              `json:"active"`
}

// SessionStore interface for session storage
type SessionStore interface {
	CreateSession(session *Session) error
	GetSession(id string) (*Session, error)
	UpdateSession(session *Session) error
	DeleteSession(id string) error
	ListSessionsByPayer(payerAddress string) ([]*Session, error)
	CleanExpired() error
}

// SessionConfig configures session-based payments
type SessionConfig struct {
	Store              SessionStore
	DefaultDuration    time.Duration // Default session duration
	DefaultMaxRequests int64         // Default max requests for request-based sessions
	PricePerHour       int64         // Price for time-based sessions
	PricePerRequest    int64         // Price per request in session
	Currency           string
	AllowedEndpoints   []string // Endpoints allowed for session access
}

// SessionPricingTier defines pricing tiers for sessions
type SessionPricingTier struct {
	Name        string        `json:"name"`
	Duration    time.Duration `json:"duration"`
	MaxRequests int64         `json:"maxRequests"`
	Price       int64         `json:"price"`
	Currency    string        `json:"currency"`
	SessionType SessionType   `json:"sessionType"`
}

// InMemorySessionStore is a simple in-memory session store
type InMemorySessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewInMemorySessionStore creates a new in-memory session store
func NewInMemorySessionStore() *InMemorySessionStore {
	return &InMemorySessionStore{
		sessions: make(map[string]*Session),
	}
}

// CreateSession stores a new session
func (s *InMemorySessionStore) CreateSession(session *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if session.ID == "" {
		session.ID = generateSessionID()
	}
	session.CreatedAt = time.Now()
	session.Active = true
	s.sessions[session.ID] = session
	return nil
}

// GetSession retrieves a session by ID
func (s *InMemorySessionStore) GetSession(id string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[id]
	if !ok {
		return nil, errors.New("session not found")
	}
	return session, nil
}

// UpdateSession updates an existing session
func (s *InMemorySessionStore) UpdateSession(session *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[session.ID]; !ok {
		return errors.New("session not found")
	}
	s.sessions[session.ID] = session
	return nil
}

// DeleteSession removes a session
func (s *InMemorySessionStore) DeleteSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, id)
	return nil
}

// ListSessionsByPayer lists all sessions for a payer
func (s *InMemorySessionStore) ListSessionsByPayer(payerAddress string) ([]*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Session
	for _, session := range s.sessions {
		if session.PayerAddress == payerAddress {
			result = append(result, session)
		}
	}
	return result, nil
}

// CleanExpired removes expired sessions
func (s *InMemorySessionStore) CleanExpired() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for id, session := range s.sessions {
		if session.ExpiresAt.Before(now) {
			delete(s.sessions, id)
		}
	}
	return nil
}

// generateSessionID creates a unique session ID
func generateSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "sess_" + hex.EncodeToString(b)
}

// SessionMiddleware validates session-based access
func SessionMiddleware(next http.Handler, config SessionConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.Header.Get("X-Session-ID")
		if sessionID == "" {
			// No session, try other payment methods
			next.ServeHTTP(w, r)
			return
		}

		session, err := config.Store.GetSession(sessionID)
		if err != nil {
			sendSessionError(w, "invalid_session", "Session not found or invalid")
			return
		}

		// Validate session
		if err := validateSession(session, r.URL.Path); err != nil {
			sendSessionError(w, "session_error", err.Error())
			return
		}

		// Increment usage for request-based sessions
		if session.SessionType == SessionTypeRequests {
			session.UsedRequests++
			_ = config.Store.UpdateSession(session)
		}

		// Add session info to response headers
		w.Header().Set("X-Session-Remaining", formatSessionRemaining(session))
		w.Header().Set("X-Session-Expires", session.ExpiresAt.Format(time.RFC3339))

		next.ServeHTTP(w, r)
	})
}

// validateSession checks if a session is valid for the request
func validateSession(session *Session, path string) error {
	if !session.Active {
		return errors.New("session is inactive")
	}

	if time.Now().After(session.ExpiresAt) {
		return errors.New("session has expired")
	}

	if session.SessionType == SessionTypeRequests && session.UsedRequests >= session.MaxRequests {
		return errors.New("session request limit exceeded")
	}

	// Check endpoint restrictions
	if len(session.AllowedEndpoints) > 0 {
		allowed := false
		for _, endpoint := range session.AllowedEndpoints {
			if path == endpoint || matchesPattern(path, endpoint) {
				allowed = true
				break
			}
		}
		if !allowed {
			return errors.New("endpoint not allowed for this session")
		}
	}

	return nil
}

// matchesPattern checks if a path matches a pattern (simple wildcard support)
func matchesPattern(path, pattern string) bool {
	if pattern == "*" || pattern == "/*" {
		return true
	}
	// Simple prefix matching for patterns ending in /*
	if len(pattern) > 2 && pattern[len(pattern)-2:] == "/*" {
		prefix := pattern[:len(pattern)-2]
		return len(path) >= len(prefix) && path[:len(prefix)] == prefix
	}
	return path == pattern
}

// formatSessionRemaining returns remaining usage as a string
func formatSessionRemaining(session *Session) string {
	switch session.SessionType {
	case SessionTypeRequests:
		remaining := session.MaxRequests - session.UsedRequests
		return formatInt64(remaining) + " requests"
	case SessionTypeTime:
		remaining := time.Until(session.ExpiresAt)
		return remaining.Round(time.Second).String()
	default:
		return "unlimited"
	}
}

func formatInt64(n int64) string {
	if n < 0 {
		return "0"
	}
	// Simple int64 to string without importing strconv
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// sendSessionError sends a session-specific error response
func sendSessionError(w http.ResponseWriter, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   code,
		"message": message,
	})
}

// SessionCreateRequest is the request body for creating a session
type SessionCreateRequest struct {
	PayerAddress string            `json:"payerAddress"`
	PaymentProof string            `json:"paymentProof"` // x402 payment proof
	SessionType  SessionType       `json:"sessionType"`
	Duration     string            `json:"duration,omitempty"` // e.g., "1h", "24h"
	MaxRequests  int64             `json:"maxRequests,omitempty"`
	Endpoints    []string          `json:"endpoints,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// SessionCreateResponse is returned when creating a session
type SessionCreateResponse struct {
	SessionID         string      `json:"sessionId"`
	ExpiresAt         time.Time   `json:"expiresAt"`
	SessionType       SessionType `json:"sessionType"`
	MaxRequests       int64       `json:"maxRequests,omitempty"`
	RemainingRequests int64       `json:"remainingRequests,omitempty"`
}

// SessionHandler returns an HTTP handler for session management
func SessionHandler(store SessionStore, config SessionConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			handleCreateSession(w, r, store, config)
		case http.MethodGet:
			handleGetSession(w, r, store)
		case http.MethodDelete:
			handleDeleteSession(w, r, store)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleCreateSession(w http.ResponseWriter, r *http.Request, store SessionStore, config SessionConfig) {
	var req SessionCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// TODO: Verify payment proof with facilitator

	// Parse duration
	duration := config.DefaultDuration
	if req.Duration != "" {
		if d, err := time.ParseDuration(req.Duration); err == nil {
			duration = d
		}
	}

	maxRequests := config.DefaultMaxRequests
	if req.MaxRequests > 0 {
		maxRequests = req.MaxRequests
	}

	session := &Session{
		PayerAddress:     req.PayerAddress,
		ExpiresAt:        time.Now().Add(duration),
		SessionType:      req.SessionType,
		MaxRequests:      maxRequests,
		Currency:         config.Currency,
		AllowedEndpoints: req.Endpoints,
		Metadata:         req.Metadata,
	}

	if err := store.CreateSession(session); err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	resp := SessionCreateResponse{
		SessionID:   session.ID,
		ExpiresAt:   session.ExpiresAt,
		SessionType: session.SessionType,
	}
	if session.SessionType == SessionTypeRequests {
		resp.MaxRequests = session.MaxRequests
		resp.RemainingRequests = session.MaxRequests
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

func handleGetSession(w http.ResponseWriter, r *http.Request, store SessionStore) {
	sessionID := r.URL.Query().Get("id")
	if sessionID == "" {
		sessionID = r.Header.Get("X-Session-ID")
	}
	if sessionID == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	session, err := store.GetSession(sessionID)
	if err != nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(session)
}

func handleDeleteSession(w http.ResponseWriter, r *http.Request, store SessionStore) {
	sessionID := r.URL.Query().Get("id")
	if sessionID == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	if err := store.DeleteSession(sessionID); err != nil {
		http.Error(w, "Failed to delete session", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// PricingHandler returns available session pricing tiers
func PricingHandler(tiers []SessionPricingTier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"tiers": tiers,
		})
	}
}

// SubscriptionInfo is returned in x402 responses for session-aware clients
type SubscriptionInfo struct {
	Available       bool                 `json:"available"`
	Tiers           []SessionPricingTier `json:"tiers,omitempty"`
	SessionEndpoint string               `json:"sessionEndpoint,omitempty"`
}

// AddSubscriptionInfo adds subscription info to PaymentRequirements
func AddSubscriptionInfo(req *PaymentRequirements, info SubscriptionInfo) {
	if req.Extra == nil {
		req.Extra = make(map[string]interface{})
	}
	infoBytes, _ := json.Marshal(info)
	var infoMap map[string]interface{}
	_ = json.Unmarshal(infoBytes, &infoMap)
	req.Extra["subscription"] = infoMap
}

// EncodeSessionToken encodes session info for the X-Session-Token header
func EncodeSessionToken(session *Session) string {
	data, _ := json.Marshal(session)
	return base64.StdEncoding.EncodeToString(data)
}

// DecodeSessionToken decodes a session token
func DecodeSessionToken(token string) (*Session, error) {
	data, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, err
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}
	return &session, nil
}
