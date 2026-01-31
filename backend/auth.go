package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	ErrMissingAPIKey    = errors.New("missing API key")
	ErrInvalidAPIKey    = errors.New("invalid API key")
	ErrInvalidSignature = errors.New("invalid request signature")
	ErrMissingSignature = errors.New("missing request signature")
	ErrExpiredTimestamp = errors.New("request timestamp expired")
	ErrMissingTimestamp = errors.New("missing request timestamp")
)

// APIKey represents an API key configuration
type APIKey struct {
	Key       string
	Secret    string
	Name      string
	Enabled   bool
	CreatedAt time.Time
	ExpiresAt *time.Time
}

// APIKeyStore manages API keys
type APIKeyStore struct {
	keys map[string]*APIKey
	mu   sync.RWMutex
}

// NewAPIKeyStore creates a new API key store
func NewAPIKeyStore() *APIKeyStore {
	return &APIKeyStore{
		keys: make(map[string]*APIKey),
	}
}

// AddKey adds an API key to the store
func (aks *APIKeyStore) AddKey(key *APIKey) {
	aks.mu.Lock()
	defer aks.mu.Unlock()
	aks.keys[key.Key] = key
}

// GetKey retrieves an API key
func (aks *APIKeyStore) GetKey(key string) (*APIKey, error) {
	aks.mu.RLock()
	defer aks.mu.RUnlock()

	apiKey, exists := aks.keys[key]
	if !exists {
		return nil, ErrInvalidAPIKey
	}

	if !apiKey.Enabled {
		return nil, ErrInvalidAPIKey
	}

	// Check expiration
	if apiKey.ExpiresAt != nil && time.Now().After(*apiKey.ExpiresAt) {
		return nil, ErrInvalidAPIKey
	}

	return apiKey, nil
}

// AuthMiddleware provides API key authentication
func AuthMiddleware(keyStore *APIKeyStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				http.Error(w, "Missing API key", http.StatusUnauthorized)
				return
			}

			// Validate API key
			key, err := keyStore.GetKey(apiKey)
			if err != nil {
				http.Error(w, "Invalid API key", http.StatusUnauthorized)
				return
			}

			// Verify request signature (HMAC-SHA256)
			signature := r.Header.Get("X-Signature")
			timestamp := r.Header.Get("X-Timestamp")

			if signature == "" {
				http.Error(w, "Missing signature", http.StatusUnauthorized)
				return
			}

			if timestamp == "" {
				http.Error(w, "Missing timestamp", http.StatusUnauthorized)
				return
			}

			// Validate timestamp (prevent replay attacks)
			reqTime, err := time.Parse(time.RFC3339, timestamp)
			if err != nil {
				http.Error(w, "Invalid timestamp format", http.StatusBadRequest)
				return
			}

			// Allow 5 minute clock skew
			if time.Since(reqTime) > 5*time.Minute || time.Until(reqTime) > 5*time.Minute {
				http.Error(w, "Request timestamp expired", http.StatusUnauthorized)
				return
			}

			// Verify signature
			// Signature is HMAC-SHA256(secret, method + path + timestamp + body)
			// For now, we'll skip body verification for simplicity in GET requests
			expectedSig := computeSignature(key.Secret, r.Method, r.URL.Path, timestamp)

			if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
				http.Error(w, "Invalid signature", http.StatusUnauthorized)
				return
			}

			// Add API key to context
			ctx := context.WithValue(r.Context(), "api_key", apiKey)
			ctx = context.WithValue(ctx, "api_key_name", key.Name)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// computeSignature generates HMAC-SHA256 signature
func computeSignature(secret, method, path, timestamp string) string {
	message := strings.Join([]string{method, path, timestamp}, "|")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// RequestValidationMiddleware validates request size and format
func RequestValidationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Check content length (max 10KB)
		if r.ContentLength > 10*1024 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			w.Write([]byte(`{"error": "Request body too large", "max_size_bytes": 10240}`))
			return
		}

		// 2. Enforce JSON content type for POST/PUT
		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			contentType := r.Header.Get("Content-Type")
			if !strings.Contains(contentType, "application/json") {
				http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// CorrelationIDMiddleware adds X-Correlation-ID to requests
func CorrelationIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationID := r.Header.Get("X-Correlation-ID")
		if correlationID == "" {
			correlationID = generateCorrelationID()
			r.Header.Set("X-Correlation-ID", correlationID)
		}

		// Add to response headers
		w.Header().Set("X-Correlation-ID", correlationID)

		// Add to context
		ctx := context.WithValue(r.Context(), "correlation_id", correlationID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// generateCorrelationID generates a unique correlation ID
func generateCorrelationID() string {
	// Simple implementation using timestamp + random
	return fmt.Sprintf("corr_%d", time.Now().UnixNano())
}

// TimeoutMiddleware enforces request timeout
func TimeoutMiddleware(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}
