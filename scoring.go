package main

import (
	"math"
	"sync"
	"time"
)

type ErrorType int

const (
	ErrorTypeGateway ErrorType = iota
	ErrorTypeBank
	ErrorTypeNetwork
	ErrorTypeClient
)

type ServerMetrics struct {
	ServerURL string
	Score     float64

	// Request metrics
	TotalRequests   int64
	SuccessRequests int64
	FailedRequests  int64

	// Latency tracking
	TotalLatency time.Duration
	AvgLatency   time.Duration
	MinLatency   time.Duration
	MaxLatency   time.Duration

	// Error counts
	GatewayErrors []ErrorEvent
	BankErrors    []ErrorEvent
	NetworkErrors []ErrorEvent
	ClientErrors  []ErrorEvent

	ActiveConnections int
	QueueDepth        int

	LastUpdated time.Time
	LastRequest time.Time

	mu sync.RWMutex
}

type ErrorEvent struct {
	Timestamp time.Time
	Message   string
}

type ScoringConfig struct {
	BaseScore            float64
	LatencyThresholdLow  time.Duration
	LatencyThresholdMed  time.Duration
	LatencyThresholdHigh time.Duration
	LatencyPenaltyLow    float64
	LatencyPenaltyMed    float64
	LatencyPenaltyHigh   float64

	GatewayErrorPenalty float64
	BankErrorPenalty    float64
	NetworkErrorPenalty float64
	ClientErrorPenalty  float64

	HighLoadThreshold int
	LoadPenalty       float64

	ErrorDecayWindow time.Duration

	RecoveryRate      float64
	MinScore          float64
	MaxScore          float64
	ScoreUpdatePeriod time.Duration
}

func DefaultScoringConfig() *ScoringConfig {
	return &ScoringConfig{
		BaseScore:            100.0,
		LatencyThresholdLow:  100 * time.Millisecond,
		LatencyThresholdMed:  500 * time.Millisecond,
		LatencyThresholdHigh: 1000 * time.Millisecond,
		LatencyPenaltyLow:    5.0,
		LatencyPenaltyMed:    15.0,
		LatencyPenaltyHigh:   30.0,
		GatewayErrorPenalty:  10.0,
		BankErrorPenalty:     5.0,
		NetworkErrorPenalty:  15.0,
		ClientErrorPenalty:   2.0,
		HighLoadThreshold:    50,
		LoadPenalty:          20.0,
		ErrorDecayWindow:     5 * time.Minute,
		RecoveryRate:         0.5,
		MinScore:             0.0,
		MaxScore:             100.0,
		ScoreUpdatePeriod:    10 * time.Second,
	}
}

func NewServerMetrics(serverURL string) *ServerMetrics {
	return &ServerMetrics{
		ServerURL:     serverURL,
		Score:         100.0,
		MinLatency:    time.Duration(math.MaxInt64),
		GatewayErrors: make([]ErrorEvent, 0),
		BankErrors:    make([]ErrorEvent, 0),
		NetworkErrors: make([]ErrorEvent, 0),
		ClientErrors:  make([]ErrorEvent, 0),
		LastUpdated:   time.Now(),
	}
}

func (sm *ServerMetrics) RecordRequest(latency time.Duration, success bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.TotalRequests++
	sm.LastRequest = time.Now()

	if success {
		sm.SuccessRequests++
	} else {
		sm.FailedRequests++
	}

	sm.TotalLatency += latency
	sm.AvgLatency = time.Duration(int64(sm.TotalLatency) / sm.TotalRequests)

	if latency < sm.MinLatency {
		sm.MinLatency = latency
	}
	if latency > sm.MaxLatency {
		sm.MaxLatency = latency
	}
}

func (sm *ServerMetrics) RecordError(errorType ErrorType, message string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	event := ErrorEvent{
		Timestamp: time.Now(),
		Message:   message,
	}

	switch errorType {
	case ErrorTypeGateway:
		sm.GatewayErrors = append(sm.GatewayErrors, event)
	case ErrorTypeBank:
		sm.BankErrors = append(sm.BankErrors, event)
	case ErrorTypeNetwork:
		sm.NetworkErrors = append(sm.NetworkErrors, event)
	case ErrorTypeClient:
		sm.ClientErrors = append(sm.ClientErrors, event)
	}
}

func (sm *ServerMetrics) UpdateActiveConnections(count int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.ActiveConnections = count
}

func (sm *ServerMetrics) GetScore() float64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.Score
}

func (sm *ServerMetrics) GetMetricsSummary() map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	successRate := 0.0
	if sm.TotalRequests > 0 {
		successRate = float64(sm.SuccessRequests) / float64(sm.TotalRequests) * 100
	}

	return map[string]interface{}{
		"server_url":         sm.ServerURL,
		"score":              sm.Score,
		"total_requests":     sm.TotalRequests,
		"success_rate":       successRate,
		"avg_latency_ms":     sm.AvgLatency.Milliseconds(),
		"gateway_errors":     len(sm.GatewayErrors),
		"bank_errors":        len(sm.BankErrors),
		"network_errors":     len(sm.NetworkErrors),
		"active_connections": sm.ActiveConnections,
		"last_updated":       sm.LastUpdated.Format(time.RFC3339),
	}
}

func (sm *ServerMetrics) cleanOldErrors(decayWindow time.Duration) {
	now := time.Now()
	cutoff := now.Add(-decayWindow)

	sm.GatewayErrors = filterErrors(sm.GatewayErrors, cutoff)
	sm.BankErrors = filterErrors(sm.BankErrors, cutoff)
	sm.NetworkErrors = filterErrors(sm.NetworkErrors, cutoff)
	sm.ClientErrors = filterErrors(sm.ClientErrors, cutoff)
}

func filterErrors(errors []ErrorEvent, cutoff time.Time) []ErrorEvent {
	filtered := make([]ErrorEvent, 0)
	for _, err := range errors {
		if err.Timestamp.After(cutoff) {
			filtered = append(filtered, err)
		}
	}
	return filtered
}

func (sm *ServerMetrics) CalculateScore(config *ScoringConfig) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.cleanOldErrors(config.ErrorDecayWindow)

	score := config.BaseScore

	if sm.AvgLatency >= config.LatencyThresholdHigh {
		score -= config.LatencyPenaltyHigh
	} else if sm.AvgLatency >= config.LatencyThresholdMed {
		score -= config.LatencyPenaltyMed
	} else if sm.AvgLatency >= config.LatencyThresholdLow {
		score -= config.LatencyPenaltyLow
	}

	score -= float64(len(sm.GatewayErrors)) * config.GatewayErrorPenalty
	score -= float64(len(sm.BankErrors)) * config.BankErrorPenalty
	score -= float64(len(sm.NetworkErrors)) * config.NetworkErrorPenalty
	score -= float64(len(sm.ClientErrors)) * config.ClientErrorPenalty

	if sm.ActiveConnections >= config.HighLoadThreshold {
		loadFactor := float64(sm.ActiveConnections-config.HighLoadThreshold) / float64(config.HighLoadThreshold)
		score -= config.LoadPenalty * math.Min(loadFactor, 1.0)
	}

	if score < config.MinScore {
		score = config.MinScore
	}
	if score > config.MaxScore {
		score = config.MaxScore
	}

	sm.Score = score
	sm.LastUpdated = time.Now()
}
