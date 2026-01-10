package x402

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsAIAgent_ExplicitHeader(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-AI-Agent", "true")

	if !isAIAgent(req) {
		t.Error("Expected X-AI-Agent header to be detected")
	}
}

func TestIsAIAgent_UserAgentPatterns(t *testing.T) {
	tests := []struct {
		userAgent string
		expected  bool
	}{
		{"OpenAI-API/1.0", true},
		{"Anthropic-Claude/1.0", true},
		{"LangChain/0.1", true},
		{"AutoGPT/1.0", true},
		{"AgentGPT/2.0", true},
		{"BabyAGI/1.0", true},
		{"CrewAI/1.0", true},
		{"MCP-Client/1.0", true},
		{"Mozilla/5.0 Chrome", false},
		{"curl/7.68.0", false},
		{"PostmanRuntime/7.28.4", false},
	}

	for _, tt := range tests {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.Header.Set("User-Agent", tt.userAgent)

		got := isAIAgent(req)
		if got != tt.expected {
			t.Errorf("isAIAgent with UA %q = %v, want %v", tt.userAgent, got, tt.expected)
		}
	}
}

func TestIsAIAgent_BudgetHeader(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Agent-Budget", "10000")

	if !isAIAgent(req) {
		t.Error("Expected X-Agent-Budget header to indicate AI agent")
	}
}

func TestParseAIAgentHeaders(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Agent-Budget", "5000")
	req.Header.Set("X-Agent-Task-ID", "task_123")
	req.Header.Set("X-Agent-Batch-Size", "10")
	req.Header.Set("X-Agent-Priority", "high")
	req.Header.Set("X-Agent-Retry-Count", "2")

	headers := ParseAIAgentHeaders(req)

	if headers.AgentBudget != 5000 {
		t.Errorf("Expected budget 5000, got %d", headers.AgentBudget)
	}
	if headers.AgentTaskID != "task_123" {
		t.Errorf("Expected task ID task_123, got %s", headers.AgentTaskID)
	}
	if headers.AgentBatchSize != 10 {
		t.Errorf("Expected batch size 10, got %d", headers.AgentBatchSize)
	}
	if headers.AgentPriority != "high" {
		t.Errorf("Expected priority high, got %s", headers.AgentPriority)
	}
	if headers.AgentRetryCount != 2 {
		t.Errorf("Expected retry count 2, got %d", headers.AgentRetryCount)
	}
}

func TestSetAIAgentResponseHeaders(t *testing.T) {
	rr := httptest.NewRecorder()

	headers := AIAgentHeaders{
		EstimatedCost:     100,
		ActualCost:        100,
		RemainingBudget:   4900,
		RecommendedRetry:  5,
		BatchPricePerItem: 90,
		StreamingSupport:  true,
	}

	SetAIAgentResponseHeaders(rr, headers)

	if rr.Header().Get("X-Estimated-Cost") != "100" {
		t.Errorf("Expected X-Estimated-Cost 100, got %s", rr.Header().Get("X-Estimated-Cost"))
	}
	if rr.Header().Get("X-Remaining-Budget") != "4900" {
		t.Errorf("Expected X-Remaining-Budget 4900, got %s", rr.Header().Get("X-Remaining-Budget"))
	}
	if rr.Header().Get("Retry-After") != "5" {
		t.Errorf("Expected Retry-After 5, got %s", rr.Header().Get("Retry-After"))
	}
	if rr.Header().Get("X-Streaming-Supported") != "true" {
		t.Errorf("Expected X-Streaming-Supported true, got %s", rr.Header().Get("X-Streaming-Supported"))
	}
}

