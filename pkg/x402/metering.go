// Package x402 - Usage Metering & Analytics
// Tracks API usage, costs, and revenue per endpoint with real-time metrics.
package x402

import (
	"encoding/json"
	"net/http"
	"sort"
	"sync"
	"time"
)

// MeteringStore defines the interface for storing usage metrics
type MeteringStore interface {
	RecordRequest(metric UsageMetric) error
	GetMetrics(filter MetricsFilter) (*MetricsReport, error)
	GetEndpointStats() ([]EndpointStats, error)
}

// UsageMetric represents a single API usage event
type UsageMetric struct {
	Timestamp    time.Time `json:"timestamp"`
	Endpoint     string    `json:"endpoint"`
	Method       string    `json:"method"`
	PayerID      string    `json:"payerId,omitempty"` // Wallet address or session ID
	AmountPaid   int64     `json:"amountPaid"`        // In smallest currency unit
	Currency     string    `json:"currency"`
	ResponseCode int       `json:"responseCode"`
	Latency      int64     `json:"latencyMs"`   // Response time in milliseconds
	PaymentType  string    `json:"paymentType"` // "per-request", "session", "subscription"
	SessionID    string    `json:"sessionId,omitempty"`
	UserAgent    string    `json:"userAgent,omitempty"`
	IsAIAgent    bool      `json:"isAiAgent"` // Detected AI agent request
}

// MetricsFilter for querying metrics
type MetricsFilter struct {
	StartTime    *time.Time `json:"startTime,omitempty"`
	EndTime      *time.Time `json:"endTime,omitempty"`
	Endpoint     string     `json:"endpoint,omitempty"`
	PayerID      string     `json:"payerId,omitempty"`
	PaymentType  string     `json:"paymentType,omitempty"`
	AIAgentsOnly bool       `json:"aiAgentsOnly,omitempty"`
}

// MetricsReport contains aggregated metrics
type MetricsReport struct {
	Period          string          `json:"period"`
	TotalRequests   int64           `json:"totalRequests"`
	TotalRevenue    int64           `json:"totalRevenue"`
	Currency        string          `json:"currency"`
	UniqueUsers     int64           `json:"uniqueUsers"`
	AvgLatencyMs    float64         `json:"avgLatencyMs"`
	RequestsByHour  map[int]int64   `json:"requestsByHour"`
	RevenueByHour   map[int]int64   `json:"revenueByHour"`
	TopEndpoints    []EndpointStats `json:"topEndpoints"`
	TopPayers       []PayerStats    `json:"topPayers"`
	AIAgentRequests int64           `json:"aiAgentRequests"`
	AIAgentRevenue  int64           `json:"aiAgentRevenue"`
	ErrorRate       float64         `json:"errorRate"`
}

// EndpointStats contains per-endpoint metrics
type EndpointStats struct {
	Endpoint      string  `json:"endpoint"`
	TotalRequests int64   `json:"totalRequests"`
	TotalRevenue  int64   `json:"totalRevenue"`
	AvgLatencyMs  float64 `json:"avgLatencyMs"`
	ErrorRate     float64 `json:"errorRate"`
	UniqueUsers   int64   `json:"uniqueUsers"`
}

// PayerStats contains per-payer metrics
type PayerStats struct {
	PayerID       string `json:"payerId"`
	TotalRequests int64  `json:"totalRequests"`
	TotalSpent    int64  `json:"totalSpent"`
	LastSeen      string `json:"lastSeen"`
	IsAIAgent     bool   `json:"isAiAgent"`
}

// InMemoryMeteringStore is a simple in-memory implementation
type InMemoryMeteringStore struct {
	mu       sync.RWMutex
	metrics  []UsageMetric
	maxSize  int
	currency string
}

