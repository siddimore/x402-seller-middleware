// Package x402 - AI Agent Optimized Mode
// Special handling for AI agents: budget awareness, auto-retry hints, cost estimation, streaming support.
package x402

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// AI Agent Detection Patterns
var aiAgentPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)openai`),
	regexp.MustCompile(`(?i)anthropic`),
	regexp.MustCompile(`(?i)claude`),
	regexp.MustCompile(`(?i)gpt-?[34]`),
	regexp.MustCompile(`(?i)langchain`),
	regexp.MustCompile(`(?i)autogpt`),
	regexp.MustCompile(`(?i)agent-?gpt`),
	regexp.MustCompile(`(?i)babyagi`),
	regexp.MustCompile(`(?i)superagi`),
	regexp.MustCompile(`(?i)crewai`),
	regexp.MustCompile(`(?i)autogen`),
	regexp.MustCompile(`(?i)llama-?index`),
	regexp.MustCompile(`(?i)semantic-?kernel`),
	regexp.MustCompile(`(?i)haystack`),
	regexp.MustCompile(`(?i)dspy`),
	regexp.MustCompile(`(?i)bot`),
	regexp.MustCompile(`(?i)crawler`),
	regexp.MustCompile(`(?i)spider`),
	regexp.MustCompile(`(?i)agent/`),
	regexp.MustCompile(`(?i)aiagent`),
	regexp.MustCompile(`(?i)mcp-client`), // Model Context Protocol clients
}

// isAIAgent detects if the request is from an AI agent
func isAIAgent(r *http.Request) bool {
	// Check explicit header
	if r.Header.Get("X-AI-Agent") == "true" {
		return true
	}

	// Check User-Agent
	ua := r.UserAgent()
	for _, pattern := range aiAgentPatterns {
		if pattern.MatchString(ua) {
			return true
		}
	}

	// Check for AI-specific headers
	if r.Header.Get("X-Agent-Budget") != "" {
		return true
	}
	if r.Header.Get("X-Agent-Task-ID") != "" {
		return true
	}

	return false
}

// AIAgentConfig configures AI agent handling
type AIAgentConfig struct {
	// EnableBudgetAwareness enables budget tracking for agents
	EnableBudgetAwareness bool

	// EnableCostEstimation adds cost estimates to responses
	EnableCostEstimation bool

	// EnableAutoRetryHints provides retry guidance in 402 responses
	EnableAutoRetryHints bool

	// EnableBatchPricing offers discounts for batch requests
	EnableBatchPricing bool

	// BatchDiscount percentage (e.g., 10 = 10% off for batches)
	BatchDiscount int

	// MinBatchSize minimum batch size for discount
	MinBatchSize int

	// EnableStreaming supports streaming responses with payment
	EnableStreaming bool

	// MaxBudgetPerRequest maximum cost a single request can incur
	MaxBudgetPerRequest int64

	// CostPerToken for token-based pricing (e.g., LLM wrappers)
	CostPerToken float64

	// Currency for pricing
	Currency string
}

// AIAgentHeaders contains headers specifically for AI agent communication
type AIAgentHeaders struct {
	// Input headers (from agent)
	AgentBudget     int64  `json:"agentBudget"`     // Max budget agent is willing to spend
	AgentTaskID     string `json:"agentTaskId"`     // Task identifier for tracking
	AgentBatchSize  int    `json:"agentBatchSize"`  // Number of items in batch request
	AgentPriority   string `json:"agentPriority"`   // "low", "normal", "high"
	AgentRetryCount int    `json:"agentRetryCount"` // Number of retries attempted

	// Output headers (to agent)
	EstimatedCost     int64  `json:"estimatedCost"`     // Estimated cost for this request
	ActualCost        int64  `json:"actualCost"`        // Actual cost after completion
	RemainingBudget   int64  `json:"remainingBudget"`   // Budget remaining after request
	RecommendedRetry  int    `json:"recommendedRetry"`  // Seconds to wait before retry
	BatchPricePerItem int64  `json:"batchPricePerItem"` // Price per item in batch
	StreamingSupport  bool   `json:"streamingSupported"`
	CostBreakdown     string `json:"costBreakdown"` // JSON breakdown of costs
}

// ParseAIAgentHeaders extracts AI agent headers from request
func ParseAIAgentHeaders(r *http.Request) AIAgentHeaders {
	headers := AIAgentHeaders{}

	if budget := r.Header.Get("X-Agent-Budget"); budget != "" {
		if b, err := strconv.ParseInt(budget, 10, 64); err == nil {
			headers.AgentBudget = b
		}
	}

	headers.AgentTaskID = r.Header.Get("X-Agent-Task-ID")

	if batch := r.Header.Get("X-Agent-Batch-Size"); batch != "" {
		if b, err := strconv.Atoi(batch); err == nil {
			headers.AgentBatchSize = b
		}
	}

	headers.AgentPriority = r.Header.Get("X-Agent-Priority")

	if retry := r.Header.Get("X-Agent-Retry-Count"); retry != "" {
		if rc, err := strconv.Atoi(retry); err == nil {
			headers.AgentRetryCount = rc
		}
	}

	return headers
}

// SetAIAgentResponseHeaders sets response headers for AI agents
func SetAIAgentResponseHeaders(w http.ResponseWriter, headers AIAgentHeaders) {
	if headers.EstimatedCost > 0 {
		w.Header().Set("X-Estimated-Cost", strconv.FormatInt(headers.EstimatedCost, 10))
	}
	if headers.ActualCost > 0 {
		w.Header().Set("X-Actual-Cost", strconv.FormatInt(headers.ActualCost, 10))
	}
	if headers.RemainingBudget > 0 {
		w.Header().Set("X-Remaining-Budget", strconv.FormatInt(headers.RemainingBudget, 10))
	}
	if headers.RecommendedRetry > 0 {
		w.Header().Set("X-Recommended-Retry", strconv.Itoa(headers.RecommendedRetry))
		w.Header().Set("Retry-After", strconv.Itoa(headers.RecommendedRetry))
	}
	if headers.BatchPricePerItem > 0 {
		w.Header().Set("X-Batch-Price-Per-Item", strconv.FormatInt(headers.BatchPricePerItem, 10))
	}
	if headers.StreamingSupport {
		w.Header().Set("X-Streaming-Supported", "true")
	}
	if headers.CostBreakdown != "" {
		w.Header().Set("X-Cost-Breakdown", headers.CostBreakdown)
	}
}

// AIAgentPaymentInfo extends PaymentRequiredResponse with agent-specific info
type AIAgentPaymentInfo struct {
	// Standard x402 info
	PaymentRequired PaymentRequiredResponse `json:"paymentRequired"`

	// AI Agent specific
	EstimatedCost        int64                `json:"estimatedCost"`
	CostPerRequest       int64                `json:"costPerRequest"`
	BatchAvailable       bool                 `json:"batchAvailable"`
	BatchDiscount        int                  `json:"batchDiscount,omitempty"` // Percentage
	MinBatchSize         int                  `json:"minBatchSize,omitempty"`
	BatchEndpoint        string               `json:"batchEndpoint,omitempty"`
	SessionAvailable     bool                 `json:"sessionAvailable"`
	SessionPricing       []SessionPricingTier `json:"sessionPricing,omitempty"`
	RetryStrategy        RetryStrategy        `json:"retryStrategy"`
	StreamingEndpoint    string               `json:"streamingEndpoint,omitempty"`
	BudgetRecommendation string               `json:"budgetRecommendation,omitempty"`
}

// RetryStrategy provides guidance for agents on retry behavior
type RetryStrategy struct {
	ShouldRetry       bool    `json:"shouldRetry"`
	RetryAfterSec     int     `json:"retryAfterSec"`
	MaxRetries        int     `json:"maxRetries"`
	BackoffMultiplier float64 `json:"backoffMultiplier"`
	Reason            string  `json:"reason,omitempty"`
}

// AIAgentMiddleware wraps the standard middleware with AI agent optimizations
func AIAgentMiddleware(next http.Handler, x402Config Config, agentConfig AIAgentConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Detect if this is an AI agent
		isAgent := isAIAgent(r)

		if isAgent {
			// Parse agent headers
			agentHeaders := ParseAIAgentHeaders(r)

			// Budget check
			if agentConfig.EnableBudgetAwareness && agentHeaders.AgentBudget > 0 {
				if x402Config.PricePerRequest > agentHeaders.AgentBudget {
					sendBudgetExceededResponse(w, x402Config, agentConfig, agentHeaders)
					return
				}
			}

			// Calculate batch pricing
			if agentConfig.EnableBatchPricing && agentHeaders.AgentBatchSize >= agentConfig.MinBatchSize {
				batchPrice := calculateBatchPrice(x402Config.PricePerRequest, agentHeaders.AgentBatchSize, agentConfig.BatchDiscount)
				responseHeaders := AIAgentHeaders{
					BatchPricePerItem: batchPrice,
					StreamingSupport:  agentConfig.EnableStreaming,
				}
				SetAIAgentResponseHeaders(w, responseHeaders)
			}

			// Add cost estimation headers
			if agentConfig.EnableCostEstimation {
				w.Header().Set("X-Estimated-Cost", strconv.FormatInt(x402Config.PricePerRequest, 10))
				w.Header().Set("X-Currency", agentConfig.Currency)
			}

			// Mark as AI agent request for downstream handlers
			r.Header.Set("X-AI-Agent-Detected", "true")
		}

		// Wrap response writer to capture for post-processing
		wrapped := &aiAgentResponseWriter{
			ResponseWriter: w,
			isAgent:        isAgent,
			config:         agentConfig,
			startTime:      time.Now(),
		}

		next.ServeHTTP(wrapped, r)

		// Add actual cost after processing
		if isAgent && agentConfig.EnableCostEstimation {
			wrapped.Header().Set("X-Actual-Cost", strconv.FormatInt(x402Config.PricePerRequest, 10))
			wrapped.Header().Set("X-Processing-Time-Ms", strconv.FormatInt(time.Since(wrapped.startTime).Milliseconds(), 10))
		}
	})
}

// aiAgentResponseWriter wraps response writer for AI agent handling
type aiAgentResponseWriter struct {
	http.ResponseWriter
	isAgent   bool
	config    AIAgentConfig
	startTime time.Time
	written   bool
}

func (w *aiAgentResponseWriter) WriteHeader(code int) {
	if !w.written && w.isAgent && code == http.StatusPaymentRequired {
		// Add retry hints for 402 responses
		if w.config.EnableAutoRetryHints {
			w.Header().Set("X-Recommended-Retry", "5")
			w.Header().Set("Retry-After", "5")
		}
	}
	w.written = true
	w.ResponseWriter.WriteHeader(code)
}

// sendBudgetExceededResponse sends a budget-specific 402 response for agents
func sendBudgetExceededResponse(w http.ResponseWriter, x402Config Config, agentConfig AIAgentConfig, headers AIAgentHeaders) {
	response := AIAgentPaymentInfo{
		EstimatedCost:  x402Config.PricePerRequest,
		CostPerRequest: x402Config.PricePerRequest,
		BatchAvailable: agentConfig.EnableBatchPricing,
		BatchDiscount:  agentConfig.BatchDiscount,
		MinBatchSize:   agentConfig.MinBatchSize,
		RetryStrategy: RetryStrategy{
			ShouldRetry:       false,
			Reason:            "Agent budget exceeded",
			MaxRetries:        0,
			BackoffMultiplier: 1.0,
		},
		BudgetRecommendation: formatBudgetRecommendation(x402Config.PricePerRequest, headers.AgentBudget),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Budget-Exceeded", "true")
	w.WriteHeader(http.StatusPaymentRequired)
	_ = json.NewEncoder(w).Encode(response)
}

// calculateBatchPrice calculates discounted price for batch requests
func calculateBatchPrice(basePrice int64, batchSize int, discountPercent int) int64 {
	if discountPercent <= 0 || discountPercent > 100 {
		return basePrice
	}
	discount := basePrice * int64(discountPercent) / 100
	return basePrice - discount
}

// formatBudgetRecommendation provides a helpful budget recommendation
func formatBudgetRecommendation(required, available int64) string {
	if available <= 0 {
		return "Set X-Agent-Budget header with your available budget"
	}
	deficit := required - available
	return "Increase budget by at least " + strconv.FormatInt(deficit, 10) + " units"
}

// BatchRequest represents a batch API request
type BatchRequest struct {
	Requests []BatchRequestItem `json:"requests"`
	TaskID   string             `json:"taskId,omitempty"`
}

// BatchRequestItem is a single request in a batch
type BatchRequestItem struct {
	ID      string            `json:"id"`
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    json.RawMessage   `json:"body,omitempty"`
}

// BatchResponse is the response to a batch request
type BatchResponse struct {
	Responses   []BatchResponseItem `json:"responses"`
	TotalCost   int64               `json:"totalCost"`
	TaskID      string              `json:"taskId,omitempty"`
	ProcessedAt time.Time           `json:"processedAt"`
}

// BatchResponseItem is a single response in a batch
type BatchResponseItem struct {
	ID         string            `json:"id"`
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       json.RawMessage   `json:"body,omitempty"`
	Error      string            `json:"error,omitempty"`
	Cost       int64             `json:"cost"`
}

// CostEstimate provides upfront cost estimation
type CostEstimate struct {
	Endpoint      string       `json:"endpoint"`
	Method        string       `json:"method"`
	EstimatedCost int64        `json:"estimatedCost"`
	Currency      string       `json:"currency"`
	Factors       []CostFactor `json:"factors,omitempty"`
	ValidUntil    time.Time    `json:"validUntil"`
}

// CostFactor breaks down cost components
type CostFactor struct {
	Name   string `json:"name"`
	Amount int64  `json:"amount"`
	Unit   string `json:"unit,omitempty"`
}

// CostEstimateHandler returns estimated costs for endpoints
func CostEstimateHandler(pricing map[string]int64, currency string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		endpoint := r.URL.Query().Get("endpoint")
		method := r.URL.Query().Get("method")
		if method == "" {
			method = "GET"
		}

		key := strings.ToUpper(method) + ":" + endpoint
		cost, ok := pricing[key]
		if !ok {
			// Try just endpoint
			cost, ok = pricing[endpoint]
			if !ok {
				cost = pricing["default"]
			}
		}

		estimate := CostEstimate{
			Endpoint:      endpoint,
			Method:        method,
			EstimatedCost: cost,
			Currency:      currency,
			ValidUntil:    time.Now().Add(5 * time.Minute),
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(estimate)
	}
}

// AgentWelcomeInfo provides onboarding information for AI agents
type AgentWelcomeInfo struct {
	Service          string              `json:"service"`
	Version          string              `json:"version"`
	X402Compliant    bool                `json:"x402Compliant"`
	AIAgentOptimized bool                `json:"aiAgentOptimized"`
	Features         []string            `json:"features"`
	Endpoints        []AgentEndpointInfo `json:"endpoints"`
	Authentication   AgentAuthInfo       `json:"authentication"`
	Pricing          AgentPricingInfo    `json:"pricing"`
	Documentation    string              `json:"documentation,omitempty"`
}

// AgentEndpointInfo describes an endpoint for agents
type AgentEndpointInfo struct {
	Path            string `json:"path"`
	Method          string `json:"method"`
	Description     string `json:"description"`
	Cost            int64  `json:"cost"`
	RequiresPayment bool   `json:"requiresPayment"`
}

// AgentAuthInfo describes authentication for agents
type AgentAuthInfo struct {
	Methods          []string `json:"methods"`
	SessionSupported bool     `json:"sessionSupported"`
	SessionEndpoint  string   `json:"sessionEndpoint,omitempty"`
	BatchSupported   bool     `json:"batchSupported"`
	BatchEndpoint    string   `json:"batchEndpoint,omitempty"`
}

// AgentPricingInfo describes pricing for agents
type AgentPricingInfo struct {
	Currency        string   `json:"currency"`
	BasePrice       int64    `json:"basePrice"`
	SessionDiscount int      `json:"sessionDiscount,omitempty"`
	BatchDiscount   int      `json:"batchDiscount,omitempty"`
	FreeEndpoints   []string `json:"freeEndpoints,omitempty"`
}

// AgentWelcomeHandler returns service info optimized for AI agents
func AgentWelcomeHandler(info AgentWelcomeInfo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-AI-Agent-Optimized", "true")
		_ = json.NewEncoder(w).Encode(info)
	}
}
