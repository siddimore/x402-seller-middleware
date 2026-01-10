// Package mcp provides a Model Context Protocol server for x402 payments.
// This allows AI agents (Claude, GPT, etc.) to automatically discover,
// pay for, and call x402-protected APIs.
//
// Usage:
//
//	server := mcp.NewServer(mcp.ServerConfig{
//	    WalletAddress: "0x...",
//	    Network:       "base",
//	    Facilitator:   "https://facilitator.example.com",
//	})
//	server.ListenStdio() // For CLI usage
//	// or
//	server.ListenHTTP(":8080") // For HTTP transport
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

// ============================================================================
// MCP PROTOCOL TYPES
// Based on https://modelcontextprotocol.io/docs/specification
// ============================================================================

// JSONRPCRequest is a JSON-RPC 2.0 request
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse is a JSON-RPC 2.0 response
type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      interface{}   `json:"id,omitempty"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

// JSONRPCError is a JSON-RPC 2.0 error
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// MCP error codes
const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
)

// Tool represents an MCP tool definition
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

// InputSchema defines the tool's input parameters
type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
}

// Property defines a single input property
type Property struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Default     any      `json:"default,omitempty"`
}

// ToolResult is the result of a tool call
type ToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock is a piece of content in a tool result
type ContentBlock struct {
	Type string `json:"type"` // "text", "image", "resource"
	Text string `json:"text,omitempty"`
}

// ============================================================================
// X402 MCP SERVER
// ============================================================================

// ServerConfig configures the MCP server
type ServerConfig struct {
	// Wallet configuration
	WalletAddress string // Your wallet address for payments
	PrivateKey    string // Private key for signing (optional, for auto-pay)

	// Network configuration
	Network     string // "base", "base-sepolia", "ethereum"
	Facilitator string // Facilitator URL for payment verification

	// Budget configuration
	DefaultBudget    int64  // Default spending budget per session
	MaxBudgetPerCall int64  // Maximum spend per single call
	Currency         string // "USDC", "ETH", etc.

	// Known APIs (pre-configured endpoints)
	KnownAPIs []KnownAPI

	// HTTP client for making requests
	HTTPClient *http.Client
}

// KnownAPI represents a pre-configured API endpoint
type KnownAPI struct {
	Name        string `json:"name"`
	BaseURL     string `json:"baseUrl"`
	Description string `json:"description"`
	AuthHeader  string `json:"authHeader,omitempty"` // Default: "X-PAYMENT"
}

// Server is the MCP server for x402 payments
type Server struct {
	config  ServerConfig
	mu      sync.RWMutex
	budgets map[string]*Budget // sessionID -> budget
	cache   map[string]*APIDiscoveryCache
}

// Budget tracks spending for a session
type Budget struct {
	SessionID    string
	Total        int64
	Spent        int64
	Remaining    int64
	Currency     string
	CreatedAt    time.Time
	LastUsedAt   time.Time
	Transactions []Transaction
}

// Transaction records a payment
type Transaction struct {
	Timestamp time.Time
	API       string
	Endpoint  string
	Amount    int64
	Currency  string
	Success   bool
	RequestID string
}

// APIDiscoveryCache caches API discovery results
type APIDiscoveryCache struct {
	URL       string
	Endpoints []DiscoveredEndpoint
	CachedAt  time.Time
	ExpiresAt time.Time
}

// DiscoveredEndpoint represents a discovered API endpoint
type DiscoveredEndpoint struct {
	Path        string `json:"path"`
	Method      string `json:"method"`
	Description string `json:"description"`
	Cost        int64  `json:"cost"`
	Currency    string `json:"currency"`
}

// NewServer creates a new MCP server
func NewServer(config ServerConfig) *Server {
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if config.Currency == "" {
		config.Currency = "USDC"
	}
	if config.DefaultBudget == 0 {
		config.DefaultBudget = 100000 // 0.10 USDC in smallest units
	}

	return &Server{
		config:  config,
		budgets: make(map[string]*Budget),
		cache:   make(map[string]*APIDiscoveryCache),
	}
}

// GetTools returns the list of available tools
func (s *Server) GetTools() []Tool {
	return []Tool{
		{
			Name:        "x402_discover",
			Description: "Discover available paid API endpoints and their costs. Use this before calling a paid API to understand pricing.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"url": {
						Type:        "string",
						Description: "Base URL of the API to discover (e.g., https://api.example.com)",
					},
				},
				Required: []string{"url"},
			},
		},
		{
			Name:        "x402_call",
			Description: "Call a paid API endpoint with automatic x402 payment handling. The payment will be deducted from your pre-authorized budget.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"url": {
						Type:        "string",
						Description: "Full URL of the API endpoint to call",
					},
					"method": {
						Type:        "string",
						Description: "HTTP method",
						Enum:        []string{"GET", "POST", "PUT", "DELETE", "PATCH"},
						Default:     "GET",
					},
					"headers": {
						Type:        "object",
						Description: "Additional headers to send (optional)",
					},
					"body": {
						Type:        "string",
						Description: "Request body for POST/PUT/PATCH requests (optional)",
					},
					"max_cost": {
						Type:        "number",
						Description: "Maximum cost willing to pay for this call (in smallest currency unit)",
					},
				},
				Required: []string{"url"},
			},
		},
		{
			Name:        "x402_budget",
			Description: "Manage your x402 spending budget. Create, check, or top up your pre-authorized budget.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"action": {
						Type:        "string",
						Description: "Action to perform",
						Enum:        []string{"create", "status", "topup", "close"},
					},
					"amount": {
						Type:        "number",
						Description: "Amount for create/topup actions (in smallest currency unit)",
					},
				},
				Required: []string{"action"},
			},
		},
		{
			Name:        "x402_estimate",
			Description: "Estimate the cost of an API call before making it. Useful for budget planning.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"url": {
						Type:        "string",
						Description: "Full URL of the API endpoint",
					},
					"method": {
						Type:        "string",
						Description: "HTTP method",
						Default:     "GET",
					},
				},
				Required: []string{"url"},
			},
		},
		{
			Name:        "x402_history",
			Description: "View your x402 payment history and spending analytics.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"limit": {
						Type:        "number",
						Description: "Maximum number of transactions to return",
						Default:     10,
					},
				},
			},
		},
	}
}

// CallTool handles a tool call
func (s *Server) CallTool(ctx context.Context, name string, args map[string]interface{}) (*ToolResult, error) {
	switch name {
	case "x402_discover":
		return s.handleDiscover(ctx, args)
	case "x402_call":
		return s.handleCall(ctx, args)
	case "x402_budget":
		return s.handleBudget(ctx, args)
	case "x402_estimate":
		return s.handleEstimate(ctx, args)
	case "x402_history":
		return s.handleHistory(ctx, args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// ============================================================================
// TOOL IMPLEMENTATIONS
// ============================================================================

func (s *Server) handleDiscover(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	url, ok := args["url"].(string)
	if !ok {
		return errorResult("url is required"), nil
	}

	// Check cache
	s.mu.RLock()
	cached, ok := s.cache[url]
	s.mu.RUnlock()

	if ok && time.Now().Before(cached.ExpiresAt) {
		return s.formatDiscoveryResult(cached), nil
	}

	// Discover API
	discoveryURL := url + "/ai/discover"
	req, err := http.NewRequestWithContext(ctx, "GET", discoveryURL, nil)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to create request: %v", err)), nil
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-AI-Agent", "true")

	resp, err := s.config.HTTPClient.Do(req)
	if err != nil {
		// Try alternative discovery endpoint
		return s.discoverVia402(ctx, url)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return s.discoverVia402(ctx, url)
	}

	var discovery struct {
		Endpoints []DiscoveredEndpoint `json:"endpoints"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&discovery); err != nil {
		return errorResult(fmt.Sprintf("Failed to parse discovery response: %v", err)), nil
	}

	// Cache result
	cacheEntry := &APIDiscoveryCache{
		URL:       url,
		Endpoints: discovery.Endpoints,
		CachedAt:  time.Now(),
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	s.mu.Lock()
	s.cache[url] = cacheEntry
	s.mu.Unlock()

	return s.formatDiscoveryResult(cacheEntry), nil
}

