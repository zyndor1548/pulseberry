package main

import (
	"encoding/json"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"
)

// LoadSheddingConfig holds configuration for load shedding
type LoadSheddingConfig struct {
	Enabled              bool    // Enable/disable load shedding
	MaxActiveRequests    int32   // Maximum concurrent active requests
	LatencyThresholdMs   int64   // P99 latency threshold in milliseconds
	CPUThreshold         float64 // CPU usage threshold (0.0 to 1.0)
	ErrorRateThreshold   float64 // Error rate threshold (0.0 to 1.0)
	CircuitOpenThreshold int     // Number of open circuits before shedding
}

// DefaultLoadSheddingConfig returns sensible defaults
func DefaultLoadSheddingConfig() LoadSheddingConfig {
	return LoadSheddingConfig{
		Enabled:              true,
		MaxActiveRequests:    1000,
		LatencyThresholdMs:   5000, // 5 seconds
		CPUThreshold:         0.80, // 80%
		ErrorRateThreshold:   0.50, // 50%
		CircuitOpenThreshold: 2,    // 2 or more circuits open
	}
}

// LoadShedder monitors system health and sheds load when overloaded
type LoadShedder struct {
	config           LoadSheddingConfig
	activeRequests   atomic.Int32
	totalRequests    atomic.Int64
	shedRequests     atomic.Int64
	latencyTracker   *LatencyTracker
	providerRegistry *ProviderRegistry
	lastCPUCheck     time.Time
	lastCPUUsage     float64
}

// NewLoadShedder creates a new load shedder
func NewLoadShedder(config LoadSheddingConfig, latencyTracker *LatencyTracker, registry *ProviderRegistry) *LoadShedder {
	return &LoadShedder{
		config:           config,
		latencyTracker:   latencyTracker,
		providerRegistry: registry,
		lastCPUCheck:     time.Now(),
	}
}

// IncrementActive increments the active request counter
func (ls *LoadShedder) IncrementActive() {
	ls.activeRequests.Add(1)
	ls.totalRequests.Add(1)
}

// DecrementActive decrements the active request counter
func (ls *LoadShedder) DecrementActive() {
	ls.activeRequests.Add(-1)
}

// ShouldShed determines if incoming requests should be rejected
func (ls *LoadShedder) ShouldShed() (bool, string) {
	if !ls.config.Enabled {
		return false, ""
	}

	// Check 1: Active request count
	activeReqs := ls.activeRequests.Load()
	if activeReqs > ls.config.MaxActiveRequests {
		ls.shedRequests.Add(1)
		return true, "max_active_requests_exceeded"
	}

	// Check 2: P99 Latency
	if ls.latencyTracker != nil {
		percentiles := ls.latencyTracker.GetPercentiles()
		if percentiles.P99.Milliseconds() > ls.config.LatencyThresholdMs {
			ls.shedRequests.Add(1)
			return true, "high_latency_detected"
		}
	}

	// Check 3: CPU Usage (check every 5 seconds to avoid overhead)
	if time.Since(ls.lastCPUCheck) > 5*time.Second {
		cpuUsage := ls.getCPUUsage()
		ls.lastCPUUsage = cpuUsage
		ls.lastCPUCheck = time.Now()

		if cpuUsage > ls.config.CPUThreshold {
			ls.shedRequests.Add(1)
			return true, "high_cpu_usage"
		}
	} else {
		// Use cached CPU value
		if ls.lastCPUUsage > ls.config.CPUThreshold {
			ls.shedRequests.Add(1)
			return true, "high_cpu_usage"
		}
	}

	// Check 4: Circuit Breaker States
	if ls.providerRegistry != nil {
		openCircuits := ls.countOpenCircuits()
		if openCircuits >= ls.config.CircuitOpenThreshold {
			ls.shedRequests.Add(1)
			return true, "multiple_circuits_open"
		}
	}

	return false, ""
}

