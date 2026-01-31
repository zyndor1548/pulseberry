package main

import (
	"encoding/json"
	"net/http"
	"time"
)

// AdminProvidersHandler lists all registered providers
func AdminProvidersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := providerRegistry.GetAllProviderStatus()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// AdminProviderEnableHandler enables a provider
func AdminProviderEnableHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	providerName := r.URL.Query().Get("provider")
	if providerName == "" {
		http.Error(w, "Provider name required", http.StatusBadRequest)
		return
	}

	err := providerRegistry.EnableProvider(providerName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "Provider enabled successfully",
		"provider": providerName,
	})
}

// AdminProviderDisableHandler disables a provider
func AdminProviderDisableHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	providerName := r.URL.Query().Get("provider")
	if providerName == "" {
		http.Error(w, "Provider name required", http.StatusBadRequest)
		return
	}

	err := providerRegistry.DisableProvider(providerName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "Provider disabled successfully",
		"provider": providerName,
	})
}

// AdminCircuitBreakerResetHandler resets a provider's circuit breaker
func AdminCircuitBreakerResetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	providerName := r.URL.Query().Get("provider")
	if providerName == "" {
		http.Error(w, "Provider name required", http.StatusBadRequest)
		return
	}

	config, err := providerRegistry.GetPaymentProvider(providerName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if config.CircuitBreaker != nil {
		config.CircuitBreaker.Reset()
	}

	appLogger.Info("Circuit breaker reset", map[string]interface{}{
		"provider":     providerName,
		"admin_action": "reset_circuit_breaker",
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "Circuit breaker reset successfully",
		"provider": providerName,
	})
}

// HealthCheckHandler provides system health status
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check Redis connectivity
	redisHealthy := false
	if err := rdb.Ping(ctx).Err(); err == nil {
		redisHealthy = true
	}

	// Check database connectivity
	dbHealthy := false
	if Databaseconnection != nil {
		if err := Databaseconnection.Ping(); err == nil {
			dbHealthy = true
		}
	}

	// Count healthy providers
	healthyProviders := 0
	totalProviders := 0
	status := providerRegistry.GetAllProviderStatus()
	if paymentProviders, ok := status["payment_providers"].([]map[string]interface{}); ok {
		totalProviders = len(paymentProviders)
		for _, provider := range paymentProviders {
			if enabled, ok := provider["enabled"].(bool); ok && enabled {
				if cbStats, ok := provider["circuit_breaker"].(map[string]interface{}); ok {
					if state, ok := cbStats["state"].(string); ok && state == "CLOSED" {
						healthyProviders++
					}
				}
			}
		}
	}

	overallHealthy := redisHealthy && healthyProviders > 0

	statusCode := http.StatusOK
	if !overallHealthy {
		statusCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"healthy": overallHealthy,
		"checks": map[string]interface{}{
			"redis":    redisHealthy,
			"database": dbHealthy,
			"providers": map[string]interface{}{
				"total":   totalProviders,
				"healthy": healthyProviders,
			},
		},
		"timestamp": getCurrentTimeString(),
	})
}

// getCurrentTimeString returns current time as string
func getCurrentTimeString() string {
	return time.Now().UTC().Format(time.RFC3339)
}
