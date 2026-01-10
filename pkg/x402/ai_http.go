// Package x402 - AI HTTP Integration
// This file provides HTTP middleware and handlers for AI-first API servers:
// - AIFirstMiddleware: Adds AI-friendly headers, idempotency, pre-auth budget deduction
// - AIDiscoveryHandler: Endpoint for AI agents to discover API capabilities
// - AIBudgetHandler: Endpoint for managing pre-authorized budgets
// - OpenAI/Anthropic function calling schema generation
// - MCP (Model Context Protocol) tool definitions
// - Structured, machine-readable JSON responses
package x402

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// ============================================================================
// AI-FIRST RESPONSE FORMAT
// All responses are machine-readable JSON with consistent structure
// ============================================================================

// AIResponse is the standard response wrapper for AI agents
// Every response follows this format for predictable parsing
type AIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *AIError    `json:"error,omitempty"`
	Meta    AIMetadata  `json:"meta"`
}

// AIError provides structured, machine-parseable errors
type AIError struct {
	Code        string            `json:"code"`                  // Machine-readable error code
	Message     string            `json:"message"`               // Human-readable description
	Retryable   bool              `json:"retryable"`             // Can agent retry this?
	RetryAfter  int               `json:"retryAfter,omitempty"`  // Seconds to wait before retry
	Action      string            `json:"action,omitempty"`      // Suggested action: "pay", "retry", "abort", "reduce_scope"
	Details     map[string]string `json:"details,omitempty"`     // Additional context
	PaymentInfo *PaymentAction    `json:"paymentInfo,omitempty"` // If action is "pay"
}

// AIMetadata provides request context for agents
type AIMetadata struct {
	RequestID    string     `json:"requestId"`
	Timestamp    string     `json:"timestamp"`
	ProcessingMs int64      `json:"processingMs,omitempty"`
	Cost         *Cost      `json:"cost,omitempty"`
	RateLimit    *RateLimit `json:"rateLimit,omitempty"`
	Idempotent   bool       `json:"idempotent,omitempty"`
}

// Cost breakdown for the request
type Cost struct {
	Amount    int64      `json:"amount"`   // In smallest currency unit
	Currency  string     `json:"currency"` // e.g., "USDC"
	Breakdown []CostItem `json:"breakdown,omitempty"`
}

// CostItem is a single cost component
type CostItem struct {
	Name   string `json:"name"`
	Amount int64  `json:"amount"`
	Unit   string `json:"unit,omitempty"` // e.g., "per request", "per 1K tokens"
}

// RateLimit info for agents
type RateLimit struct {
	Remaining int   `json:"remaining"`
	Limit     int   `json:"limit"`
	ResetAt   int64 `json:"resetAt"` // Unix timestamp
}