// NewInMemoryMeteringStore creates a new in-memory metering store
func NewInMemoryMeteringStore(maxSize int, currency string) *InMemoryMeteringStore {
	if maxSize <= 0 {
		maxSize = 100000 // Default 100k entries
	}
	if currency == "" {
		currency = "USDC"
	}
	return &InMemoryMeteringStore{
		metrics:  make([]UsageMetric, 0, maxSize),
		maxSize:  maxSize,
		currency: currency,
	}
}

// RecordRequest records a usage metric
func (s *InMemoryMeteringStore) RecordRequest(metric UsageMetric) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Evict oldest entries if at capacity
	if len(s.metrics) >= s.maxSize {
		s.metrics = s.metrics[1:]
	}

	s.metrics = append(s.metrics, metric)
	return nil
}

// GetMetrics retrieves aggregated metrics based on filter
func (s *InMemoryMeteringStore) GetMetrics(filter MetricsFilter) (*MetricsReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	report := &MetricsReport{
		Period:         "custom",
		Currency:       s.currency,
		RequestsByHour: make(map[int]int64),
		RevenueByHour:  make(map[int]int64),
	}

	uniqueUsers := make(map[string]bool)
	endpointStats := make(map[string]*EndpointStats)
	payerStats := make(map[string]*PayerStats)
	var totalLatency int64
	var errorCount int64

	for _, m := range s.metrics {
		// Apply filters
		if filter.StartTime != nil && m.Timestamp.Before(*filter.StartTime) {
			continue
		}
		if filter.EndTime != nil && m.Timestamp.After(*filter.EndTime) {
			continue
		}
		if filter.Endpoint != "" && m.Endpoint != filter.Endpoint {
			continue
		}
		if filter.PayerID != "" && m.PayerID != filter.PayerID {
			continue
		}
		if filter.PaymentType != "" && m.PaymentType != filter.PaymentType {
			continue
		}
		if filter.AIAgentsOnly && !m.IsAIAgent {
			continue
		}

		// Aggregate
		report.TotalRequests++
		report.TotalRevenue += m.AmountPaid
		totalLatency += m.Latency

		hour := m.Timestamp.Hour()
		report.RequestsByHour[hour]++
		report.RevenueByHour[hour] += m.AmountPaid

		if m.PayerID != "" {
			uniqueUsers[m.PayerID] = true
		}

		if m.IsAIAgent {
			report.AIAgentRequests++
			report.AIAgentRevenue += m.AmountPaid
		}

		if m.ResponseCode >= 400 {
			errorCount++
		}

		// Endpoint stats
		if _, ok := endpointStats[m.Endpoint]; !ok {
			endpointStats[m.Endpoint] = &EndpointStats{Endpoint: m.Endpoint}
		}
		es := endpointStats[m.Endpoint]
		es.TotalRequests++
		es.TotalRevenue += m.AmountPaid
		es.AvgLatencyMs = (es.AvgLatencyMs*float64(es.TotalRequests-1) + float64(m.Latency)) / float64(es.TotalRequests)
		if m.ResponseCode >= 400 {
			es.ErrorRate = float64(errorCount) / float64(es.TotalRequests)
		}

		// Payer stats
		if m.PayerID != "" {
			if _, ok := payerStats[m.PayerID]; !ok {
				payerStats[m.PayerID] = &PayerStats{PayerID: m.PayerID}
			}
			ps := payerStats[m.PayerID]
			ps.TotalRequests++
			ps.TotalSpent += m.AmountPaid
			ps.LastSeen = m.Timestamp.Format(time.RFC3339)
			ps.IsAIAgent = m.IsAIAgent
		}
	}

	report.UniqueUsers = int64(len(uniqueUsers))
	if report.TotalRequests > 0 {
		report.AvgLatencyMs = float64(totalLatency) / float64(report.TotalRequests)
		report.ErrorRate = float64(errorCount) / float64(report.TotalRequests)
	}

	// Convert maps to sorted slices
	for _, es := range endpointStats {
		report.TopEndpoints = append(report.TopEndpoints, *es)
	}
	sort.Slice(report.TopEndpoints, func(i, j int) bool {
		return report.TopEndpoints[i].TotalRevenue > report.TopEndpoints[j].TotalRevenue
	})
	if len(report.TopEndpoints) > 10 {
		report.TopEndpoints = report.TopEndpoints[:10]
	}

	for _, ps := range payerStats {
		report.TopPayers = append(report.TopPayers, *ps)
	}
	sort.Slice(report.TopPayers, func(i, j int) bool {
		return report.TopPayers[i].TotalSpent > report.TopPayers[j].TotalSpent
	})
	if len(report.TopPayers) > 10 {
		report.TopPayers = report.TopPayers[:10]
	}

	return report, nil
}