func TestAIAgentMiddleware_DetectsAgent(t *testing.T) {
	handler := AIAgentMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if AI agent was detected
			if r.Header.Get("X-AI-Agent-Detected") != "true" {
				t.Error("Expected X-AI-Agent-Detected to be set")
			}
			w.WriteHeader(http.StatusOK)
		}),
		Config{PricePerRequest: 100, Currency: "USDC"},
		AIAgentConfig{
			EnableBudgetAwareness: true,
			EnableCostEstimation:  true,
			Currency:              "USDC",
		},
	)

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-AI-Agent", "true")
	req.Header.Set("X-Agent-Budget", "10000")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	// Check cost estimation header
	if rr.Header().Get("X-Estimated-Cost") != "100" {
		t.Errorf("Expected X-Estimated-Cost 100, got %s", rr.Header().Get("X-Estimated-Cost"))
	}
}

func TestAIAgentMiddleware_BudgetExceeded(t *testing.T) {
	handler := AIAgentMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		Config{PricePerRequest: 1000, Currency: "USDC"},
		AIAgentConfig{
			EnableBudgetAwareness: true,
			Currency:              "USDC",
		},
	)

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-AI-Agent", "true")
	req.Header.Set("X-Agent-Budget", "100") // Budget less than price
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusPaymentRequired {
		t.Errorf("Expected status 402 for budget exceeded, got %d", rr.Code)
	}

	if rr.Header().Get("X-Budget-Exceeded") != "true" {
		t.Error("Expected X-Budget-Exceeded header")
	}
}

func TestAIAgentMiddleware_BatchPricing(t *testing.T) {
	handler := AIAgentMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		Config{PricePerRequest: 100, Currency: "USDC"},
		AIAgentConfig{
			EnableBatchPricing: true,
			BatchDiscount:      10, // 10% discount
			MinBatchSize:       5,
			Currency:           "USDC",
		},
	)

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-AI-Agent", "true")
	req.Header.Set("X-Agent-Batch-Size", "10")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Should get 90 (100 - 10%)
	if rr.Header().Get("X-Batch-Price-Per-Item") != "90" {
		t.Errorf("Expected batch price 90, got %s", rr.Header().Get("X-Batch-Price-Per-Item"))
	}
}

func TestCalculateBatchPrice(t *testing.T) {
	tests := []struct {
		basePrice int64
		batchSize int
		discount  int
		expected  int64
	}{
		{100, 10, 10, 90},    // 10% off
		{100, 10, 20, 80},    // 20% off
		{100, 10, 0, 100},    // No discount
		{100, 10, -5, 100},   // Invalid discount
		{1000, 100, 15, 850}, // 15% off 1000
	}

	for _, tt := range tests {
		got := calculateBatchPrice(tt.basePrice, tt.batchSize, tt.discount)
		if got != tt.expected {
			t.Errorf("calculateBatchPrice(%d, %d, %d) = %d, want %d",
				tt.basePrice, tt.batchSize, tt.discount, got, tt.expected)
		}
	}
}

func TestCostEstimateHandler(t *testing.T) {
	pricing := map[string]int64{
		"GET:/api/expensive": 500,
		"/api/cheap":         10,
		"default":            100,
	}

	handler := CostEstimateHandler(pricing, "USDC")

	// Test specific endpoint with method
	req := httptest.NewRequest("GET", "/cost?endpoint=/api/expensive&method=GET", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	// Test default pricing
	req = httptest.NewRequest("GET", "/cost?endpoint=/api/unknown", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var estimate CostEstimate
	if err := json.NewDecoder(rr.Body).Decode(&estimate); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if estimate.EstimatedCost != 100 {
		t.Errorf("Expected default cost 100, got %d", estimate.EstimatedCost)
	}
}

func TestAgentWelcomeHandler(t *testing.T) {
	info := AgentWelcomeInfo{
		Service:          "Test API",
		Version:          "1.0",
		X402Compliant:    true,
		AIAgentOptimized: true,
		Features:         []string{"sessions", "batch", "metering"},
	}

	handler := AgentWelcomeHandler(info)

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	if rr.Header().Get("X-AI-Agent-Optimized") != "true" {
		t.Error("Expected X-AI-Agent-Optimized header")
	}

	var response AgentWelcomeInfo
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Service != "Test API" {
		t.Errorf("Expected service 'Test API', got %s", response.Service)
	}
}
