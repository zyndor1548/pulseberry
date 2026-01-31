package main

import (
	"errors"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RetryConfig holds configuration for retry behavior
type RetryConfig struct {
	MaxAttempts       int           // Maximum number of retry attempts
	BaseDelay         time.Duration // Base delay for exponential backoff
	MaxDelay          time.Duration // Maximum delay between retries
	JitterFactor      float64       // Jitter as percentage (0.25 = ±25%)
	RetryableStatuses []int         // HTTP status codes that are retryable
}

// DefaultRetryConfig returns sensible defaults for retry behavior
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  5,
		BaseDelay:    100 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		JitterFactor: 0.25,
		RetryableStatuses: []int{
			http.StatusRequestTimeout,      // 408
			http.StatusTooManyRequests,     // 429
			http.StatusInternalServerError, // 500
			http.StatusBadGateway,          // 502
			http.StatusServiceUnavailable,  // 503
			http.StatusGatewayTimeout,      // 504
		},
	}
}

// RetryDecision represents whether to retry and how long to wait
type RetryDecision struct {
	ShouldRetry bool
	Backoff     time.Duration
	Reason      string
}

// RetryStrategy determines retry behavior based on error and context
type RetryStrategy struct {
	config RetryConfig
}

// NewRetryStrategy creates a new retry strategy
func NewRetryStrategy(config RetryConfig) *RetryStrategy {
	return &RetryStrategy{
		config: config,
	}
}

// ShouldRetry determines if a request should be retried
func (rs *RetryStrategy) ShouldRetry(
	err error,
	statusCode int,
	attempt int,
	resp *http.Response,
) RetryDecision {
	// Exceeded max attempts
	if attempt >= rs.config.MaxAttempts {
		return RetryDecision{
			ShouldRetry: false,
			Backoff:     0,
			Reason:      "max_attempts_exceeded",
		}
	}

	// Handle HTTP status codes
	if statusCode > 0 {
		return rs.handleHTTPStatus(statusCode, attempt, resp)
	}

	// Handle errors
	if err != nil {
		return rs.handleError(err, attempt)
	}

	// No error and no bad status - don't retry
	return RetryDecision{
		ShouldRetry: false,
		Backoff:     0,
		Reason:      "success",
	}
}

// handleHTTPStatus determines retry behavior for HTTP status codes
func (rs *RetryStrategy) handleHTTPStatus(statusCode int, attempt int, resp *http.Response) RetryDecision {
	// 2xx - Success, don't retry
	if statusCode >= 200 && statusCode < 300 {
		return RetryDecision{
			ShouldRetry: false,
			Backoff:     0,
			Reason:      "success",
		}
	}

	// 4xx Client Errors - generally don't retry
	if statusCode >= 400 && statusCode < 500 {
		// Except for specific retryable 4xx codes
		if statusCode == http.StatusRequestTimeout { // 408
			backoff := rs.calculateBackoff(attempt)
			return RetryDecision{
				ShouldRetry: true,
				Backoff:     backoff,
				Reason:      "request_timeout",
			}
		}

		if statusCode == http.StatusTooManyRequests { // 429
			// Use Retry-After header if available
			backoff := rs.calculateBackoff(attempt)
			if resp != nil {
				if retryAfter := rs.parseRetryAfter(resp.Header.Get("Retry-After")); retryAfter > 0 {
					backoff = retryAfter
				}
			}

			return RetryDecision{
				ShouldRetry: true,
				Backoff:     backoff,
				Reason:      "rate_limited",
			}
		}

		// Other 4xx errors are not retryable (e.g., 400, 401, 403, 404)
		return RetryDecision{
			ShouldRetry: false,
			Backoff:     0,
			Reason:      "client_error",
		}
	}

	// 5xx Server Errors - retry
	if statusCode >= 500 {
		backoff := rs.calculateBackoff(attempt)
		reason := "server_error"

		switch statusCode {
		case http.StatusBadGateway:
			reason = "bad_gateway"
		case http.StatusServiceUnavailable:
			reason = "service_unavailable"
		case http.StatusGatewayTimeout:
			reason = "gateway_timeout"
		}

		return RetryDecision{
			ShouldRetry: true,
			Backoff:     backoff,
			Reason:      reason,
		}
	}

	// Unknown status code - don't retry
	return RetryDecision{
		ShouldRetry: false,
		Backoff:     0,
		Reason:      "unknown_status",
	}
}