// PaymentAction tells agent exactly how to pay
type PaymentAction struct {
	Required  bool   `json:"required"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	PayTo     string `json:"payTo"`     // Wallet address
	Network   string `json:"network"`   // e.g., "base", "base-sepolia"
	Asset     string `json:"asset"`     // Token contract address
	Endpoint  string `json:"endpoint"`  // Where to submit payment proof
	ExpiresAt int64  `json:"expiresAt"` // Payment must be made before this

	// Pre-authorization option
	PreAuthAvailable bool   `json:"preAuthAvailable,omitempty"`
	PreAuthEndpoint  string `json:"preAuthEndpoint,omitempty"`
	PreAuthMinBudget int64  `json:"preAuthMinBudget,omitempty"`
}

// Standard error codes for AI agents
const (
	ErrCodePaymentRequired     = "PAYMENT_REQUIRED"
	ErrCodeInsufficientBudget  = "INSUFFICIENT_BUDGET"
	ErrCodeInvalidPayment      = "INVALID_PAYMENT"
	ErrCodeExpiredPayment      = "EXPIRED_PAYMENT"
	ErrCodeRateLimited         = "RATE_LIMITED"
	ErrCodeInvalidRequest      = "INVALID_REQUEST"
	ErrCodeServerError         = "SERVER_ERROR"
	ErrCodeNotFound            = "NOT_FOUND"
	ErrCodeIdempotencyConflict = "IDEMPOTENCY_CONFLICT"
)

// ============================================================================
// OPENAI FUNCTION CALLING SCHEMA
// Auto-generate function definitions for OpenAI's function calling
// ============================================================================

// OpenAIFunction represents an OpenAI function definition
type OpenAIFunction struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Parameters  OpenAIFunctionParams `json:"parameters"`
	Cost        *FunctionCost        `json:"x-cost,omitempty"` // Extension for cost info
}

// OpenAIFunctionParams defines the function parameters
type OpenAIFunctionParams struct {
	Type       string                       `json:"type"` // Always "object"
	Properties map[string]OpenAIPropertyDef `json:"properties"`
	Required   []string                     `json:"required,omitempty"`
}

// OpenAIPropertyDef defines a single parameter
type OpenAIPropertyDef struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Default     any      `json:"default,omitempty"`
}

// FunctionCost extends OpenAI schema with pricing
type FunctionCost struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	Unit     string `json:"unit"` // "per_call", "per_token", "per_kb"
}

// APIEndpoint defines a single API endpoint with full metadata
type APIEndpoint struct {
	Path        string             `json:"path"`
	Method      string             `json:"method"`
	Name        string             `json:"name"` // Function name for AI
	Description string             `json:"description"`
	Parameters  []EndpointParam    `json:"parameters,omitempty"`
	Cost        int64              `json:"cost"`
	Currency    string             `json:"currency"`
	CostUnit    string             `json:"costUnit"` // "per_call", "per_token"
	Tags        []string           `json:"tags,omitempty"`
	RateLimit   *EndpointRateLimit `json:"rateLimit,omitempty"`
}

// EndpointParam defines an API parameter
type EndpointParam struct {
	Name        string `json:"name"`
	In          string `json:"in"` // "path", "query", "body", "header"
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
	Default     any    `json:"default,omitempty"`
}

// EndpointRateLimit defines rate limits for an endpoint
type EndpointRateLimit struct {
	RequestsPerMinute int `json:"requestsPerMinute"`
	RequestsPerDay    int `json:"requestsPerDay,omitempty"`
}

// GenerateOpenAIFunctions converts API endpoints to OpenAI function definitions
func GenerateOpenAIFunctions(endpoints []APIEndpoint) []OpenAIFunction {
	functions := make([]OpenAIFunction, 0, len(endpoints))

	for _, ep := range endpoints {
		props := make(map[string]OpenAIPropertyDef)
		required := []string{}

		for _, param := range ep.Parameters {
			props[param.Name] = OpenAIPropertyDef{
				Type:        param.Type,
				Description: param.Description,
			}
			if param.Required {
				required = append(required, param.Name)
			}
		}

		fn := OpenAIFunction{
			Name:        ep.Name,
			Description: fmt.Sprintf("%s (Cost: %d %s %s)", ep.Description, ep.Cost, ep.Currency, ep.CostUnit),
			Parameters: OpenAIFunctionParams{
				Type:       "object",
				Properties: props,
				Required:   required,
			},
			Cost: &FunctionCost{
				Amount:   ep.Cost,
				Currency: ep.Currency,
				Unit:     ep.CostUnit,
			},
		}
		functions = append(functions, fn)
	}

	return functions
}

// ============================================================================
// MCP (MODEL CONTEXT PROTOCOL) SUPPORT
// Native tool definitions for Claude and MCP-compatible clients
// ============================================================================

// MCPTool represents an MCP tool definition
type MCPTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema MCPInputSchema `json:"inputSchema"`
	Cost        *MCPCost       `json:"cost,omitempty"` // Extension
}

// MCPInputSchema defines the tool input
type MCPInputSchema struct {
	Type       string                 `json:"type"` // Always "object"
	Properties map[string]MCPProperty `json:"properties"`
	Required   []string               `json:"required,omitempty"`
}

// MCPProperty defines a single input property
type MCPProperty struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// MCPCost extends MCP schema with pricing
type MCPCost struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	Unit     string `json:"unit"`
}

// MCPToolsResponse is the response for listing available tools
type MCPToolsResponse struct {
	Tools       []MCPTool      `json:"tools"`
	PaymentInfo MCPPaymentInfo `json:"paymentInfo"`
}

// MCPPaymentInfo provides payment context for MCP clients
type MCPPaymentInfo struct {
	Protocol        string `json:"protocol"` // "x402"
	Network         string `json:"network"`
	Currency        string `json:"currency"`
	PayTo           string `json:"payTo"`
	PreAuthEndpoint string `json:"preAuthEndpoint,omitempty"`
	SessionEndpoint string `json:"sessionEndpoint,omitempty"`
}

// GenerateMCPTools converts API endpoints to MCP tool definitions
func GenerateMCPTools(endpoints []APIEndpoint) []MCPTool {
	tools := make([]MCPTool, 0, len(endpoints))

	for _, ep := range endpoints {
		props := make(map[string]MCPProperty)
		required := []string{}

		for _, param := range ep.Parameters {
			props[param.Name] = MCPProperty{
				Type:        param.Type,
				Description: param.Description,
			}
			if param.Required {
				required = append(required, param.Name)
			}
		}

		tool := MCPTool{
			Name:        ep.Name,
			Description: ep.Description,
			InputSchema: MCPInputSchema{
				Type:       "object",
				Properties: props,
				Required:   required,
			},
			Cost: &MCPCost{
				Amount:   ep.Cost,
				Currency: ep.Currency,
				Unit:     ep.CostUnit,
			},
		}
		tools = append(tools, tool)
	}

	return tools
}

// ============================================================================
// PRE-AUTHORIZED BUDGET / AUTO-PAY
// Agents deposit a budget upfront, middleware auto-deducts per request
// ============================================================================

// PreAuthBudget represents a pre-authorized spending budget
type PreAuthBudget struct {
	ID            string            `json:"id"`
	AgentID       string            `json:"agentId"`       // Identifier for the agent
	WalletAddress string            `json:"walletAddress"` // Payer wallet
	TotalBudget   int64             `json:"totalBudget"`   // Initial budget
	Remaining     int64             `json:"remaining"`     // Current remaining
	Currency      string            `json:"currency"`
	CreatedAt     time.Time         `json:"createdAt"`
	ExpiresAt     time.Time         `json:"expiresAt"`
	Metadata      map[string]string `json:"metadata,omitempty"`

	// Usage tracking
	TotalSpent   int64 `json:"totalSpent"`
	RequestCount int64 `json:"requestCount"`
}

// PreAuthStore interface for budget storage
type PreAuthStore interface {
	Create(budget *PreAuthBudget) error
	Get(id string) (*PreAuthBudget, error)
	GetByAgentID(agentID string) (*PreAuthBudget, error)
	Deduct(id string, amount int64) error
	Refund(id string, amount int64) error
	Delete(id string) error
}

// InMemoryPreAuthStore is a simple in-memory implementation
type InMemoryPreAuthStore struct {
	mu      sync.RWMutex
	budgets map[string]*PreAuthBudget
	byAgent map[string]string // agentID -> budgetID
}

// NewInMemoryPreAuthStore creates a new pre-auth store
func NewInMemoryPreAuthStore() *InMemoryPreAuthStore {
	return &InMemoryPreAuthStore{
		budgets: make(map[string]*PreAuthBudget),
		byAgent: make(map[string]string),
	}
}

func (s *InMemoryPreAuthStore) Create(budget *PreAuthBudget) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if budget.ID == "" {
		budget.ID = generateBudgetID()
	}
	budget.CreatedAt = time.Now()
	budget.Remaining = budget.TotalBudget

	s.budgets[budget.ID] = budget
	if budget.AgentID != "" {
		s.byAgent[budget.AgentID] = budget.ID
	}
	return nil
}

func (s *InMemoryPreAuthStore) Get(id string) (*PreAuthBudget, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	budget, ok := s.budgets[id]
	if !ok {
		return nil, fmt.Errorf("budget not found")
	}
	return budget, nil
}

func (s *InMemoryPreAuthStore) GetByAgentID(agentID string) (*PreAuthBudget, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	budgetID, ok := s.byAgent[agentID]
	if !ok {
		return nil, fmt.Errorf("no budget for agent")
	}
	return s.budgets[budgetID], nil
}

func (s *InMemoryPreAuthStore) Deduct(id string, amount int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	budget, ok := s.budgets[id]
	if !ok {
		return fmt.Errorf("budget not found")
	}
	if budget.Remaining < amount {
		return fmt.Errorf("insufficient budget")
	}
	budget.Remaining -= amount
	budget.TotalSpent += amount
	budget.RequestCount++
	return nil
}

func (s *InMemoryPreAuthStore) Refund(id string, amount int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	budget, ok := s.budgets[id]
	if !ok {
		return fmt.Errorf("budget not found")
	}
	budget.Remaining += amount
	budget.TotalSpent -= amount
	return nil
}

func (s *InMemoryPreAuthStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	budget, ok := s.budgets[id]
	if ok && budget.AgentID != "" {
		delete(s.byAgent, budget.AgentID)
	}
	delete(s.budgets, id)
	return nil
}

func generateBudgetID() string {
	b := make([]byte, 16)
	return "budget_" + hex.EncodeToString(b)
}

// ============================================================================
// IDEMPOTENCY
// Safe retries for AI agents with idempotency keys
// ============================================================================

// IdempotencyStore tracks request idempotency
type IdempotencyStore interface {
	Get(key string) (*IdempotencyRecord, error)
	Set(key string, record *IdempotencyRecord) error
	Delete(key string) error
}

// IdempotencyRecord stores a request result
type IdempotencyRecord struct {
	Key        string            `json:"key"`
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body"`
	CreatedAt  time.Time         `json:"createdAt"`
	ExpiresAt  time.Time         `json:"expiresAt"`
}

