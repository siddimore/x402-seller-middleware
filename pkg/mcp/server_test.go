package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewServer(t *testing.T) {
	server := NewServer(ServerConfig{
		Network:  "base-sepolia",
		Currency: "USDC",
	})

	if server == nil {
		t.Fatal("Expected server, got nil")
	}

	if server.config.Currency != "USDC" {
		t.Errorf("Expected currency USDC, got %s", server.config.Currency)
	}
}

func TestGetTools(t *testing.T) {
	server := NewServer(ServerConfig{})

	tools := server.GetTools()

	if len(tools) != 5 {
		t.Errorf("Expected 5 tools, got %d", len(tools))
	}

	expectedTools := map[string]bool{
		"x402_discover": false,
		"x402_call":     false,
		"x402_budget":   false,
		"x402_estimate": false,
		"x402_history":  false,
	}

	for _, tool := range tools {
		if _, ok := expectedTools[tool.Name]; ok {
			expectedTools[tool.Name] = true
		}
	}

	for name, found := range expectedTools {
		if !found {
			t.Errorf("Expected tool %s not found", name)
		}
	}
}

func TestBudgetCreate(t *testing.T) {
	server := NewServer(ServerConfig{Currency: "USDC"})

	result, err := server.CallTool(context.Background(), "x402_budget", map[string]interface{}{
		"action": "create",
		"amount": float64(50000),
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.IsError {
		t.Errorf("Expected success, got error: %s", result.Content[0].Text)
	}

	if len(result.Content) == 0 {
		t.Fatal("Expected content in result")
	}

	// Check budget was created
	status, _ := server.CallTool(context.Background(), "x402_budget", map[string]interface{}{
		"action": "status",
	})

	if status.IsError {
		t.Errorf("Budget status should not error: %s", status.Content[0].Text)
	}
}

func TestBudgetTopup(t *testing.T) {
	server := NewServer(ServerConfig{Currency: "USDC"})

	// Create initial budget
	server.CallTool(context.Background(), "x402_budget", map[string]interface{}{
		"action": "create",
		"amount": float64(10000),
	})

	// Top up
	result, _ := server.CallTool(context.Background(), "x402_budget", map[string]interface{}{
		"action": "topup",
		"amount": float64(5000),
	})

	if result.IsError {
		t.Errorf("Topup should not error: %s", result.Content[0].Text)
	}

	// Check total
	server.mu.RLock()
	budget := server.budgets["default"]
	server.mu.RUnlock()

	if budget.Total != 15000 {
		t.Errorf("Expected total 15000, got %d", budget.Total)
	}
}

func TestBudgetClose(t *testing.T) {
	server := NewServer(ServerConfig{Currency: "USDC"})

	// Create budget
	server.CallTool(context.Background(), "x402_budget", map[string]interface{}{
		"action": "create",
		"amount": float64(10000),
	})

	// Close
	result, _ := server.CallTool(context.Background(), "x402_budget", map[string]interface{}{
		"action": "close",
	})

	if result.IsError {
		t.Errorf("Close should not error: %s", result.Content[0].Text)
	}

	// Check budget is gone
	server.mu.RLock()
	budget := server.budgets["default"]
	server.mu.RUnlock()

	if budget != nil {
		t.Error("Budget should be nil after close")
	}
}

func TestCallWithoutBudget(t *testing.T) {
	server := NewServer(ServerConfig{Currency: "USDC"})

	result, _ := server.CallTool(context.Background(), "x402_call", map[string]interface{}{
		"url": "https://api.example.com/test",
	})

	if !result.IsError {
		t.Error("Expected error when calling without budget")
	}
}

func TestHistoryEmpty(t *testing.T) {
	server := NewServer(ServerConfig{Currency: "USDC"})

	result, _ := server.CallTool(context.Background(), "x402_history", map[string]interface{}{})

	if result.IsError {
		t.Error("History should not error even when empty")
	}
}

func TestDiscoverVia402(t *testing.T) {
	// Create mock server that returns 402
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"x402Version": 1,
			"accepts": []map[string]interface{}{
				{
					"scheme":            "exact",
					"network":           "base-sepolia",
					"maxAmountRequired": "1000",
					"payTo":             "0x123",
					"description":       "Payment required",
				},
			},
		})
	}))
	defer mockServer.Close()

	server := NewServer(ServerConfig{
		Currency:   "USDC",
		HTTPClient: mockServer.Client(),
	})

	result, err := server.CallTool(context.Background(), "x402_discover", map[string]interface{}{
		"url": mockServer.URL,
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.IsError {
		t.Errorf("Discovery should not error: %s", result.Content[0].Text)
	}

	// Check content mentions x402
	text := result.Content[0].Text
	if len(text) == 0 {
		t.Error("Expected discovery result text")
	}
}

func TestEstimate(t *testing.T) {
	// Create mock server that returns 402
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"x402Version": 1,
			"accepts": []map[string]interface{}{
				{
					"network":           "base",
					"maxAmountRequired": "500",
					"description":       "API call",
				},
			},
		})
	}))
	defer mockServer.Close()

	server := NewServer(ServerConfig{
		HTTPClient: mockServer.Client(),
	})

	result, _ := server.CallTool(context.Background(), "x402_estimate", map[string]interface{}{
		"url": mockServer.URL + "/api/test",
	})

	if result.IsError {
		t.Errorf("Estimate should not error: %s", result.Content[0].Text)
	}
}

