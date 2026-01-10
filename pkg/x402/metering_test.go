package x402

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMeteringStore_RecordAndRetrieve(t *testing.T) {
	store := NewInMemoryMeteringStore(1000, "USDC")

	// Record some metrics
	for i := 0; i < 10; i++ {
		metric := UsageMetric{
			Timestamp:    time.Now(),
			Endpoint:     "/api/test",
			Method:       "GET",
			PayerID:      "wallet_123",
			AmountPaid:   100,
			Currency:     "USDC",
			ResponseCode: 200,
			Latency:      50,
			PaymentType:  "per-request",
			IsAIAgent:    i%2 == 0, // Half are AI agents
		}
		err := store.RecordRequest(metric)
		if err != nil {
			t.Fatalf("Failed to record metric: %v", err)
		}
	}

	// Retrieve metrics
	report, err := store.GetMetrics(MetricsFilter{})
	if err != nil {
		t.Fatalf("Failed to get metrics: %v", err)
	}

	if report.TotalRequests != 10 {
		t.Errorf("Expected 10 total requests, got %d", report.TotalRequests)
	}

	if report.TotalRevenue != 1000 {
		t.Errorf("Expected 1000 total revenue, got %d", report.TotalRevenue)
	}

	if report.AIAgentRequests != 5 {
		t.Errorf("Expected 5 AI agent requests, got %d", report.AIAgentRequests)
	}

	if report.UniqueUsers != 1 {
		t.Errorf("Expected 1 unique user, got %d", report.UniqueUsers)
	}
}

func TestMeteringStore_FilterByAIAgent(t *testing.T) {
	store := NewInMemoryMeteringStore(1000, "USDC")

	// Record AI and non-AI requests
	store.RecordRequest(UsageMetric{
		Timestamp:  time.Now(),
		Endpoint:   "/api/test",
		AmountPaid: 100,
		IsAIAgent:  true,
	})
	store.RecordRequest(UsageMetric{
		Timestamp:  time.Now(),
		Endpoint:   "/api/test",
		AmountPaid: 100,
		IsAIAgent:  false,
	})

	// Filter by AI only
	report, _ := store.GetMetrics(MetricsFilter{AIAgentsOnly: true})

	if report.TotalRequests != 1 {
		t.Errorf("Expected 1 AI request, got %d", report.TotalRequests)
	}
}

func TestMeteringMiddleware(t *testing.T) {
	store := NewInMemoryMeteringStore(1000, "USDC")

	handler := MeteringMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		MeteringConfig{
			Store:           store,
			Currency:        "USDC",
			PricePerRequest: 100,
		},
	)

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Session-ID", "sess_123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	// Check metric was recorded
	report, _ := store.GetMetrics(MetricsFilter{})
	if report.TotalRequests != 1 {
		t.Errorf("Expected 1 request recorded, got %d", report.TotalRequests)
	}
}

func TestMetricsHandler(t *testing.T) {
	store := NewInMemoryMeteringStore(1000, "USDC")
	store.RecordRequest(UsageMetric{
		Timestamp:  time.Now(),
		Endpoint:   "/api/test",
		AmountPaid: 100,
	})

	handler := MetricsHandler(store)

	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	var report MetricsReport
	if err := json.Unmarshal(rr.Body.Bytes(), &report); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if report.TotalRequests != 1 {
		t.Errorf("Expected 1 request in report, got %d", report.TotalRequests)
	}
}

func TestMeteringStore_Eviction(t *testing.T) {
	// Create store with small max size
	store := NewInMemoryMeteringStore(5, "USDC")

	// Add more than max
	for i := 0; i < 10; i++ {
		store.RecordRequest(UsageMetric{
			Timestamp:  time.Now(),
			Endpoint:   "/api/test",
			AmountPaid: int64(i),
		})
	}

	// Should only have 5 entries
	report, _ := store.GetMetrics(MetricsFilter{})
	if report.TotalRequests != 5 {
		t.Errorf("Expected 5 requests after eviction, got %d", report.TotalRequests)
	}

	// Should have the last 5 entries (amounts 5-9)
	expectedRevenue := int64(5 + 6 + 7 + 8 + 9)
	if report.TotalRevenue != expectedRevenue {
		t.Errorf("Expected revenue %d, got %d", expectedRevenue, report.TotalRevenue)
	}
}