// getCPUUsage estimates CPU usage as a percentage (0.0 to 1.0)
func (ls *LoadShedder) getCPUUsage() float64 {
	// Use Go runtime stats as a proxy for CPU usage
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// NumGoroutine as a rough indicator of system load
	numGoroutines := float64(runtime.NumGoroutine())

	// Normalize: assume 1000 goroutines = 80% load
	// This is a rough heuristic and should be calibrated per system
	cpuEstimate := numGoroutines / 1250.0 // 1250 goroutines = 100% CPU

	if cpuEstimate > 1.0 {
		cpuEstimate = 1.0
	}

	return cpuEstimate
}

// countOpenCircuits counts how many circuit breakers are in OPEN state
func (ls *LoadShedder) countOpenCircuits() int {
	// This would query the provider registry in real implementation
	openCount := 0

	ls.providerRegistry.mu.RLock()
	defer ls.providerRegistry.mu.RUnlock()

	for _, config := range ls.providerRegistry.paymentProviders {
		if config.CircuitBreaker != nil {
			if config.CircuitBreaker.GetState() == StateOpen {
				openCount++
			}
		}
	}

	return openCount
}

// GetStats returns load shedding statistics
func (ls *LoadShedder) GetStats() LoadSheddingStats {
	totalReqs := ls.totalRequests.Load()
	shedReqs := ls.shedRequests.Load()

	shedRate := 0.0
	if totalReqs > 0 {
		shedRate = float64(shedReqs) / float64(totalReqs) * 100
	}

	return LoadSheddingStats{
		Enabled:          ls.config.Enabled,
		ActiveRequests:   int(ls.activeRequests.Load()),
		TotalRequests:    totalReqs,
		ShedRequests:     shedReqs,
		ShedRate:         shedRate,
		MaxActiveAllowed: int(ls.config.MaxActiveRequests),
		CPUUsage:         ls.lastCPUUsage,
		CPUThreshold:     ls.config.CPUThreshold,
	}
}

// LoadSheddingStats holds statistics about load shedding
type LoadSheddingStats struct {
	Enabled          bool    `json:"enabled"`
	ActiveRequests   int     `json:"active_requests"`
	TotalRequests    int64   `json:"total_requests"`
	ShedRequests     int64   `json:"shed_requests"`
	ShedRate         float64 `json:"shed_rate_percent"`
	MaxActiveAllowed int     `json:"max_active_allowed"`
	CPUUsage         float64 `json:"cpu_usage"`
	CPUThreshold     float64 `json:"cpu_threshold"`
}

// LoadSheddingMiddleware wraps HTTP handlers with load shedding
func LoadSheddingMiddleware(loadShedder *LoadShedder) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if we should shed this request
			shouldShed, reason := loadShedder.ShouldShed()
			if shouldShed {
				// Log shedding event
				if appLogger != nil {
					correlationID, _ := r.Context().Value("correlation_id").(string)
					appLogger.Warn("Load shedding activated", map[string]interface{}{
						"correlation_id":  correlationID,
						"reason":          reason,
						"active_requests": loadShedder.activeRequests.Load(),
					})
				}

				// Return 503 Service Unavailable
				w.Header().Set("Retry-After", "5") // Suggest retry after 5 seconds
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)

				response := NewErrorResponse(
					ErrRateLimited,
					"System overloaded, please retry",
					"REJECTED",
					reason,
				)
				json.NewEncoder(w).Encode(response)
				return
			}

			// Track active request
			loadShedder.IncrementActive()
			defer loadShedder.DecrementActive()

			// Continue to next handler
			next.ServeHTTP(w, r)
		})
	}
}

// Global load shedder instance
var globalLoadShedder *LoadShedder

// InitLoadShedder initializes the global load shedder
func InitLoadShedder(config LoadSheddingConfig, latencyTracker *LatencyTracker, registry *ProviderRegistry) {
	globalLoadShedder = NewLoadShedder(config, latencyTracker, registry)
}

// GetLoadShedder returns the global load shedder instance
func GetLoadShedder() *LoadShedder {
	return globalLoadShedder
}
