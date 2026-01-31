package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
)

// ProviderPriority defines provider selection priority
type ProviderPriority int

const (
	PriorityPrimary ProviderPriority = iota
	PrioritySecondary
	PriorityTertiary
)

// ProviderConfig holds configuration for a registered provider
type ProviderConfig struct {
	Name           string
	Provider       Provider
	Enabled        bool
	Priority       ProviderPriority
	RateLimit      int // requests per second
	CircuitBreaker *CircuitBreaker
	SLA            SLAConfig
}

// SLAConfig defines SLA parameters for a provider
type SLAConfig struct {
	MaxLatencyP95Ms int     // Maximum acceptable P95 latency in ms
	MinSuccessRate  float64 // Minimum acceptable success rate (0.0-1.0)
}

// ProviderRegistry manages all payment and compliance providers
type ProviderRegistry struct {
	paymentProviders    map[string]*ProviderConfig
	complianceProviders map[string]*ComplianceProviderConfig
	mu                  sync.RWMutex
}

// ComplianceProviderConfig holds compliance provider configuration
type ComplianceProviderConfig struct {
	Name     string
	Provider ComplianceProvider
	Enabled  bool
}

// NewProviderRegistry creates a new provider registry
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		paymentProviders:    make(map[string]*ProviderConfig),
		complianceProviders: make(map[string]*ComplianceProviderConfig),
	}
}

// RegisterPaymentProvider adds a payment provider to the registry
func (pr *ProviderRegistry) RegisterPaymentProvider(config *ProviderConfig) error {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	if config.Provider == nil {
		return errors.New("provider cannot be nil")
	}

	name := config.Provider.Name()
	if name == "" {
		return errors.New("provider name cannot be empty")
	}

	// Create circuit breaker if not provided
	if config.CircuitBreaker == nil {
		cbConfig := DefaultCircuitBreakerConfig()
		config.CircuitBreaker = NewCircuitBreaker(name, cbConfig)
	}

	pr.paymentProviders[name] = config
	log.Printf("[ProviderRegistry] Registered payment provider: %s (priority: %d, enabled: %v)",
		name, config.Priority, config.Enabled)

	return nil
}

// RegisterComplianceProvider adds a compliance provider to the registry
func (pr *ProviderRegistry) RegisterComplianceProvider(config *ComplianceProviderConfig) error {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	if config.Provider == nil {
		return errors.New("compliance provider cannot be nil")
	}

	name := config.Provider.Name()
	if name == "" {
		return errors.New("compliance provider name cannot be empty")
	}

	pr.complianceProviders[name] = config
	log.Printf("[ProviderRegistry] Registered compliance provider: %s (enabled: %v)",
		name, config.Enabled)

	return nil
}

// GetPaymentProvider retrieves a payment provider by name
func (pr *ProviderRegistry) GetPaymentProvider(name string) (*ProviderConfig, error) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	config, exists := pr.paymentProviders[name]
	if !exists {
		return nil, fmt.Errorf("provider '%s' not found", name)
	}

	if !config.Enabled {
		return nil, fmt.Errorf("provider '%s' is disabled", name)
	}

	return config, nil
}

// GetComplianceProvider retrieves a compliance provider by name
func (pr *ProviderRegistry) GetComplianceProvider(name string) (*ComplianceProviderConfig, error) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	config, exists := pr.complianceProviders[name]
	if !exists {
		return nil, fmt.Errorf("compliance provider '%s' not found", name)
	}

	if !config.Enabled {
		return nil, fmt.Errorf("compliance provider '%s' is disabled", name)
	}

	return config, nil
}

// GetEligiblePaymentProviders returns providers matching requirements
func (pr *ProviderRegistry) GetEligiblePaymentProviders(req *PaymentRequest) ([]*ProviderConfig, error) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	eligible := make([]*ProviderConfig, 0)

	for _, config := range pr.paymentProviders {
		if !config.Enabled {
			continue
		}

		// Check circuit breaker state
		if config.CircuitBreaker.GetState() == StateOpen {
			log.Printf("[ProviderRegistry] Skipping %s: circuit breaker is OPEN", config.Name)
			continue
		}

		// Check capabilities
		caps := config.Provider.Capabilities()

		// Check amount limits
		if req.Amount < caps.MinAmountCents || req.Amount > caps.MaxAmountCents {
			log.Printf("[ProviderRegistry] Skipping %s: amount %d outside limits [%d, %d]",
				config.Name, req.Amount, caps.MinAmountCents, caps.MaxAmountCents)
			continue
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
			log.Printf("[ProviderRegistry] Skipping %s: currency %s not supported",
				config.Name, req.Currency)
			continue
		}

		eligible = append(eligible, config)
	}

	if len(eligible) == 0 {
		return nil, errors.New("no eligible providers found for this request")
	}

	// Sort by priority
	pr.sortByPriority(eligible)

	return eligible, nil
}

// sortByPriority sorts providers by priority (primary first)
func (pr *ProviderRegistry) sortByPriority(providers []*ProviderConfig) {
	// Simple bubble sort by priority
	n := len(providers)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if providers[j].Priority > providers[j+1].Priority {
				providers[j], providers[j+1] = providers[j+1], providers[j]
			}
		}
	}
}

// EnableProvider enables a provider
func (pr *ProviderRegistry) EnableProvider(name string) error {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	config, exists := pr.paymentProviders[name]
	if !exists {
		return fmt.Errorf("provider '%s' not found", name)
	}

	config.Enabled = true
	log.Printf("[ProviderRegistry] Enabled provider: %s", name)
	return nil
}

// DisableProvider disables a provider
func (pr *ProviderRegistry) DisableProvider(name string) error {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	config, exists := pr.paymentProviders[name]
	if !exists {
		return fmt.Errorf("provider '%s' not found", name)
	}

	config.Enabled = false
	log.Printf("[ProviderRegistry] Disabled provider: %s", name)
	return nil
}

// GetAllProviderStatus returns status of all providers
func (pr *ProviderRegistry) GetAllProviderStatus() map[string]interface{} {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	paymentStatus := make([]map[string]interface{}, 0)
	for name, config := range pr.paymentProviders {
		status := map[string]interface{}{
			"name":            name,
			"enabled":         config.Enabled,
			"priority":        config.Priority,
			"circuit_breaker": config.CircuitBreaker.GetStats(),
			"capabilities":    config.Provider.Capabilities(),
		}
		paymentStatus = append(paymentStatus, status)
	}

	complianceStatus := make([]map[string]interface{}, 0)
	for name, config := range pr.complianceProviders {
		status := map[string]interface{}{
			"name":    name,
			"enabled": config.Enabled,
		}
		complianceStatus = append(complianceStatus, status)
	}

	return map[string]interface{}{
		"payment_providers":    paymentStatus,
		"compliance_providers": complianceStatus,
	}
}

// PerformComplianceCheck executes compliance checks for high-risk transactions
func (pr *ProviderRegistry) PerformComplianceCheck(ctx context.Context, req *ComplianceCheckRequest) (*ComplianceCheckResponse, error) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	// Try to find an enabled compliance provider
	for _, config := range pr.complianceProviders {
		if !config.Enabled {
			continue
		}

		switch req.CheckType {
		case ComplianceCheckKYC:
			return config.Provider.CheckKYC(ctx, req)
		case ComplianceCheckAML:
			return config.Provider.CheckAML(ctx, req)
		default:
			return nil, fmt.Errorf("unknown compliance check type: %s", req.CheckType)
		}
	}

	return nil, errors.New("no enabled compliance providers available")
}