// GetEndpointStats returns stats for all endpoints
func (s *InMemoryMeteringStore) GetEndpointStats() ([]EndpointStats, error) {
	report, err := s.GetMetrics(MetricsFilter{})
	if err != nil {
		return nil, err
	}
	return report.TopEndpoints, nil
}

// MeteringConfig configures the metering middleware
type MeteringConfig struct {
	Store           MeteringStore
	Currency        string
	PricePerRequest int64
}

// MeteringMiddleware wraps a handler with usage metering
func MeteringMiddleware(next http.Handler, config MeteringConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseRecorder{ResponseWriter: w, statusCode: 200}

		next.ServeHTTP(wrapped, r)

		// Record metric
		metric := UsageMetric{
			Timestamp:    start,
			Endpoint:     r.URL.Path,
			Method:       r.Method,
			PayerID:      extractPayerID(r),
			AmountPaid:   config.PricePerRequest,
			Currency:     config.Currency,
			ResponseCode: wrapped.statusCode,
			Latency:      time.Since(start).Milliseconds(),
			PaymentType:  detectPaymentType(r),
			SessionID:    r.Header.Get("X-Session-ID"),
			UserAgent:    r.UserAgent(),
			IsAIAgent:    isAIAgent(r),
		}

		if config.Store != nil {
			_ = config.Store.RecordRequest(metric)
		}
	})
}

// responseRecorder captures the response status code
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.statusCode = code
	rr.ResponseWriter.WriteHeader(code)
}

// extractPayerID extracts the payer identifier from the request
func extractPayerID(r *http.Request) string {
	// Check for wallet address in payment headers
	if payer := r.Header.Get("X-Payer-Address"); payer != "" {
		return payer
	}
	// Check session
	if session := r.Header.Get("X-Session-ID"); session != "" {
		return "session:" + session
	}
	// Check API key
	if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
		// Hash or truncate for privacy
		if len(apiKey) > 8 {
			return "key:" + apiKey[:8] + "..."
		}
		return "key:" + apiKey
	}
	return ""
}

// detectPaymentType determines the payment type from headers
func detectPaymentType(r *http.Request) string {
	if r.Header.Get("X-Session-ID") != "" {
		return "session"
	}
	if r.Header.Get("X-Subscription-ID") != "" {
		return "subscription"
	}
	return "per-request"
}

// MetricsHandler returns an HTTP handler for the metrics endpoint
func MetricsHandler(store MeteringStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		filter := MetricsFilter{}

		// Parse query params
		if start := r.URL.Query().Get("start"); start != "" {
			if t, err := time.Parse(time.RFC3339, start); err == nil {
				filter.StartTime = &t
			}
		}
		if end := r.URL.Query().Get("end"); end != "" {
			if t, err := time.Parse(time.RFC3339, end); err == nil {
				filter.EndTime = &t
			}
		}
		filter.Endpoint = r.URL.Query().Get("endpoint")
		filter.PayerID = r.URL.Query().Get("payer")
		filter.PaymentType = r.URL.Query().Get("paymentType")
		filter.AIAgentsOnly = r.URL.Query().Get("aiOnly") == "true"

		report, err := store.GetMetrics(filter)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(report)
	}
}
