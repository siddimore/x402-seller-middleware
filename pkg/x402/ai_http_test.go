package x402

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGenerateOpenAIFunctions(t *testing.T) {
	endpoints := []APIEndpoint{
		{
			Path:        "/api/articles",
			Method:      "GET",
			Name:        "list_articles",
			Description: "List all articles",
			Parameters: []EndpointParam{
				{Name: "limit", In: "query", Type: "integer", Required: false, Description: "Max results"},
				{Name: "category", In: "query", Type: "string", Required: true, Description: "Category filter"},
			},
			Cost:     100,
			Currency: "USDC",
			CostUnit: "per_call",
		},
	}

	functions := GenerateOpenAIFunctions(endpoints)

	if len(functions) != 1 {
		t.Fatalf("Expected 1 function, got %d", len(functions))
	}

	fn := functions[0]
	if fn.Name != "list_articles" {
		t.Errorf("Expected name 'list_articles', got '%s'", fn.Name)
	}

	if fn.Cost == nil {
		t.Fatal("Expected cost info")
	}
	if fn.Cost.Amount != 100 {
		t.Errorf("Expected cost 100, got %d", fn.Cost.Amount)
	}

	if len(fn.Parameters.Properties) != 2 {
		t.Errorf("Expected 2 properties, got %d", len(fn.Parameters.Properties))
	}

	if len(fn.Parameters.Required) != 1 || fn.Parameters.Required[0] != "category" {
		t.Errorf("Expected 'category' to be required, got %v", fn.Parameters.Required)
	}
}

func TestGenerateMCPTools(t *testing.T) {
	endpoints := []APIEndpoint{
		{
			Path:        "/api/search",
			Method:      "POST",
			Name:        "search_content",
			Description: "Search for content",
			Parameters: []EndpointParam{
				{Name: "query", In: "body", Type: "string", Required: true},
			},
			Cost:     50,
			Currency: "USDC",
			CostUnit: "per_call",
		},
	}

	tools := GenerateMCPTools(endpoints)

	if len(tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(tools))
	}

	tool := tools[0]
	if tool.Name != "search_content" {
		t.Errorf("Expected name 'search_content', got '%s'", tool.Name)
	}

	if tool.InputSchema.Type != "object" {
		t.Errorf("Expected type 'object', got '%s'", tool.InputSchema.Type)
	}

	if tool.Cost == nil || tool.Cost.Amount != 50 {
		t.Error("Expected cost of 50")
	}
}

func TestPreAuthStore_CreateAndDeduct(t *testing.T) {
	store := NewInMemoryPreAuthStore()

	budget := &PreAuthBudget{
		AgentID:       "agent_123",
		WalletAddress: "0xabc",
		TotalBudget:   10000,
		Currency:      "USDC",
		ExpiresAt:     time.Now().Add(time.Hour),
	}

	err := store.Create(budget)
	if err != nil {
		t.Fatalf("Failed to create budget: %v", err)
	}

	if budget.ID == "" {
		t.Error("Expected budget ID to be set")
	}

	// Get by agent ID
	retrieved, err := store.GetByAgentID("agent_123")
	if err != nil {
		t.Fatalf("Failed to get budget: %v", err)
	}
	if retrieved.Remaining != 10000 {
		t.Errorf("Expected remaining 10000, got %d", retrieved.Remaining)
	}

	// Deduct
	err = store.Deduct(budget.ID, 100)
	if err != nil {
		t.Fatalf("Failed to deduct: %v", err)
	}

	retrieved, _ = store.Get(budget.ID)
	if retrieved.Remaining != 9900 {
		t.Errorf("Expected remaining 9900, got %d", retrieved.Remaining)
	}
	if retrieved.TotalSpent != 100 {
		t.Errorf("Expected spent 100, got %d", retrieved.TotalSpent)
	}
	if retrieved.RequestCount != 1 {
		t.Errorf("Expected 1 request, got %d", retrieved.RequestCount)
	}
}

func TestPreAuthStore_InsufficientBudget(t *testing.T) {
	store := NewInMemoryPreAuthStore()

	budget := &PreAuthBudget{
		AgentID:     "agent_456",
		TotalBudget: 100,
		Currency:    "USDC",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	store.Create(budget)

	// Try to deduct more than available
	err := store.Deduct(budget.ID, 200)
	if err == nil {
		t.Error("Expected error for insufficient budget")
	}
}

func TestIdempotencyStore(t *testing.T) {
	store := NewInMemoryIdempotencyStore()

	record := &IdempotencyRecord{
		StatusCode: 200,
		Headers:    map[string]string{"X-Custom": "value"},
		Body:       []byte(`{"result":"success"}`),
		ExpiresAt:  time.Now().Add(time.Hour),
	}

	err := store.Set("key_123", record)
	if err != nil {
		t.Fatalf("Failed to set record: %v", err)
	}

	// Retrieve
	retrieved, err := store.Get("key_123")
	if err != nil {
		t.Fatalf("Failed to get record: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Expected record, got nil")
	}
	if retrieved.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", retrieved.StatusCode)
	}

	// Non-existent key
	missing, _ := store.Get("nonexistent")
	if missing != nil {
		t.Error("Expected nil for non-existent key")
	}
}