// InMemoryIdempotencyStore is a simple in-memory implementation
type InMemoryIdempotencyStore struct {
	mu      sync.RWMutex
	records map[string]*IdempotencyRecord
}

// NewInMemoryIdempotencyStore creates a new idempotency store
func NewInMemoryIdempotencyStore() *InMemoryIdempotencyStore {
	return &InMemoryIdempotencyStore{
		records: make(map[string]*IdempotencyRecord),
	}
}

func (s *InMemoryIdempotencyStore) Get(key string) (*IdempotencyRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record, ok := s.records[key]
	if !ok {
		return nil, nil
	}
	if time.Now().After(record.ExpiresAt) {
		return nil, nil
	}
	return record, nil
}

func (s *InMemoryIdempotencyStore) Set(key string, record *IdempotencyRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record.Key = key
	record.CreatedAt = time.Now()
	if record.ExpiresAt.IsZero() {
		record.ExpiresAt = time.Now().Add(24 * time.Hour)
	}
	s.records[key] = record
	return nil
}

func (s *InMemoryIdempotencyStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, key)
	return nil
}

// ============================================================================
// AI-FIRST MIDDLEWARE
// Combines all AI agent optimizations into a single middleware
// ============================================================================

// AIFirstConfig configures the AI-first middleware
type AIFirstConfig struct {
	// API definition
	Endpoints []APIEndpoint

	// Payment config
	PayTo    string
	Network  string
	Currency string
	Asset    string

	// Stores
	PreAuthStore     PreAuthStore
	IdempotencyStore IdempotencyStore

	// Feature flags
	EnablePreAuth     bool
	EnableIdempotency bool
	IdempotencyTTL    time.Duration

	// Pricing
	DefaultCost int64
}

