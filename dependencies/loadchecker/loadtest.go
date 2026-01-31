package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Comprehensive Load Testing Suite

type LoadTestConfig struct {
	BaseURL           string
	TotalRequests     int
	Concurrency       int
	RampUpDurationSec int
	TestScenario      string
}

type LoadTestStats struct {
	TotalRequests int64
	SuccessCount  int64
	FailureCount  int64
	TotalLatency  int64
	MinLatency    int64
	MaxLatency    int64
	StatusCodes   map[int]int64
	Latencies     []int64
	mu            sync.Mutex
}

func (s *LoadTestStats) RecordRequest(statusCode int, latency time.Duration) {
	atomic.AddInt64(&s.TotalRequests, 1)
	latencyMs := latency.Milliseconds()
	atomic.AddInt64(&s.TotalLatency, latencyMs)

	if statusCode >= 200 && statusCode < 300 {
		atomic.AddInt64(&s.SuccessCount, 1)
	} else {
		atomic.AddInt64(&s.FailureCount, 1)
	}

	// Update min/max
	for {
		oldMin := atomic.LoadInt64(&s.MinLatency)
		if oldMin == 0 || latencyMs < oldMin {
			if atomic.CompareAndSwapInt64(&s.MinLatency, oldMin, latencyMs) {
				break
			}
		} else {
			break
		}
	}

	for {
		oldMax := atomic.LoadInt64(&s.MaxLatency)
		if latencyMs > oldMax {
			if atomic.CompareAndSwapInt64(&s.MaxLatency, oldMax, latencyMs) {
				break
			}
		} else {
			break
		}
	}

	// Track latencies for percentile calculation
	s.mu.Lock()
	s.Latencies = append(s.Latencies, latencyMs)
	s.StatusCodes[statusCode]++
	s.mu.Unlock()
}

func (s *LoadTestStats) CalculatePercentiles() (p50, p95, p99 int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.Latencies) == 0 {
		return 0, 0, 0
	}

	// Simple percentile calculation (not perfectly accurate but good enough)
	sorted := make([]int64, len(s.Latencies))
	copy(sorted, s.Latencies)

	// Bubble sort (fine for test data)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	p50 = sorted[len(sorted)/2]
	p95 = sorted[int(float64(len(sorted))*0.95)]
	p99 = sorted[int(float64(len(sorted))*0.99)]
	return
}

func (s *LoadTestStats) PrintStats(duration time.Duration) {
	total := atomic.LoadInt64(&s.TotalRequests)
	success := atomic.LoadInt64(&s.SuccessCount)
	failure := atomic.LoadInt64(&s.FailureCount)
	totalLatency := atomic.LoadInt64(&s.TotalLatency)
	minLatency := atomic.LoadInt64(&s.MinLatency)
	maxLatency := atomic.LoadInt64(&s.MaxLatency)

	p50, p95, p99 := s.CalculatePercentiles()

	fmt.Println("\n╔═══════════════════════════════════════════════════════╗")
	fmt.Println("║          LOAD TEST RESULTS                            ║")
	fmt.Println("╠═══════════════════════════════════════════════════════╣")
	fmt.Printf("║ Total Requests:    %-30d ║\n", total)
	fmt.Printf("║ Successful:        %-15d (%.2f%%)      ║\n", success, float64(success)/float64(total)*100)
	fmt.Printf("║ Failed:            %-15d (%.2f%%)      ║\n", failure, float64(failure)/float64(total)*100)
	fmt.Println("╠═══════════════════════════════════════════════════════╣")
	fmt.Printf("║ Latency (ms):                                         ║\n")
	fmt.Printf("║   Min:             %-30d ║\n", minLatency)
	fmt.Printf("║   P50:             %-30d ║\n", p50)
	fmt.Printf("║   P95:             %-30d ║\n", p95)
	fmt.Printf("║   P99:             %-30d ║\n", p99)
	fmt.Printf("║   Max:             %-30d ║\n", maxLatency)
	if total > 0 {
		fmt.Printf("║   Average:         %-30d ║\n", totalLatency/total)
	}
	fmt.Println("╠═══════════════════════════════════════════════════════╣")
	fmt.Println("║ Status Code Distribution:                            ║")
	s.mu.Lock()
	for code, count := range s.StatusCodes {
		fmt.Printf("║   %d: %-15d (%.2f%%)                   ║\n", code, count, float64(count)/float64(total)*100)
	}
	s.mu.Unlock()
	fmt.Println("╠═══════════════════════════════════════════════════════╣")
	fmt.Printf("║ Total Duration:    %-30v ║\n", duration)
	fmt.Printf("║ Requests/sec:      %-30.2f ║\n", float64(total)/duration.Seconds())
	fmt.Println("╚═══════════════════════════════════════════════════════╝")
}

