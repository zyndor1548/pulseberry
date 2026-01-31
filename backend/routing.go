package main

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"
)

// RoutingStrategy defines how providers are selected
type RoutingStrategy string

const (
	RoutingStrategyPriority     RoutingStrategy = "priority"      // Use provider priority (existing)
	RoutingStrategyLeastLatency RoutingStrategy = "least_latency" // Select provider with lowest P95 latency
	RoutingStrategyHealthScore  RoutingStrategy = "health_score"  // Select based on composite health score
	RoutingStrategyAffinity     RoutingStrategy = "affinity"      // Stick to same provider for user
	RoutingStrategyRoundRobin   RoutingStrategy = "round_robin"   // Distribute evenly
)

// ProviderSelector handles intelligent provider selection
type ProviderSelector struct {
	registry *ProviderRegistry
	strategy RoutingStrategy
	rdb      *redis.Client
}

// NewProviderSelector creates a new provider selector
func NewProviderSelector(registry *ProviderRegistry, strategy RoutingStrategy, rdb *redis.Client) *ProviderSelector {
	return &ProviderSelector{
		registry: registry,
		strategy: strategy,
		rdb:      rdb,
	}
}

// SelectProvider selects the best provider for a payment request
func (ps *ProviderSelector) SelectProvider(ctx context.Context, req *PaymentRequest) (*ProviderConfig, error) {
	switch ps.strategy {
	case RoutingStrategyLeastLatency:
		return ps.selectByLeastLatency(req)
	case RoutingStrategyHealthScore:
		return ps.selectByHealthScore(req)
	case RoutingStrategyAffinity:
		return ps.selectByAffinity(ctx, req)
	case RoutingStrategyRoundRobin:
		return ps.selectRoundRobin(req)
	case RoutingStrategyPriority:
		fallthrough
	default:
		return ps.selectByPriority(req)
	}
}

// selectByPriority selects provider based on priority (existing logic)
func (ps *ProviderSelector) selectByPriority(req *PaymentRequest) (*ProviderConfig, error) {
	eligible, err := ps.registry.GetEligiblePaymentProviders(req)
	if err != nil {
		return nil, err
	}

	if len(eligible) == 0 {
		return nil, fmt.Errorf("no eligible providers for request")
	}

	// Return first provider (highest priority)
	return eligible[0], nil
}

// selectByLeastLatency selects provider with lowest P95 latency
func (ps *ProviderSelector) selectByLeastLatency(req *PaymentRequest) (*ProviderConfig, error) {
	eligible, err := ps.registry.GetEligiblePaymentProviders(req)
	if err != nil {
		return nil, err
	}

	if len(eligible) == 0 {
		return nil, fmt.Errorf("no eligible providers for request")
	}

	// Sort by P95 latency (ascending)
	sort.Slice(eligible, func(i, j int) bool {
		latencyI := ps.getProviderLatencyP95(eligible[i])
		latencyJ := ps.getProviderLatencyP95(eligible[j])
		return latencyI < latencyJ
	})

	return eligible[0], nil
}

// selectByHealthScore selects provider based on composite health score
func (ps *ProviderSelector) selectByHealthScore(req *PaymentRequest) (*ProviderConfig, error) {
	eligible, err := ps.registry.GetEligiblePaymentProviders(req)
	if err != nil {
		return nil, err
	}

	if len(eligible) == 0 {
		return nil, fmt.Errorf("no eligible providers for request")
	}

	// Calculate health score for each provider
	type providerScore struct {
		config *ProviderConfig
		score  float64
	}

	scores := make([]providerScore, 0, len(eligible))
	for _, config := range eligible {
		score := ps.calculateHealthScore(config)
		scores = append(scores, providerScore{
			config: config,
			score:  score,
		})
	}

	// Sort by score (descending - higher is better)
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	return scores[0].config, nil
}

// selectByAffinity selects provider with affinity to user
func (ps *ProviderSelector) selectByAffinity(ctx context.Context, req *PaymentRequest) (*ProviderConfig, error) {
	// Get affinity from Redis (if exists)
	if req.UserID != "" {
		affinityKey := fmt.Sprintf("provider_affinity:%s", req.UserID)
		providerName, err := ps.rdb.Get(ctx, affinityKey).Result()

		if err == nil && providerName != "" {
			// Try to use affinity provider
			config, err := ps.registry.GetPaymentProvider(providerName)
			if err == nil && config.Enabled {
				// Check if provider supports this request
				if ps.isProviderEligible(config, req) {
					return config, nil
				}
			}
		}
	}

	// No affinity or affinity provider unavailable - fall back to health score
	config, err := ps.selectByHealthScore(req)
	if err != nil {
		return nil, err
	}

	// Store affinity for next time
	if req.UserID != "" {
		affinityKey := fmt.Sprintf("provider_affinity:%s", req.UserID)
		ps.rdb.Set(ctx, affinityKey, config.Provider.Name(), 24*time.Hour)
	}

	return config, nil
}