// AIFirstMiddleware provides AI-optimized request handling
func AIFirstMiddleware(next http.Handler, config AIFirstConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestID := generateRequestID(r)

		// Set AI-friendly headers
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("X-AI-Optimized", "true")

		// Check idempotency key
		if config.EnableIdempotency && config.IdempotencyStore != nil {
			if idempKey := r.Header.Get("Idempotency-Key"); idempKey != "" {
				if record, _ := config.IdempotencyStore.Get(idempKey); record != nil {
					// Return cached response
					for k, v := range record.Headers {
						w.Header().Set(k, v)
					}
					w.Header().Set("X-Idempotent-Replay", "true")
					w.WriteHeader(record.StatusCode)
					_, _ = w.Write(record.Body)
					return
				}
			}
		}

		// Check pre-authorized budget
		if config.EnablePreAuth && config.PreAuthStore != nil {
			agentID := r.Header.Get("X-Agent-ID")
			if agentID != "" {
				budget, err := config.PreAuthStore.GetByAgentID(agentID)
				if err == nil && budget != nil {
					cost := getCostForPath(r.URL.Path, r.Method, config.Endpoints, config.DefaultCost)

					if budget.Remaining < cost {
						sendAIError(w, requestID, start, AIError{
							Code:      ErrCodeInsufficientBudget,
							Message:   "Pre-authorized budget exhausted",
							Retryable: false,
							Action:    "pay",
							Details: map[string]string{
								"remaining": fmt.Sprintf("%d", budget.Remaining),
								"required":  fmt.Sprintf("%d", cost),
								"budgetId":  budget.ID,
							},
							PaymentInfo: &PaymentAction{
								Required:         true,
								Amount:           cost - budget.Remaining,
								Currency:         config.Currency,
								PayTo:            config.PayTo,
								Network:          config.Network,
								PreAuthAvailable: true,
								PreAuthEndpoint:  "/ai/budget",
							},
						})
						return
					}

					// Deduct from budget
					if err := config.PreAuthStore.Deduct(budget.ID, cost); err != nil {
						sendAIError(w, requestID, start, AIError{
							Code:       ErrCodeServerError,
							Message:    "Failed to deduct from budget",
							Retryable:  true,
							RetryAfter: 1,
							Action:     "retry",
						})
						return
					}

					// Add budget info to headers (budget.Remaining is already updated by Deduct)
					w.Header().Set("X-Budget-Remaining", fmt.Sprintf("%d", budget.Remaining))
					w.Header().Set("X-Budget-Deducted", fmt.Sprintf("%d", cost))

					// Mark as paid
					r.Header.Set("X-Payment-Verified", "true")
				}
			}
		}

		// Wrap response for idempotency caching
		wrapped := &aiResponseRecorder{
			ResponseWriter: w,
			statusCode:     200,
			body:           []byte{},
		}

		next.ServeHTTP(wrapped, r)

		// Store idempotency record
		if config.EnableIdempotency && config.IdempotencyStore != nil {
			if idempKey := r.Header.Get("Idempotency-Key"); idempKey != "" {
				headers := make(map[string]string)
				for k := range wrapped.Header() {
					headers[k] = wrapped.Header().Get(k)
				}
				ttl := config.IdempotencyTTL
				if ttl == 0 {
					ttl = 24 * time.Hour
				}
				_ = config.IdempotencyStore.Set(idempKey, &IdempotencyRecord{
					StatusCode: wrapped.statusCode,
					Headers:    headers,
					Body:       wrapped.body,
					ExpiresAt:  time.Now().Add(ttl),
				})
			}
		}
	})
}

type aiResponseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       []byte
}

func (r *aiResponseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *aiResponseRecorder) Write(b []byte) (int, error) {
	r.body = append(r.body, b...)
	return r.ResponseWriter.Write(b)
}

func generateRequestID(r *http.Request) string {
	h := sha256.New()
	h.Write([]byte(time.Now().String()))
	h.Write([]byte(r.URL.Path))
	return "req_" + hex.EncodeToString(h.Sum(nil))[:16]
}

func getCostForPath(path, method string, endpoints []APIEndpoint, defaultCost int64) int64 {
	for _, ep := range endpoints {
		if ep.Path == path && ep.Method == method {
			return ep.Cost
		}
	}
	return defaultCost
}

func sendAIError(w http.ResponseWriter, requestID string, start time.Time, err AIError) {
	response := AIResponse{
		Success: false,
		Error:   &err,
		Meta: AIMetadata{
			RequestID:    requestID,
			Timestamp:    time.Now().Format(time.RFC3339),
			ProcessingMs: time.Since(start).Milliseconds(),
		},
	}

	w.Header().Set("Content-Type", "application/json")

	switch err.Code {
	case ErrCodePaymentRequired, ErrCodeInsufficientBudget:
		w.WriteHeader(http.StatusPaymentRequired)
	case ErrCodeRateLimited:
		w.WriteHeader(http.StatusTooManyRequests)
	case ErrCodeNotFound:
		w.WriteHeader(http.StatusNotFound)
	case ErrCodeInvalidRequest:
		w.WriteHeader(http.StatusBadRequest)
	default:
		w.WriteHeader(http.StatusInternalServerError)
	}

	_ = json.NewEncoder(w).Encode(response)
}