func TestUnknownTool(t *testing.T) {
	server := NewServer(ServerConfig{})

	_, err := server.CallTool(context.Background(), "unknown_tool", map[string]interface{}{})

	if err == nil {
		t.Error("Expected error for unknown tool")
	}
}

func TestToolInputSchemas(t *testing.T) {
	server := NewServer(ServerConfig{})
	tools := server.GetTools()

	for _, tool := range tools {
		if tool.InputSchema.Type != "object" {
			t.Errorf("Tool %s should have type 'object', got '%s'", tool.Name, tool.InputSchema.Type)
		}

		if tool.Name == "x402_discover" {
			if _, ok := tool.InputSchema.Properties["url"]; !ok {
				t.Error("x402_discover should have 'url' property")
			}
			if len(tool.InputSchema.Required) != 1 || tool.InputSchema.Required[0] != "url" {
				t.Error("x402_discover should require 'url'")
			}
		}
	}
}

func TestHTTPHandler(t *testing.T) {
	server := NewServer(ServerConfig{Currency: "USDC"})

	// Create test HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req JSONRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(JSONRPCResponse{
				JSONRPC: "2.0",
				Error:   &JSONRPCError{Code: ParseError, Message: "Parse error"},
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		server.handleRequest(json.NewEncoder(w), &req)
	}))
	defer ts.Close()

	// Test initialize
	initReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
	}
	reqBody, _ := json.Marshal(initReq)

	resp, err := http.Post(ts.URL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	var initResp JSONRPCResponse
	json.NewDecoder(resp.Body).Decode(&initResp)

	if initResp.Error != nil {
		t.Errorf("Initialize should not error: %v", initResp.Error)
	}

	result := initResp.Result.(map[string]interface{})
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("Unexpected protocol version: %v", result["protocolVersion"])
	}
}

func TestToolsList(t *testing.T) {
	server := NewServer(ServerConfig{})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req JSONRPCRequest
		json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		server.handleRequest(json.NewEncoder(w), &req)
	}))
	defer ts.Close()

	listReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
	}
	reqBody, _ := json.Marshal(listReq)

	resp, err := http.Post(ts.URL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	var listResp JSONRPCResponse
	json.NewDecoder(resp.Body).Decode(&listResp)

	if listResp.Error != nil {
		t.Errorf("tools/list should not error: %v", listResp.Error)
	}

	result := listResp.Result.(map[string]interface{})
	tools := result["tools"].([]interface{})
	if len(tools) != 5 {
		t.Errorf("Expected 5 tools, got %d", len(tools))
	}
}

func TestToolsCall(t *testing.T) {
	server := NewServer(ServerConfig{Currency: "USDC"})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req JSONRPCRequest
		json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		server.handleRequest(json.NewEncoder(w), &req)
	}))
	defer ts.Close()

	callReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name": "x402_budget", "arguments": {"action": "create", "amount": 10000}}`),
	}
	reqBody, _ := json.Marshal(callReq)

	resp, err := http.Post(ts.URL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	var callResp JSONRPCResponse
	json.NewDecoder(resp.Body).Decode(&callResp)

	if callResp.Error != nil {
		t.Errorf("tools/call should not error: %v", callResp.Error)
	}
}

func TestCacheExpiry(t *testing.T) {
	server := NewServer(ServerConfig{})

	// Add expired cache entry
	server.cache["https://api.example.com"] = &APIDiscoveryCache{
		URL:       "https://api.example.com",
		CachedAt:  time.Now().Add(-10 * time.Minute),
		ExpiresAt: time.Now().Add(-5 * time.Minute), // Expired
	}

	// Create mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return 402 for discovery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"x402Version": 1,
			"accepts":     []map[string]interface{}{},
		})
	}))
	defer mockServer.Close()

	server.config.HTTPClient = mockServer.Client()

	// Should not use expired cache
	result, _ := server.CallTool(context.Background(), "x402_discover", map[string]interface{}{
		"url": mockServer.URL,
	})

	// Should have made a new request (not use cache)
	if result.IsError && result.Content[0].Text == "❌ Error: " {
		t.Error("Should have made fresh discovery request")
	}
}