func (s *Server) discoverVia402(ctx context.Context, baseURL string) (*ToolResult, error) {
	// Make a request to trigger 402 and extract payment requirements
	req, _ := http.NewRequestWithContext(ctx, "GET", baseURL, nil)
	req.Header.Set("X-AI-Agent", "true")

	resp, err := s.config.HTTPClient.Do(req)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to connect to API: %v", err)), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPaymentRequired {
		return textResult(fmt.Sprintf("API at %s does not require payment (status: %d)", baseURL, resp.StatusCode)), nil
	}

	// Parse x402 response
	var x402Resp struct {
		X402Version int `json:"x402Version"`
		Accepts     []struct {
			Scheme            string `json:"scheme"`
			Network           string `json:"network"`
			MaxAmountRequired string `json:"maxAmountRequired"`
			PayTo             string `json:"payTo"`
			Description       string `json:"description"`
		} `json:"accepts"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&x402Resp); err != nil {
		return errorResult("API returned 402 but response is not x402 compliant"), nil
	}

	// Format result
	var result string
	result = fmt.Sprintf("# API Discovery: %s\n\n", baseURL)
	result += "This API uses **x402 payment protocol**.\n\n"
	result += "## Payment Options:\n"

	for i, accept := range x402Resp.Accepts {
		result += fmt.Sprintf("\n### Option %d\n", i+1)
		result += fmt.Sprintf("- **Network**: %s\n", accept.Network)
		result += fmt.Sprintf("- **Amount**: %s\n", accept.MaxAmountRequired)
		result += fmt.Sprintf("- **Pay To**: %s\n", accept.PayTo)
		result += fmt.Sprintf("- **Description**: %s\n", accept.Description)
	}

	result += "\n\nUse `x402_call` to make a paid request to this API."

	return textResult(result), nil
}

func (s *Server) handleCall(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	url, _ := args["url"].(string)
	method, _ := args["method"].(string)
	if method == "" {
		method = "GET"
	}

	// Check budget
	s.mu.RLock()
	budget := s.budgets["default"]
	s.mu.RUnlock()

	if budget == nil {
		return errorResult("No budget set. Use x402_budget to create a spending budget first."), nil
	}

	maxCost := int64(0)
	if mc, ok := args["max_cost"].(float64); ok {
		maxCost = int64(mc)
	}

	// First, make request to get 402 requirements
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return errorResult(fmt.Sprintf("Invalid URL: %v", err)), nil
	}
	req.Header.Set("X-AI-Agent", "true")
	req.Header.Set("X-Agent-Budget", fmt.Sprintf("%d", budget.Remaining))

	resp, err := s.config.HTTPClient.Do(req)
	if err != nil {
		return errorResult(fmt.Sprintf("Request failed: %v", err)), nil
	}
	defer resp.Body.Close()

	// If not 402, return response directly
	if resp.StatusCode != http.StatusPaymentRequired {
		body, _ := io.ReadAll(resp.Body)
		return textResult(fmt.Sprintf("Response (Status %d):\n\n%s", resp.StatusCode, string(body))), nil
	}

	// Parse 402 response
	var x402Resp struct {
		Accepts []struct {
			MaxAmountRequired string `json:"maxAmountRequired"`
			Network           string `json:"network"`
			PayTo             string `json:"payTo"`
		} `json:"accepts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&x402Resp); err != nil {
		return errorResult("Failed to parse 402 response"), nil
	}

	if len(x402Resp.Accepts) == 0 {
		return errorResult("API returned 402 but no payment options available"), nil
	}

	// Get cost
	var cost int64
	if _, err := fmt.Sscanf(x402Resp.Accepts[0].MaxAmountRequired, "%d", &cost); err != nil {
		return errorResult("Failed to parse cost"), nil
	}

	// Check budget
	if cost > budget.Remaining {
		return errorResult(fmt.Sprintf(
			"Insufficient budget. Required: %d, Available: %d. Use x402_budget to top up.",
			cost, budget.Remaining,
		)), nil
	}

	// Check max cost limit
	if maxCost > 0 && cost > maxCost {
		return errorResult(fmt.Sprintf(
			"Cost (%d) exceeds your max_cost limit (%d). Increase limit or skip this call.",
			cost, maxCost,
		)), nil
	}

	// TODO: In production, this would:
	// 1. Sign the payment transaction
	// 2. Submit to facilitator
	// 3. Get payment proof
	// 4. Retry request with payment header

	// For now, simulate payment
	s.mu.Lock()
	budget.Spent += cost
	budget.Remaining -= cost
	budget.LastUsedAt = time.Now()
	budget.Transactions = append(budget.Transactions, Transaction{
		Timestamp: time.Now(),
		API:       url,
		Amount:    cost,
		Currency:  budget.Currency,
		Success:   true,
	})
	s.mu.Unlock()

	// Make paid request (simulated)
	result := "# Payment Processed\n\n"
	result += fmt.Sprintf("- **Amount**: %d %s\n", cost, budget.Currency)
	result += fmt.Sprintf("- **Remaining Budget**: %d %s\n", budget.Remaining, budget.Currency)
	result += "\n---\n\n"
	result += "⚠️ **Note**: In production, this would make the actual paid API call and return the response.\n"
	result += "Payment signing requires wallet integration."

	return textResult(result), nil
}