// handleError determines retry behavior for errors
func (rs *RetryStrategy) handleError(err error, attempt int) RetryDecision {
	if err == nil {
		return RetryDecision{
			ShouldRetry: false,
			Backoff:     0,
			Reason:      "no_error",
		}
	}

	// Network errors - retryable
	if isNetworkError(err) {
		backoff := rs.calculateBackoff(attempt)
		return RetryDecision{
			ShouldRetry: true,
			Backoff:     backoff,
			Reason:      "network_error",
		}
	}

	// Timeout errors - retryable
	if isTimeoutError(err) {
		backoff := rs.calculateBackoff(attempt)
		return RetryDecision{
			ShouldRetry: true,
			Backoff:     backoff,
			Reason:      "timeout",
		}
	}

	// Connection refused - retryable
	if isConnectionRefused(err) {
		backoff := rs.calculateBackoff(attempt)
		return RetryDecision{
			ShouldRetry: true,
			Backoff:     backoff,
			Reason:      "connection_refused",
		}
	}

	// Other errors - not retryable by default
	return RetryDecision{
		ShouldRetry: false,
		Backoff:     0,
		Reason:      "unretryable_error",
	}
}

// calculateBackoff calculates exponential backoff with jitter
func (rs *RetryStrategy) calculateBackoff(attempt int) time.Duration {
	// Exponential backoff: baseDelay * 2^attempt
	backoff := rs.config.BaseDelay * time.Duration(1<<uint(attempt))

	// Cap at max delay
	if backoff > rs.config.MaxDelay {
		backoff = rs.config.MaxDelay
	}

	// Add jitter: ±jitterFactor%
	jitterRange := float64(backoff) * rs.config.JitterFactor
	jitter := time.Duration(rand.Float64()*2*jitterRange - jitterRange)

	backoff += jitter

	// Ensure non-negative
	if backoff < 0 {
		backoff = rs.config.BaseDelay
	}

	return backoff
}

// parseRetryAfter parses the Retry-After header
func (rs *RetryStrategy) parseRetryAfter(retryAfter string) time.Duration {
	if retryAfter == "" {
		return 0
	}

	// Try parsing as seconds (integer)
	if seconds, err := strconv.Atoi(retryAfter); err == nil {
		return time.Duration(seconds) * time.Second
	}

	// Try parsing as HTTP date
	if t, err := http.ParseTime(retryAfter); err == nil {
		duration := time.Until(t)
		if duration < 0 {
			return 0
		}
		return duration
	}

	return 0
}

// Helper functions for error classification

// isNetworkError checks if an error is a network error
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}

	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return true
	}

	// Check for common network error strings
	errStr := err.Error()
	networkErrorPatterns := []string{
		"connection reset",
		"connection refused",
		"no such host",
		"network is unreachable",
		"network is down",
		"broken pipe",
	}

	for _, pattern := range networkErrorPatterns {
		if strings.Contains(strings.ToLower(errStr), pattern) {
			return true
		}
	}

	return false
}

// isTimeoutError checks if an error is a timeout error
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	// Check if error implements Timeout() method
	type timeoutError interface {
		Timeout() bool
	}

	if te, ok := err.(timeoutError); ok {
		return te.Timeout()
	}

	// Check for timeout in error string
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline exceeded")
}

// isConnectionRefused checks if an error is a connection refused error
func isConnectionRefused(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "connection refused")
}

// RetryableError wraps an error with retry context
type RetryableError struct {
	OriginalError error
	Attempt       int
	NextBackoff   time.Duration
	Retryable     bool
	Reason        string
}

// Error implements the error interface
func (re *RetryableError) Error() string {
	if re.OriginalError != nil {
		return re.OriginalError.Error()
	}
	return "retryable error"
}

// Unwrap implements the unwrap interface for error chains
func (re *RetryableError) Unwrap() error {
	return re.OriginalError
}
