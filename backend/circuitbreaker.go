package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker
type CircuitState int

const (
	StateClosed CircuitState = iota
	StateOpen
	StateHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF_OPEN"
	default:
		return "UNKNOWN"
	}
}

// CircuitBreakerConfig holds configuration for circuit breaker
type CircuitBreakerConfig struct {
	FailureThreshold    int           // Number of consecutive failures before opening
	ErrorRateThreshold  float64       // Error rate (0.0-1.0) over window before opening
	WindowDuration      time.Duration // Duration for error rate calculation
	CooldownPeriod      time.Duration // How long to wait in OPEN before transitioning to HALF_OPEN
	HalfOpenMaxRequests int           // Number of successful requests in HALF_OPEN before CLOSED
}

// DefaultCircuitBreakerConfig returns production-ready defaults
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold:    10,               // 10 consecutive failures
		ErrorRateThreshold:  0.5,              // 50% error rate
		WindowDuration:      60 * time.Second, // 1 minute window
		CooldownPeriod:      30 * time.Second, // 30 second cooldown
		HalfOpenMaxRequests: 5,                // 5 successful probes
	}
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	name            string
	state           CircuitState
	failureCount    int
	successCount    int
	totalRequests   int
	errorCount      int
	lastStateChange time.Time
	lastError       error
	mu              sync.RWMutex
	config          CircuitBreakerConfig
	requestHistory  []requestRecord
}

type requestRecord struct {
	timestamp time.Time
	success   bool
}

// NewCircuitBreaker creates a new circuit breaker with given config
func NewCircuitBreaker(name string, config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		name:            name,
		state:           StateClosed,
		config:          config,
		lastStateChange: time.Now(),
		requestHistory:  make([]requestRecord, 0),
	}
}

// Execute runs the given function with circuit breaker protection
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	// Check if we can proceed
	if err := cb.beforeRequest(); err != nil {
		return err
	}

	// Execute the function
	err := fn()

	// Record the result
	cb.afterRequest(err)

	return err
}

// beforeRequest checks if the request should be allowed
func (cb *CircuitBreaker) beforeRequest() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateOpen:
		// Check if cooldown period has elapsed
		if time.Since(cb.lastStateChange) > cb.config.CooldownPeriod {
			cb.transitionTo(StateHalfOpen)
			log.Printf("[CircuitBreaker:%s] Transitioning to HALF_OPEN after cooldown", cb.name)
			return nil
		}
		// Return a properly formatted error
		return fmt.Errorf("circuit breaker is open: %s", cb.name)

	case StateHalfOpen:
		// Allow limited requests in half-open state
		return nil

	case StateClosed:
		return nil

	default:
		return nil
	}
}

// afterRequest records the result and potentially changes state
func (cb *CircuitBreaker) afterRequest(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Record in history
	record := requestRecord{
		timestamp: time.Now(),
		success:   err == nil,
	}
	cb.requestHistory = append(cb.requestHistory, record)
	cb.cleanOldHistory()

	cb.totalRequests++

	if err != nil {
		cb.errorCount++
		cb.failureCount++
		cb.successCount = 0 // Reset consecutive success count
		cb.lastError = err

		switch cb.state {
		case StateClosed:
			// Check if we should open the circuit
			if cb.shouldOpen() {
				cb.transitionTo(StateOpen)
				log.Printf("[CircuitBreaker:%s] Opening circuit: %d consecutive failures, error rate: %.2f%%",
					cb.name, cb.failureCount, cb.calculateErrorRate()*100)
			}

		case StateHalfOpen:
			// Any failure in half-open state reopens the circuit
			cb.transitionTo(StateOpen)
			log.Printf("[CircuitBreaker:%s] Reopening circuit after failure in HALF_OPEN state", cb.name)
		}
	} else {
		// Success
		cb.failureCount = 0 // Reset consecutive failure count
		cb.successCount++

		switch cb.state {
		case StateHalfOpen:
			// Check if we should close the circuit
			if cb.successCount >= cb.config.HalfOpenMaxRequests {
				cb.transitionTo(StateClosed)
				log.Printf("[CircuitBreaker:%s] Closing circuit after %d successful probes", cb.name, cb.successCount)
			}
		}
	}
}

// shouldOpen determines if the circuit should open based on failures
func (cb *CircuitBreaker) shouldOpen() bool {
	// Check consecutive failures
	if cb.failureCount >= cb.config.FailureThreshold {
		return true
	}

	// Check error rate over window
	errorRate := cb.calculateErrorRate()
	if errorRate >= cb.config.ErrorRateThreshold && cb.totalRequests >= 10 {
		return true
	}

	return false
}

// calculateErrorRate computes error rate over the configured window
func (cb *CircuitBreaker) calculateErrorRate() float64 {
	if len(cb.requestHistory) == 0 {
		return 0.0
	}

	windowStart := time.Now().Add(-cb.config.WindowDuration)
	totalInWindow := 0
	errorsInWindow := 0

	for _, record := range cb.requestHistory {
		if record.timestamp.After(windowStart) {
			totalInWindow++
			if !record.success {
				errorsInWindow++
			}
		}
	}

	if totalInWindow == 0 {
		return 0.0
	}

	return float64(errorsInWindow) / float64(totalInWindow)
}

// cleanOldHistory removes records outside the window
func (cb *CircuitBreaker) cleanOldHistory() {
	windowStart := time.Now().Add(-cb.config.WindowDuration)
	newHistory := make([]requestRecord, 0)

	for _, record := range cb.requestHistory {
		if record.timestamp.After(windowStart) {
			newHistory = append(newHistory, record)
		}
	}

	cb.requestHistory = newHistory
}

// transitionTo changes the circuit breaker state
func (cb *CircuitBreaker) transitionTo(newState CircuitState) {
	oldState := cb.state
	cb.state = newState
	cb.lastStateChange = time.Now()

	// Reset counters on state transition
	if newState == StateClosed {
		cb.failureCount = 0
		cb.successCount = 0
		cb.errorCount = 0
		cb.totalRequests = 0
	} else if newState == StateHalfOpen {
		cb.successCount = 0
		cb.failureCount = 0
	}

	log.Printf("[CircuitBreaker:%s] State transition: %s -> %s", cb.name, oldState, newState)
}

// GetState returns the current state (thread-safe)
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetStats returns current statistics
func (cb *CircuitBreaker) GetStats() map[string]interface{} {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	errorRate := cb.calculateErrorRate()

	stats := map[string]interface{}{
		"name":                  cb.name,
		"state":                 cb.state.String(),
		"failure_count":         cb.failureCount,
		"success_count":         cb.successCount,
		"total_requests":        cb.totalRequests,
		"error_count":           cb.errorCount,
		"error_rate":            fmt.Sprintf("%.2f%%", errorRate*100),
		"last_state_change":     cb.lastStateChange.Format(time.RFC3339),
		"time_in_current_state": time.Since(cb.lastStateChange).String(),
	}

	if cb.lastError != nil {
		stats["last_error"] = cb.lastError.Error()
	}

	return stats
}

// Reset resets the circuit breaker to initial state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateClosed
	cb.failureCount = 0
	cb.successCount = 0
	cb.totalRequests = 0
	cb.errorCount = 0
	cb.lastStateChange = time.Now()
	cb.lastError = nil
	cb.requestHistory = make([]requestRecord, 0)

	log.Printf("[CircuitBreaker:%s] Reset to CLOSED state", cb.name)
}