func (s *Server) handleBudget(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	action, _ := args["action"].(string)

	switch action {
	case "create":
		amount := s.config.DefaultBudget
		if a, ok := args["amount"].(float64); ok {
			amount = int64(a)
		}

		s.mu.Lock()
		s.budgets["default"] = &Budget{
			SessionID:  "default",
			Total:      amount,
			Spent:      0,
			Remaining:  amount,
			Currency:   s.config.Currency,
			CreatedAt:  time.Now(),
			LastUsedAt: time.Now(),
		}
		s.mu.Unlock()

		return textResult(fmt.Sprintf(
			"✅ Budget created!\n\n- **Total**: %d %s\n- **Available**: %d %s\n\nYou can now use `x402_call` to make paid API requests.",
			amount, s.config.Currency, amount, s.config.Currency,
		)), nil

	case "status":
		s.mu.RLock()
		budget := s.budgets["default"]
		s.mu.RUnlock()

		if budget == nil {
			return textResult("No budget set. Use `x402_budget` with action `create` to set up a spending budget."), nil
		}

		return textResult(fmt.Sprintf(
			"# Budget Status\n\n- **Total**: %d %s\n- **Spent**: %d %s\n- **Remaining**: %d %s\n- **Transactions**: %d\n- **Created**: %s",
			budget.Total, budget.Currency,
			budget.Spent, budget.Currency,
			budget.Remaining, budget.Currency,
			len(budget.Transactions),
			budget.CreatedAt.Format(time.RFC3339),
		)), nil

	case "topup":
		amount := int64(0)
		if a, ok := args["amount"].(float64); ok {
			amount = int64(a)
		}
		if amount <= 0 {
			return errorResult("amount is required for topup"), nil
		}

		s.mu.Lock()
		budget := s.budgets["default"]
		if budget == nil {
			budget = &Budget{
				SessionID: "default",
				Currency:  s.config.Currency,
				CreatedAt: time.Now(),
			}
			s.budgets["default"] = budget
		}
		budget.Total += amount
		budget.Remaining += amount
		budget.LastUsedAt = time.Now()
		s.mu.Unlock()

		return textResult(fmt.Sprintf(
			"✅ Budget topped up!\n\n- **Added**: %d %s\n- **New Total**: %d %s\n- **Available**: %d %s",
			amount, s.config.Currency,
			budget.Total, budget.Currency,
			budget.Remaining, budget.Currency,
		)), nil

	case "close":
		s.mu.Lock()
		budget := s.budgets["default"]
		delete(s.budgets, "default")
		s.mu.Unlock()

		if budget == nil {
			return textResult("No budget to close."), nil
		}

		return textResult(fmt.Sprintf(
			"✅ Budget closed!\n\n- **Total Spent**: %d %s\n- **Refunded**: %d %s\n- **Transactions**: %d",
			budget.Spent, budget.Currency,
			budget.Remaining, budget.Currency,
			len(budget.Transactions),
		)), nil

	default:
		return errorResult("Invalid action. Use: create, status, topup, or close"), nil
	}
}

