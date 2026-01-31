package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// LogLevel defines logging severity levels
type LogLevel string

const (
	LogLevelDebug LogLevel = "DEBUG"
	LogLevelInfo  LogLevel = "INFO"
	LogLevelWarn  LogLevel = "WARN"
	LogLevelError LogLevel = "ERROR"
	LogLevelFatal LogLevel = "FATAL"
)

// StructuredLogger provides structured JSON logging with PII masking
type StructuredLogger struct {
	mu      sync.Mutex
	level   LogLevel
	output  *os.File
	masking bool
}

// LogEntry represents a structured log entry
type LogEntry struct {
	Timestamp     string                 `json:"timestamp"`
	Level         string                 `json:"level"`
	Message       string                 `json:"message"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	PaymentID     string                 `json:"payment_id,omitempty"`
	Provider      string                 `json:"provider,omitempty"`
	Operation     string                 `json:"operation,omitempty"`
	Latency       int64                  `json:"latency_ms,omitempty"`
	ErrorCode     string                 `json:"error_code,omitempty"`
	Fields        map[string]interface{} `json:"fields,omitempty"`
}

// NewStructuredLogger creates a new structured logger
func NewStructuredLogger(level LogLevel, enableMasking bool) *StructuredLogger {
	return &StructuredLogger{
		level:   level,
		output:  os.Stdout,
		masking: enableMasking,
	}
}

// Log writes a structured log entry
func (sl *StructuredLogger) Log(level LogLevel, message string, fields map[string]interface{}) {
	if !sl.shouldLog(level) {
		return
	}

	sl.mu.Lock()
	defer sl.mu.Unlock()

	// Mask PII if enabled
	if sl.masking && fields != nil {
		fields = maskPII(fields)
	}

	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     string(level),
		Message:   message,
		Fields:    fields,
	}

	// Extract common fields if present
	if correlationID, ok := fields["correlation_id"].(string); ok {
		entry.CorrelationID = correlationID
		delete(fields, "correlation_id")
	}

	if paymentID, ok := fields["payment_id"].(string); ok {
		entry.PaymentID = paymentID
		delete(fields, "payment_id")
	}

	if provider, ok := fields["provider"].(string); ok {
		entry.Provider = provider
		delete(fields, "provider")
	}

	if operation, ok := fields["operation"].(string); ok {
		entry.Operation = operation
		delete(fields, "operation")
	}

	if latency, ok := fields["latency_ms"].(int64); ok {
		entry.Latency = latency
		delete(fields, "latency_ms")
	}

	if errorCode, ok := fields["error_code"].(string); ok {
		entry.ErrorCode = errorCode
		delete(fields, "error_code")
	}

	// Marshal to JSON
	jsonBytes, err := json.Marshal(entry)
	if err != nil {
		log.Printf("Failed to marshal log entry: %v", err)
		return
	}

	fmt.Fprintln(sl.output, string(jsonBytes))
}

// Info logs an info level message
func (sl *StructuredLogger) Info(message string, fields map[string]interface{}) {
	sl.Log(LogLevelInfo, message, fields)
}

// Warn logs a warning level message
func (sl *StructuredLogger) Warn(message string, fields map[string]interface{}) {
	sl.Log(LogLevelWarn, message, fields)
}

// Error logs an error level message
func (sl *StructuredLogger) Error(message string, fields map[string]interface{}) {
	sl.Log(LogLevelError, message, fields)
}

// Debug logs a debug level message
func (sl *StructuredLogger) Debug(message string, fields map[string]interface{}) {
	sl.Log(LogLevelDebug, message, fields)
}

// Fatal logs a fatal level message and exits
func (sl *StructuredLogger) Fatal(message string, fields map[string]interface{}) {
	sl.Log(LogLevelFatal, message, fields)
	os.Exit(1)
}

// shouldLog determines if a message should be logged based on level
func (sl *StructuredLogger) shouldLog(level LogLevel) bool {
	levels := map[LogLevel]int{
		LogLevelDebug: 0,
		LogLevelInfo:  1,
		LogLevelWarn:  2,
		LogLevelError: 3,
		LogLevelFatal: 4,
	}

	return levels[level] >= levels[sl.level]
}

// maskPII masks sensitive information in log fields
func maskPII(fields map[string]interface{}) map[string]interface{} {
	masked := make(map[string]interface{})

	for k, v := range fields {
		key := strings.ToLower(k)

		// Check if this is a sensitive field
		if strings.Contains(key, "card") ||
			strings.Contains(key, "cvv") ||
			strings.Contains(key, "pin") ||
			strings.Contains(key, "password") ||
			strings.Contains(key, "secret") ||
			strings.Contains(key, "token") && !strings.Contains(key, "idempotency") {

			// Mask the value
			switch val := v.(type) {
			case string:
				masked[k] = maskString(val)
			default:
				masked[k] = "[REDACTED]"
			}
		} else if key == "email" {
			// Partially mask email
			if email, ok := v.(string); ok {
				masked[k] = maskEmail(email)
			} else {
				masked[k] = v
			}
		} else {
			masked[k] = v
		}
	}

	return masked
}

// maskString masks a string value, showing only first and last 4 characters
func maskString(s string) string {
	if len(s) <= 8 {
		return "[REDACTED]"
	}

	return s[:4] + strings.Repeat("*", len(s)-8) + s[len(s)-4:]
}

// maskEmail partially masks an email address
func maskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "[REDACTED]"
	}

	username := parts[0]
	domain := parts[1]

	maskedUsername := ""
	if len(username) <= 2 {
		maskedUsername = strings.Repeat("*", len(username))
	} else {
		maskedUsername = string(username[0]) + strings.Repeat("*", len(username)-2) + string(username[len(username)-1])
	}

	return maskedUsername + "@" + domain
}

// maskCardNumber masks a credit card number
func maskCardNumber(cardNumber string) string {
	// Remove non-numeric characters
	re := regexp.MustCompile(`\D`)
	digits := re.ReplaceAllString(cardNumber, "")

	if len(digits) < 4 {
		return "[REDACTED]"
	}

	// Show last 4 digits only
	return strings.Repeat("*", len(digits)-4) + digits[len(digits)-4:]
}

// LogRoutingDecision logs provider routing decisions
func LogRoutingDecision(logger *StructuredLogger, correlationID, paymentID string, providers []string, selected string, reason string) {
	logger.Info("Provider routing decision", map[string]interface{}{
		"correlation_id":     correlationID,
		"payment_id":         paymentID,
		"eligible_providers": providers,
		"selected_provider":  selected,
		"selection_reason":   reason,
		"operation":          "routing",
	})
}

// LogProviderRequest logs outgoing provider requests
func LogProviderRequest(logger *StructuredLogger, correlationID, paymentID, provider, operation string) {
	logger.Info("Provider request initiated", map[string]interface{}{
		"correlation_id": correlationID,
		"payment_id":     paymentID,
		"provider":       provider,
		"operation":      operation,
	})
}

// LogProviderResponse logs provider responses
func LogProviderResponse(logger *StructuredLogger, correlationID, paymentID, provider, operation string, latency int64, success bool, errorCode string) {
	level := LogLevelInfo
	message := "Provider request completed"

	if !success {
		level = LogLevelError
		message = "Provider request failed"
	}

	logger.Log(level, message, map[string]interface{}{
		"correlation_id": correlationID,
		"payment_id":     paymentID,
		"provider":       provider,
		"operation":      operation,
		"latency_ms":     latency,
		"success":        success,
		"error_code":     errorCode,
	})
}

// LogCircuitBreakerStateChange logs circuit breaker state transitions
func LogCircuitBreakerStateChange(logger *StructuredLogger, provider, oldState, newState, reason string) {
	logger.Warn("Circuit breaker state changed", map[string]interface{}{
		"provider":  provider,
		"old_state": oldState,
		"new_state": newState,
		"reason":    reason,
		"operation": "circuit_breaker",
	})
}

// InitLogger initializes the global logger
func InitLogger(level LogLevel, enablePIIMasking bool) {
	if appLogger == nil {
		appLogger = NewStructuredLogger(level, enablePIIMasking)
	}
}

// GetLogger returns the global logger instance
func GetLogger() *StructuredLogger {
	if appLogger == nil {
		appLogger = NewStructuredLogger(LogLevelInfo, true)
	}
	return appLogger
}
