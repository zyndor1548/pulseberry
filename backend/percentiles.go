package main

import (
	"sort"
	"sync"
	"time"
)

// LatencyTracker tracks latency percentiles using a sliding window
type LatencyTracker struct {
	samples    []time.Duration
	maxSamples int
	mu         sync.RWMutex
}

// NewLatencyTracker creates a new latency tracker
func NewLatencyTracker(maxSamples int) *LatencyTracker {
	return &LatencyTracker{
		samples:    make([]time.Duration, 0, maxSamples),
		maxSamples: maxSamples,
	}
}

// AddSample records a new latency sample
func (lt *LatencyTracker) AddSample(latency time.Duration) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	lt.samples = append(lt.samples, latency)

	// Keep only the most recent samples
	if len(lt.samples) > lt.maxSamples {
		// Remove oldest samples
		excess := len(lt.samples) - lt.maxSamples
		lt.samples = lt.samples[excess:]
	}
}

// GetPercentiles calculates P50, P95, and P99 latencies
func (lt *LatencyTracker) GetPercentiles() LatencyPercentiles {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	if len(lt.samples) == 0 {
		return LatencyPercentiles{
			P50: 0,
			P95: 0,
			P99: 0,
		}
	}

	// Create a sorted copy
	sorted := make([]time.Duration, len(lt.samples))
	copy(sorted, lt.samples)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	return LatencyPercentiles{
		P50: percentile(sorted, 50),
		P95: percentile(sorted, 95),
		P99: percentile(sorted, 99),
	}
}

// percentile calculates the p-th percentile from sorted samples
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}

	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}

	// Calculate index
	index := (p / 100.0) * float64(len(sorted)-1)
	lower := int(index)
	upper := lower + 1

	if upper >= len(sorted) {
		return sorted[len(sorted)-1]
	}

	// Linear interpolation
	weight := index - float64(lower)
	return time.Duration(float64(sorted[lower])*(1-weight) + float64(sorted[upper])*weight)
}

// GetAverage calculates average latency
func (lt *LatencyTracker) GetAverage() time.Duration {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	if len(lt.samples) == 0 {
		return 0
	}

	var total time.Duration
	for _, sample := range lt.samples {
		total += sample
	}

	return total / time.Duration(len(lt.samples))
}

// GetMin returns minimum latency
func (lt *LatencyTracker) GetMin() time.Duration {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	if len(lt.samples) == 0 {
		return 0
	}

	min := lt.samples[0]
	for _, sample := range lt.samples {
		if sample < min {
			min = sample
		}
	}

	return min
}

// GetMax returns maximum latency
func (lt *LatencyTracker) GetMax() time.Duration {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	if len(lt.samples) == 0 {
		return 0
	}

	max := lt.samples[0]
	for _, sample := range lt.samples {
		if sample > max {
			max = sample
		}
	}

	return max
}

// Reset clears all samples
func (lt *LatencyTracker) Reset() {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	lt.samples = make([]time.Duration, 0, lt.maxSamples)
}

// GetSampleCount returns the number of samples
func (lt *LatencyTracker) GetSampleCount() int {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	return len(lt.samples)
}