func TestIdempotencyStore_Expiry(t *testing.T) {
	store := NewInMemoryIdempotencyStore()

	record := &IdempotencyRecord{
		StatusCode: 200,
		Body:       []byte(`test`),
		ExpiresAt:  time.Now().Add(-time.Hour), // Already expired
	}
	store.Set("expired_key", record)

	// Should not return expired record
	retrieved, _ := store.Get("expired_key")
	if retrieved != nil {
		t.Error("Expected nil for expired record")
	}
}

func TestAIDiscoveryHandler_Default(t *testing.T) {
	config := AIFirstConfig{
		Endpoints: []APIEndpoint{
			{Path: "/api/test", Method: "GET", Name: "test", Cost: 100, Currency: "USDC"},
		},
		PayTo:    "0x123",
		Network:  "base",
		Currency: "USDC",
	}

	handler := AIDiscoveryHandler(config)

	req := httptest.NewRequest("GET", "/ai/discover", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["name"] != "AI-First x402 API" {
		t.Errorf("Unexpected name: %v", response["name"])
	}

	protocol := response["protocol"].(map[string]interface{})
	if protocol["aiOptimized"] != true {
		t.Error("Expected aiOptimized to be true")
	}
}

func TestAIDiscoveryHandler_OpenAIFormat(t *testing.T) {
	config := AIFirstConfig{
		Endpoints: []APIEndpoint{
			{Path: "/api/test", Method: "GET", Name: "test_function", Cost: 100, Currency: "USDC"},
		},
	}

	handler := AIDiscoveryHandler(config)

	req := httptest.NewRequest("GET", "/ai/discover?format=openai", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	var response map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &response)

	functions := response["functions"].([]interface{})
	if len(functions) != 1 {
		t.Errorf("Expected 1 function, got %d", len(functions))
	}

	fn := functions[0].(map[string]interface{})
	if fn["name"] != "test_function" {
		t.Errorf("Expected function name 'test_function', got %v", fn["name"])
	}
}

func TestAIDiscoveryHandler_MCPFormat(t *testing.T) {
	config := AIFirstConfig{
		Endpoints: []APIEndpoint{
			{Path: "/api/tool", Method: "POST", Name: "my_tool", Cost: 50, Currency: "USDC"},
		},
		Network:  "base-sepolia",
		Currency: "USDC",
	}

	handler := AIDiscoveryHandler(config)

	req := httptest.NewRequest("GET", "/ai/discover?format=mcp", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	var response MCPToolsResponse
	json.Unmarshal(rr.Body.Bytes(), &response)

	if len(response.Tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(response.Tools))
	}

	if response.PaymentInfo.Protocol != "x402" {
		t.Errorf("Expected protocol 'x402', got '%s'", response.PaymentInfo.Protocol)
	}
}

func TestAIBudgetHandler_Create(t *testing.T) {
	store := NewInMemoryPreAuthStore()
	config := AIFirstConfig{Currency: "USDC"}

	handler := AIBudgetHandler(store, config)

	body := `{"agentId": "test_agent", "walletAddress": "0xabc", "budget": 5000}`
	req := httptest.NewRequest("POST", "/ai/budget", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", rr.Code)
	}

	var budget PreAuthBudget
	json.Unmarshal(rr.Body.Bytes(), &budget)

	if budget.AgentID != "test_agent" {
		t.Errorf("Expected agentId 'test_agent', got '%s'", budget.AgentID)
	}
	if budget.TotalBudget != 5000 {
		t.Errorf("Expected budget 5000, got %d", budget.TotalBudget)
	}
}

func TestAIBudgetHandler_Get(t *testing.T) {
	store := NewInMemoryPreAuthStore()
	config := AIFirstConfig{Currency: "USDC"}

	// Create a budget first
	budget := &PreAuthBudget{
		AgentID:     "existing_agent",
		TotalBudget: 1000,
		Currency:    "USDC",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	store.Create(budget)

	handler := AIBudgetHandler(store, config)

	req := httptest.NewRequest("GET", "/ai/budget?agentId=existing_agent", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	var result PreAuthBudget
	json.Unmarshal(rr.Body.Bytes(), &result)

	if result.TotalBudget != 1000 {
		t.Errorf("Expected budget 1000, got %d", result.TotalBudget)
	}
}

func TestAIFirstMiddleware_IdempotentRequest(t *testing.T) {
	idempStore := NewInMemoryIdempotencyStore()

	callCount := 0
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"count":` + string(rune('0'+callCount)) + `}`))
	})

	handler := AIFirstMiddleware(innerHandler, AIFirstConfig{
		EnableIdempotency: true,
		IdempotencyStore:  idempStore,
	})

	// First request
	req1 := httptest.NewRequest("POST", "/api/test", nil)
	req1.Header.Set("Idempotency-Key", "unique_key_123")
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)

	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}

	// Second request with same key - should replay
	req2 := httptest.NewRequest("POST", "/api/test", nil)
	req2.Header.Set("Idempotency-Key", "unique_key_123")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if callCount != 1 {
		t.Errorf("Expected call count to stay 1, got %d", callCount)
	}

	if rr2.Header().Get("X-Idempotent-Replay") != "true" {
		t.Error("Expected X-Idempotent-Replay header")
	}
}

func TestAIFirstMiddleware_PreAuthBudget(t *testing.T) {
	preAuthStore := NewInMemoryPreAuthStore()

	// Create budget
	budget := &PreAuthBudget{
		AgentID:     "my_agent",
		TotalBudget: 1000,
		Currency:    "USDC",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	preAuthStore.Create(budget)

	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	config := AIFirstConfig{
		EnablePreAuth: true,
		PreAuthStore:  preAuthStore,
		DefaultCost:   100,
		Endpoints:     []APIEndpoint{{Path: "/api/test", Method: "GET", Cost: 100}},
	}

	handler := AIFirstMiddleware(innerHandler, config)

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Agent-ID", "my_agent")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	if rr.Header().Get("X-Budget-Remaining") != "900" {
		t.Errorf("Expected remaining 900, got %s", rr.Header().Get("X-Budget-Remaining"))
	}

	// Check budget was deducted
	updated, _ := preAuthStore.GetByAgentID("my_agent")
	if updated.Remaining != 900 {
		t.Errorf("Expected budget remaining 900, got %d", updated.Remaining)
	}
}

func TestAIFirstMiddleware_InsufficientPreAuth(t *testing.T) {
	preAuthStore := NewInMemoryPreAuthStore()

	// Create budget with low balance
	budget := &PreAuthBudget{
		AgentID:     "poor_agent",
		TotalBudget: 50,
		Currency:    "USDC",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	preAuthStore.Create(budget)

	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	config := AIFirstConfig{
		EnablePreAuth: true,
		PreAuthStore:  preAuthStore,
		DefaultCost:   100, // More than budget
		PayTo:         "0x123",
		Network:       "base",
		Currency:      "USDC",
	}

	handler := AIFirstMiddleware(innerHandler, config)

	req := httptest.NewRequest("GET", "/api/expensive", nil)
	req.Header.Set("X-Agent-ID", "poor_agent")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusPaymentRequired {
		t.Errorf("Expected status 402, got %d", rr.Code)
	}

	var response AIResponse
	json.Unmarshal(rr.Body.Bytes(), &response)

	if response.Success {
		t.Error("Expected success to be false")
	}

	if response.Error == nil {
		t.Fatal("Expected error in response")
	}

	if response.Error.Code != ErrCodeInsufficientBudget {
		t.Errorf("Expected error code '%s', got '%s'", ErrCodeInsufficientBudget, response.Error.Code)
	}

	if response.Error.Action != "pay" {
		t.Errorf("Expected action 'pay', got '%s'", response.Error.Action)
	}

	if response.Error.PaymentInfo == nil {
		t.Error("Expected payment info")
	}
}

func TestAIResponse_Structure(t *testing.T) {
	// Test that AIResponse serializes correctly
	response := AIResponse{
		Success: true,
		Data:    map[string]string{"result": "test"},
		Meta: AIMetadata{
			RequestID:    "req_123",
			Timestamp:    "2026-01-10T12:00:00Z",
			ProcessingMs: 50,
			Cost: &Cost{
				Amount:   100,
				Currency: "USDC",
			},
		},
	}

	bytes, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var parsed AIResponse
	if err := json.Unmarshal(bytes, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if !parsed.Success {
		t.Error("Expected success to be true")
	}
	if parsed.Meta.RequestID != "req_123" {
		t.Errorf("Expected requestId 'req_123', got '%s'", parsed.Meta.RequestID)
	}
	if parsed.Meta.Cost.Amount != 100 {
		t.Errorf("Expected cost 100, got %d", parsed.Meta.Cost.Amount)
	}
}

func TestAIError_Structure(t *testing.T) {
	err := AIError{
		Code:       ErrCodePaymentRequired,
		Message:    "Payment required to access this resource",
		Retryable:  true,
		RetryAfter: 5,
		Action:     "pay",
		Details:    map[string]string{"endpoint": "/api/premium"},
		PaymentInfo: &PaymentAction{
			Required: true,
			Amount:   100,
			Currency: "USDC",
			Network:  "base",
		},
	}

	bytes, _ := json.Marshal(err)
	var parsed AIError
	json.Unmarshal(bytes, &parsed)

	if parsed.Code != ErrCodePaymentRequired {
		t.Errorf("Expected code '%s', got '%s'", ErrCodePaymentRequired, parsed.Code)
	}
	if !parsed.Retryable {
		t.Error("Expected retryable to be true")
	}
	if parsed.PaymentInfo.Amount != 100 {
		t.Errorf("Expected payment amount 100, got %d", parsed.PaymentInfo.Amount)
	}
}