// SendAISuccess sends a successful AI response
func SendAISuccess(w http.ResponseWriter, requestID string, start time.Time, data interface{}, cost *Cost) {
	response := AIResponse{
		Success: true,
		Data:    data,
		Meta: AIMetadata{
			RequestID:    requestID,
			Timestamp:    time.Now().Format(time.RFC3339),
			ProcessingMs: time.Since(start).Milliseconds(),
			Cost:         cost,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

// ============================================================================
// AI DISCOVERY HANDLERS
// Endpoints for AI agents to discover API capabilities
// ============================================================================

// AIDiscoveryHandler returns comprehensive API info for AI agents
func AIDiscoveryHandler(config AIFirstConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		format := r.URL.Query().Get("format")

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-AI-Optimized", "true")

		switch format {
		case "openai":
			// Return OpenAI function calling format
			functions := GenerateOpenAIFunctions(config.Endpoints)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"functions": functions,
				"payment": map[string]interface{}{
					"protocol":        "x402",
					"network":         config.Network,
					"currency":        config.Currency,
					"payTo":           config.PayTo,
					"preAuth":         config.EnablePreAuth,
					"preAuthEndpoint": "/ai/budget",
				},
			})

		case "mcp":
			// Return MCP tool format
			tools := GenerateMCPTools(config.Endpoints)
			response := MCPToolsResponse{
				Tools: tools,
				PaymentInfo: MCPPaymentInfo{
					Protocol:        "x402",
					Network:         config.Network,
					Currency:        config.Currency,
					PayTo:           config.PayTo,
					PreAuthEndpoint: "/ai/budget",
					SessionEndpoint: "/sessions",
				},
			}
			_ = json.NewEncoder(w).Encode(response)

		default:
			// Return full discovery info
			discovery := map[string]interface{}{
				"name":    "AI-First x402 API",
				"version": "1.0",
				"protocol": map[string]interface{}{
					"x402Version":          1,
					"aiOptimized":          true,
					"preAuthSupported":     config.EnablePreAuth,
					"idempotencySupported": config.EnableIdempotency,
				},
				"payment": map[string]interface{}{
					"network":  config.Network,
					"currency": config.Currency,
					"payTo":    config.PayTo,
					"asset":    config.Asset,
				},
				"endpoints": config.Endpoints,
				"schemas": map[string]interface{}{
					"openai": "/ai/discover?format=openai",
					"mcp":    "/ai/discover?format=mcp",
				},
				"features": []string{
					"pre-authorized-budgets",
					"idempotent-requests",
					"structured-errors",
					"cost-estimation",
					"session-payments",
					"batch-requests",
				},
			}
			_ = json.NewEncoder(w).Encode(discovery)
		}
	}
}

// AIBudgetHandler manages pre-authorized budgets
func AIBudgetHandler(store PreAuthStore, config AIFirstConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case http.MethodPost:
			// Create new budget
			var req struct {
				AgentID       string `json:"agentId"`
				WalletAddress string `json:"walletAddress"`
				Budget        int64  `json:"budget"`
				PaymentProof  string `json:"paymentProof"` // x402 payment proof
				ExpiresIn     string `json:"expiresIn"`    // e.g., "24h", "7d"
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
				return
			}

			// TODO: Verify payment proof with facilitator

			expiry := 24 * time.Hour
			if req.ExpiresIn != "" {
				if d, err := time.ParseDuration(req.ExpiresIn); err == nil {
					expiry = d
				}
			}

			budget := &PreAuthBudget{
				AgentID:       req.AgentID,
				WalletAddress: req.WalletAddress,
				TotalBudget:   req.Budget,
				Currency:      config.Currency,
				ExpiresAt:     time.Now().Add(expiry),
			}

			if err := store.Create(budget); err != nil {
				http.Error(w, `{"error":"failed to create budget"}`, http.StatusInternalServerError)
				return
			}

			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(budget)

		case http.MethodGet:
			// Get budget status
			agentID := r.URL.Query().Get("agentId")
			budgetID := r.URL.Query().Get("id")

			var budget *PreAuthBudget
			var err error

			if budgetID != "" {
				budget, err = store.Get(budgetID)
			} else if agentID != "" {
				budget, err = store.GetByAgentID(agentID)
			} else {
				http.Error(w, `{"error":"agentId or id required"}`, http.StatusBadRequest)
				return
			}

			if err != nil {
				http.Error(w, `{"error":"budget not found"}`, http.StatusNotFound)
				return
			}

			_ = json.NewEncoder(w).Encode(budget)

		case http.MethodDelete:
			// Close budget (refund remaining)
			budgetID := r.URL.Query().Get("id")
			if budgetID == "" {
				http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
				return
			}

			budget, err := store.Get(budgetID)
			if err != nil {
				http.Error(w, `{"error":"budget not found"}`, http.StatusNotFound)
				return
			}

			// TODO: Process refund for remaining balance

			if err := store.Delete(budgetID); err != nil {
				http.Error(w, `{"error":"failed to delete budget"}`, http.StatusInternalServerError)
				return
			}

			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"deleted":    true,
				"refunded":   budget.Remaining,
				"totalSpent": budget.TotalSpent,
			})

		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	}
}