func (s *Server) handleEstimate(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	url, _ := args["url"].(string)

	// Make request to get 402
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("X-AI-Agent", "true")

	resp, err := s.config.HTTPClient.Do(req)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to connect: %v", err)), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPaymentRequired {
		return textResult(fmt.Sprintf("This endpoint does not require payment (status: %d)", resp.StatusCode)), nil
	}

	var x402Resp struct {
		Accepts []struct {
			MaxAmountRequired string `json:"maxAmountRequired"`
			Network           string `json:"network"`
			Description       string `json:"description"`
		} `json:"accepts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&x402Resp); err != nil {
		return errorResult("Failed to parse estimate response"), nil
	}

	if len(x402Resp.Accepts) == 0 {
		return errorResult("Could not determine cost"), nil
	}

	accept := x402Resp.Accepts[0]
	return textResult(fmt.Sprintf(
		"# Cost Estimate\n\n- **URL**: %s\n- **Cost**: %s\n- **Network**: %s\n- **Description**: %s",
		url, accept.MaxAmountRequired, accept.Network, accept.Description,
	)), nil
}

func (s *Server) handleHistory(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	limit := 10
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	s.mu.RLock()
	budget := s.budgets["default"]
	s.mu.RUnlock()

	if budget == nil || len(budget.Transactions) == 0 {
		return textResult("No transaction history."), nil
	}

	result := "# Transaction History\n\n"
	result += "| Time | API | Amount | Status |\n"
	result += "|------|-----|--------|--------|\n"

	start := len(budget.Transactions) - limit
	if start < 0 {
		start = 0
	}

	for i := len(budget.Transactions) - 1; i >= start; i-- {
		tx := budget.Transactions[i]
		status := "✅"
		if !tx.Success {
			status = "❌"
		}
		result += fmt.Sprintf("| %s | %s | %d %s | %s |\n",
			tx.Timestamp.Format("15:04:05"),
			truncateURL(tx.API, 30),
			tx.Amount,
			tx.Currency,
			status,
		)
	}

	result += fmt.Sprintf("\n**Total Spent**: %d %s", budget.Spent, budget.Currency)

	return textResult(result), nil
}

// ============================================================================
// TRANSPORT: STDIO (for CLI usage)
// ============================================================================

// ListenStdio starts the server on stdin/stdout (standard MCP transport)
func (s *Server) ListenStdio() error {
	reader := bufio.NewReader(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendError(encoder, nil, ParseError, "Parse error")
			continue
		}

		s.handleRequest(encoder, &req)
	}
}

// ============================================================================
// TRANSPORT: HTTP (for web usage)
// ============================================================================

// ListenHTTP starts the server on HTTP
func (s *Server) ListenHTTP(addr string) error {
	http.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req JSONRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(JSONRPCResponse{
				JSONRPC: "2.0",
				Error:   &JSONRPCError{Code: ParseError, Message: "Parse error"},
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		s.handleRequest(json.NewEncoder(w), &req)
	})

	return http.ListenAndServe(addr, nil)
}

// ============================================================================
// REQUEST HANDLING
// ============================================================================

func (s *Server) handleRequest(encoder *json.Encoder, req *JSONRPCRequest) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(encoder, req)
	case "tools/list":
		s.handleToolsList(encoder, req)
	case "tools/call":
		s.handleToolsCall(encoder, req)
	default:
		s.sendError(encoder, req.ID, MethodNotFound, "Method not found")
	}
}

func (s *Server) handleInitialize(encoder *json.Encoder, req *JSONRPCRequest) {
	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"serverInfo": map[string]string{
			"name":    "x402-mcp-server",
			"version": "1.0.0",
		},
		"capabilities": map[string]interface{}{
			"tools": map[string]bool{},
		},
	}
	s.sendResult(encoder, req.ID, result)
}

func (s *Server) handleToolsList(encoder *json.Encoder, req *JSONRPCRequest) {
	result := map[string]interface{}{
		"tools": s.GetTools(),
	}
	s.sendResult(encoder, req.ID, result)
}

func (s *Server) handleToolsCall(encoder *json.Encoder, req *JSONRPCRequest) {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendError(encoder, req.ID, InvalidParams, "Invalid params")
		return
	}

	result, err := s.CallTool(context.Background(), params.Name, params.Arguments)
	if err != nil {
		s.sendError(encoder, req.ID, InternalError, err.Error())
		return
	}

	s.sendResult(encoder, req.ID, result)
}

func (s *Server) sendResult(encoder *json.Encoder, id interface{}, result interface{}) {
	_ = encoder.Encode(JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func (s *Server) sendError(encoder *json.Encoder, id interface{}, code int, message string) {
	_ = encoder.Encode(JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &JSONRPCError{Code: code, Message: message},
	})
}

// ============================================================================
// HELPERS
// ============================================================================

func (s *Server) formatDiscoveryResult(cache *APIDiscoveryCache) *ToolResult {
	result := fmt.Sprintf("# API Discovery: %s\n\n", cache.URL)
	result += "## Available Endpoints:\n\n"
	result += "| Endpoint | Method | Cost | Description |\n"
	result += "|----------|--------|------|-------------|\n"

	for _, ep := range cache.Endpoints {
		result += fmt.Sprintf("| %s | %s | %d %s | %s |\n",
			ep.Path, ep.Method, ep.Cost, ep.Currency, ep.Description)
	}

	return textResult(result)
}

func textResult(text string) *ToolResult {
	return &ToolResult{
		Content: []ContentBlock{{Type: "text", Text: text}},
	}
}

func errorResult(message string) *ToolResult {
	return &ToolResult{
		Content: []ContentBlock{{Type: "text", Text: "❌ Error: " + message}},
		IsError: true,
	}
}

func truncateURL(url string, max int) string {
	if len(url) <= max {
		return url
	}
	return url[:max-3] + "..."
}