// Test Scenario 1: Normal Load
func normalLoadScenario(config LoadTestConfig) *LoadTestStats {
	fmt.Println("\n🔥 Starting Test Scenario: NORMAL LOAD")
	fmt.Printf("   Requests: %d | Concurrency: %d\n", config.TotalRequests, config.Concurrency)

	stats := &LoadTestStats{StatusCodes: make(map[int]int64)}
	startTime := time.Now()

	sem := make(chan struct{}, config.Concurrency)
	var wg sync.WaitGroup

	for i := 1; i <= config.TotalRequests; i++ {
		wg.Add(1)
		sem <- struct{}{}

		go func(reqNum int) {
			defer func() { <-sem }()
			sendPaymentRequest(config.BaseURL, reqNum, stats, &wg)
		}(i)

		// Gradual ramp-up
		if i <= config.TotalRequests/10 {
			time.Sleep(100 * time.Millisecond)
		} else {
			time.Sleep(5000 * time.Microsecond)
		}
	}

	wg.Wait()
	duration := time.Since(startTime)
	stats.PrintStats(duration)
	return stats
}

// Test Scenario 2: Circuit Breaker Test
func circuitBreakerTest(config LoadTestConfig) {
	fmt.Println("\n🔌 Starting Test Scenario: CIRCUIT BREAKER TRIGGER")

	// Step 1: Configure gateway to fail
	fmt.Println("   Configuring test gateway to fail...")
	configGateway(config.BaseURL, "test1", 100, 1.0, "Gateway down", 503, "status_code")

	// Step 2: Send requests to trigger circuit breaker
	fmt.Println("   Sending 15 requests to trigger circuit breaker...")
	stats := &LoadTestStats{StatusCodes: make(map[int]int64)}
	var wg sync.WaitGroup

	for i := 1; i <= 15; i++ {
		wg.Add(1)
		go sendPaymentRequest(config.BaseURL, i, stats, &wg)
		time.Sleep(100 * time.Millisecond)
	}

	wg.Wait()

	// Step 3: Check circuit breaker state
	fmt.Println("   Checking circuit breaker state...")
	resp, err := http.Get(config.BaseURL + "/metrics")
	if err != nil {
		fmt.Printf("   ⚠️  Failed to get metrics: %v\n", err)
		return
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var metrics map[string]interface{}
	if err := json.Unmarshal(body, &metrics); err != nil {
		fmt.Printf("   ⚠️  Failed to parse metrics: %v\n", err)
		return
	}

	fmt.Println("\n   Circuit Breaker Status:")
	if pr, ok := metrics["provider_registry"].(map[string]interface{}); ok {
		if providers, ok := pr["payment_providers"].([]interface{}); ok {
			for _, p := range providers {
				if provider, ok := p.(map[string]interface{}); ok {
					if name, ok := provider["name"].(string); ok {
						if cb, ok := provider["circuit_breaker"].(map[string]interface{}); ok {
							if state, ok := cb["state"].(string); ok {
								fmt.Printf("     %s: %s\n", name, state)
							}
						}
					}
				}
			}
		} else {
			fmt.Println("   ⚠️  No payment providers found in metrics")
		}
	} else {
		fmt.Println("   ⚠️  No provider registry found in metrics")
	}

	// Step 4: Reset gateway
	fmt.Println("\n   Resetting gateway to normal...")
	configGateway(config.BaseURL, "test1", 100, 0.1, "Normal operation", 200, "json")
}

// Test Scenario 3: Rate Limiting Test
func rateLimitTest(config LoadTestConfig) {
	fmt.Println("\n🚦 Starting Test Scenario: RATE LIMIT TEST")
	fmt.Println("   Sending 150 requests (quota: 100/min)")

	stats := &LoadTestStats{StatusCodes: make(map[int]int64)}
	startTime := time.Now()

	for i := 1; i <= 150; i++ {
		reqStart := time.Now()
		resp, err := http.Post(config.BaseURL+"/payment", "application/json",
			bytes.NewBufferString(fmt.Sprintf(`{"id":"test_%d","amount":%d,"payment_id":"pay_%d"}`, i, 1000+i, i)))

		latency := time.Since(reqStart)

		if err != nil {
			stats.RecordRequest(0, latency)
		} else {
			stats.RecordRequest(resp.StatusCode, latency)
			io.ReadAll(resp.Body)
			resp.Body.Close()

			if resp.StatusCode == 429 {
				fmt.Printf("   ⚠️  Request %d: Rate limited (429)\n", i)
			}
		}

		time.Sleep(100 * time.Millisecond)
	}

	duration := time.Since(startTime)
	stats.PrintStats(duration)
}

// Test Scenario 4: High-Value Compliance Test
func complianceTest(config LoadTestConfig) {
	fmt.Println("\n✅ Starting Test Scenario: COMPLIANCE CHECK TEST")
	fmt.Println("   Testing high-value transactions (>$10k) trigger KYC")

	// Test 1: Low value (should not trigger compliance)
	fmt.Println("\n   Test 1: $50 payment (no compliance check)")
	sendSinglePayment(config.BaseURL, "compliance_low", 5000, "user_123")

	// Test 2: High value (should trigger compliance)
	fmt.Println("\n   Test 2: $15,000 payment (triggers KYC)")
	sendSinglePayment(config.BaseURL, "compliance_high", 1500000, "user_123")

	// Test 3: High value without user_id (should proceed)
	fmt.Println("\n   Test 3: $20,000 payment without user_id (backward compat)")
	sendSinglePayment(config.BaseURL, "compliance_no_user", 2000000, "")
}

// Helper Functions

func sendPaymentRequest(baseURL string, requestNum int, stats *LoadTestStats, wg *sync.WaitGroup) {
	defer wg.Done()

	// Get payment key
	keyReq := map[string]interface{}{
		"id":     fmt.Sprintf("order_%d", requestNum),
		"amount": 1000 + requestNum,
	}
	reqBody, _ := json.Marshal(keyReq)
	resp, err := http.Post(baseURL+"/paymentKey", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		stats.RecordRequest(0, 0)
		return
	}

	var keyResp map[string]string
	json.NewDecoder(resp.Body).Decode(&keyResp)
	resp.Body.Close()

	paymentID, ok := keyResp["payment_id"]
	if !ok {
		stats.RecordRequest(resp.StatusCode, 0)
		return
	}

	// Send payment
	paymentReq := map[string]interface{}{
		"id":         fmt.Sprintf("order_%d", requestNum),
		"amount":     1000 + requestNum,
		"payment_id": paymentID,
		"currency":   "USD",
	}
	reqBody, _ = json.Marshal(paymentReq)

	startTime := time.Now()
	resp, err = http.Post(baseURL+"/payment", "application/json", bytes.NewBuffer(reqBody))
	latency := time.Since(startTime)

	if err != nil {
		stats.RecordRequest(0, latency)
		return
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)

	stats.RecordRequest(resp.StatusCode, latency)
}

func sendSinglePayment(baseURL, orderID string, amount int64, userID string) {
	// Get key
	keyReq := map[string]interface{}{"id": orderID, "amount": amount}
	reqBody, _ := json.Marshal(keyReq)
	resp, _ := http.Post(baseURL+"/paymentKey", "application/json", bytes.NewBuffer(reqBody))
	var keyResp map[string]string
	json.NewDecoder(resp.Body).Decode(&keyResp)
	resp.Body.Close()

	// Send payment
	paymentReq := map[string]interface{}{
		"id":         orderID,
		"amount":     amount,
		"payment_id": keyResp["payment_id"],
		"currency":   "USD",
	}
	if userID != "" {
		paymentReq["user_id"] = userID
	}

	reqBody, _ = json.Marshal(paymentReq)
	resp, _ = http.Post(baseURL+"/payment", "application/json", bytes.NewBuffer(reqBody))
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	fmt.Printf("   Response (%d): %s\n", resp.StatusCode, string(body))
}

func configGateway(baseURL, gateway string, latency int, failureRate float64, errorMsg string, statusCode int, errorType string) {
	// This would call the gateway control endpoint if available
	// For now, just log the action
	log.Printf("Configuring %s: latency=%dms, failureRate=%.2f", gateway, latency, failureRate)
}

func main() {
	baseURL := "http://localhost:3000"

	fmt.Println("╔═══════════════════════════════════════════════════════════╗")
	fmt.Println("║      PULSEBERRY COMPREHENSIVE LOAD TEST SUITE             ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════╝")

	// Menu
	fmt.Println("\nSelect test scenario:")
	fmt.Println("  1. Normal Load Test (1000 requests, 100 concurrent)")
	fmt.Println("  2. Circuit Breaker Test")
	fmt.Println("  3. Rate Limiting Test")
	fmt.Println("  4. Compliance (KYC) Test")
	fmt.Println("  5. Run All Tests")

	var choice int
	fmt.Print("\nEnter choice (1-5): ")
	fmt.Scan(&choice)

	config := LoadTestConfig{
		BaseURL:       baseURL,
		TotalRequests: 1000,
		Concurrency:   100,
	}

	switch choice {
	case 1:
		normalLoadScenario(config)
	case 2:
		circuitBreakerTest(config)
	case 3:
		rateLimitTest(config)
	case 4:
		complianceTest(config)
	case 5:
		fmt.Println("\n🚀 Running full test suite...\n")
		normalLoadScenario(config)
		time.Sleep(2 * time.Second)

		circuitBreakerTest(config)
		time.Sleep(2 * time.Second)

		rateLimitTest(config)
		time.Sleep(2 * time.Second)

		complianceTest(config)

		fmt.Println("\n✅ Full test suite completed!")
	default:
		fmt.Println("Invalid choice")
	}
}