// selectRoundRobin distributes requests evenly across providers
func (ps *ProviderSelector) selectRoundRobin(req *PaymentRequest) (*ProviderConfig, error) {
	eligible, err := ps.registry.GetEligiblePaymentProviders(req)
	if err != nil {
		return nil, err
	}

	if len(eligible) == 0 {
		return nil, fmt.Errorf("no eligible providers for request")
	}

	// Use request hash to distribute evenly
	hash := hashString(req.IdempotencyKey)
	index := int(hash) % len(eligible)

	return eligible[index], nil
}

// calculateHealthScore computes composite health score for a provider
func (ps *ProviderSelector) calculateHealthScore(config *ProviderConfig) float64 {
	// Get provider metrics (would come from ServerMetrics in real implementation)
	successRate := ps.getProviderSuccessRate(config)
	latencyScore := ps.getProviderLatencyScore(config)
	availabilityScore := ps.getProviderAvailabilityScore(config)

	// Weighted composite score
	// successRate: 40%, latencyScore: 30%, availabilityScore: 30%
	healthScore := (successRate * 0.4) +
		(latencyScore * 0.3) +
		(availabilityScore * 0.3)

	return healthScore
}

// getProviderSuccessRate returns success rate for a provider (0.0 to 1.0)
func (ps *ProviderSelector) getProviderSuccessRate(config *ProviderConfig) float64 {
	// In real implementation, this would query ServerMetrics
	// For now, return a default high score
	return 0.95
}

// getProviderLatencyScore returns latency score for a provider (0.0 to 1.0)
func (ps *ProviderSelector) getProviderLatencyScore(config *ProviderConfig) float64 {
	p95 := ps.getProviderLatencyP95(config)

	// Convert latency to score (inverse relationship)
	// < 100ms = 1.0, 500ms = 0.5, > 1000ms = 0.0
	if p95 < 100 {
		return 1.0
	} else if p95 > 1000 {
		return 0.0
	}

	// Linear interpolation between 100ms and 1000ms
	score := 1.0 - ((float64(p95) - 100.0) / 900.0)
	return score
}

// getProviderAvailabilityScore returns availability score based on circuit breaker state
func (ps *ProviderSelector) getProviderAvailabilityScore(config *ProviderConfig) float64 {
	if config.CircuitBreaker == nil {
		return 1.0 // No circuit breaker, assume available
	}

	state := config.CircuitBreaker.GetState()
	switch state {
	case StateClosed:
		return 1.0
	case StateHalfOpen:
		return 0.5
	case StateOpen:
		return 0.0
	default:
		return 0.0
	}
}

// getProviderLatencyP95 returns P95 latency for a provider in milliseconds
func (ps *ProviderSelector) getProviderLatencyP95(config *ProviderConfig) int64 {
	// In real implementation, this would query ServerMetrics
	// For now, return SLA threshold or default
	if config.SLA.MaxLatencyP95Ms > 0 {
		return int64(config.SLA.MaxLatencyP95Ms)
	}
	return 500 // Default 500ms
}

// isProviderEligible checks if a provider can handle this request
func (ps *ProviderSelector) isProviderEligible(config *ProviderConfig, req *PaymentRequest) bool {
	if !config.Enabled {
		return false
	}

	caps := config.Provider.Capabilities()

	// Check amount limits
	if req.Amount < caps.MinAmountCents || req.Amount > caps.MaxAmountCents {
		return false
	}

	// Check currency support
	currencySupported := false
	for _, curr := range caps.SupportedCurrencies {
		if curr == req.Currency {
			currencySupported = true
			break
		}
	}

	if !currencySupported {
		return false
	}

	// Check circuit breaker state
	if config.CircuitBreaker != nil {
		if config.CircuitBreaker.GetState() == StateOpen {
			return false
		}
	}

	return true
}

// hashString creates a simple hash of a string
func hashString(s string) uint32 {
	h := uint32(0)
	for _, c := range s {
		h = h*31 + uint32(c)
	}
	return h
}

// GetRoutingReason returns human-readable reason for routing decision
func (ps *ProviderSelector) GetRoutingReason(config *ProviderConfig, req *PaymentRequest) string {
	switch ps.strategy {
	case RoutingStrategyLeastLatency:
		p95 := ps.getProviderLatencyP95(config)
		return fmt.Sprintf("least_latency (P95: %dms)", p95)
	case RoutingStrategyHealthScore:
		score := ps.calculateHealthScore(config)
		return fmt.Sprintf("health_score (score: %.2f)", score)
	case RoutingStrategyAffinity:
		return "user_affinity"
	case RoutingStrategyRoundRobin:
		return "round_robin"
	case RoutingStrategyPriority:
		return fmt.Sprintf("priority_%d", config.Priority)
	default:
		return "default"
	}
}
