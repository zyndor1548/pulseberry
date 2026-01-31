package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ErrorCode represents canonical error codes
type ErrorCode string

// Error codes from errors.go
const (
	ErrInvalidRequest    ErrorCode = "INVALID_REQUEST"
	ErrInsufficientFunds ErrorCode = "INSUFFICIENT_FUNDS"
	ErrCardDeclined      ErrorCode = "CARD_DECLINED"
	ErrAuthFailed        ErrorCode = "AUTHENTICATION_FAILED"
	ErrGatewayTimeout    ErrorCode = "GATEWAY_TIMEOUT"
	ErrProviderError     ErrorCode = "PROVIDER_ERROR"
	ErrProviderDown      ErrorCode = "PROVIDER_DOWN"
	ErrConnectionReset   ErrorCode = "CONNECTION_RESET"
	ErrConnectionTimeout ErrorCode = "CONNECTION_TIMEOUT"
	ErrMalformedResponse ErrorCode = "MALFORMED_RESPONSE"
	ErrEmptyResponse     ErrorCode = "EMPTY_RESPONSE"
	ErrSlowResponse      ErrorCode = "SLOW_RESPONSE"
	ErrInvalidJSON       ErrorCode = "INVALID_JSON"
	ErrRateLimited       ErrorCode = "RATE_LIMITED"
	ErrInternalError     ErrorCode = "INTERNAL_ERROR"
	ErrPanic             ErrorCode = "PANIC"
	ErrComplianceFailed  ErrorCode = "COMPLIANCE_FAILED"
	ErrKYCRequired       ErrorCode = "KYC_REQUIRED"
)

// GatewayConfig holds configuration for each gateway/provider
type GatewayConfig struct {
	Name         string
	Type         string // "provider" or "test"
	LatencyMs    int
	ErrorRate    float64
	RateLimit    int
	ErrorType    ErrorCode
	StatusCode   int
	mu           sync.RWMutex
	requestCount int
	lastReset    time.Time
}

var (
	gateways = map[string]*GatewayConfig{
		// Realistic Provider APIs
		"stripe": {
			Name:       "stripe",
			Type:       "provider",
			LatencyMs:  100,
			ErrorRate:  0.05,
			RateLimit:  100,
			ErrorType:  ErrInsufficientFunds,
			StatusCode: 402,
			lastReset:  time.Now(),
		},
		"razorpay": {
			Name:       "razorpay",
			Type:       "provider",
			LatencyMs:  120,
			ErrorRate:  0.07,
			RateLimit:  80,
			ErrorType:  ErrGatewayTimeout,
			StatusCode: 504,
			lastReset:  time.Now(),
		},
		"klarna": {
			Name:       "klarna",
			Type:       "provider",
			LatencyMs:  200,
			ErrorRate:  0.03,
			RateLimit:  50,
			ErrorType:  ErrAuthFailed,
			StatusCode: 401,
			lastReset:  time.Now(),
		},
		"onfido": {
			Name:       "onfido",
			Type:       "provider",
			LatencyMs:  300,
			ErrorRate:  0.02,
			RateLimit:  30,
			ErrorType:  ErrComplianceFailed,
			StatusCode: 422,
			lastReset:  time.Now(),
		},
		// Simple Test Gateways (like original test1, test2, test3)
		"test1": {
			Name:       "test1",
			Type:       "test",
			LatencyMs:  100,
			ErrorRate:  0.1,
			RateLimit:  0, // unlimited
			ErrorType:  ErrGatewayTimeout,
			StatusCode: 504,
			lastReset:  time.Now(),
		},
		"test2": {
			Name:       "test2",
			Type:       "test",
			LatencyMs:  200,
			ErrorRate:  0.2,
			RateLimit:  0,
			ErrorType:  ErrInsufficientFunds,
			StatusCode: 402,
			lastReset:  time.Now(),
		},
		"test3": {
			Name:       "test3",
			Type:       "test",
			LatencyMs:  50,
			ErrorRate:  0.05,
			RateLimit:  0,
			ErrorType:  ErrConnectionTimeout,
			StatusCode: 504,
			lastReset:  time.Now(),
		},
	}
	gatewaysMu sync.RWMutex
)

// UpdateConfig updates gateway configuration
func (gc *GatewayConfig) UpdateConfig(latencyMs int, errorRate float64, rateLimit int, errorType ErrorCode, statusCode int) {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	if latencyMs >= 0 {
		gc.LatencyMs = latencyMs
	}
	if errorRate >= 0 && errorRate <= 1 {
		gc.ErrorRate = errorRate
	}
	if rateLimit >= 0 {
		gc.RateLimit = rateLimit
	}
	if errorType != "" {
		gc.ErrorType = errorType
	}
	if statusCode > 0 {
		gc.StatusCode = statusCode
	}
}

// CheckRateLimit checks if rate limit is exceeded
func (gc *GatewayConfig) CheckRateLimit() bool {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	if gc.RateLimit == 0 {
		return false
	}

	now := time.Now()
	if now.Sub(gc.lastReset) >= time.Second {
		gc.requestCount = 0
		gc.lastReset = now
	}

	gc.requestCount++
	return gc.requestCount > gc.RateLimit
}

// ============================================================================
// STRIPE PROVIDER
// ============================================================================

type StripeChargeRequest struct {
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
	Source      string `json:"source"`
	Description string `json:"description"`
}

type StripeChargeResponse struct {
	ID             string `json:"id"`
	Object         string `json:"object"`
	Amount         int64  `json:"amount"`
	Currency       string `json:"currency"`
	Status         string `json:"status"`
	Paid           bool   `json:"paid"`
	Created        int64  `json:"created"`
	FailureCode    string `json:"failure_code,omitempty"`
	FailureMessage string `json:"failure_message,omitempty"`
}

type StripeRefundResponse struct {
	ID     string `json:"id"`
	Object string `json:"object"`
	Amount int64  `json:"amount"`
	Status string `json:"status"`
}

func stripeChargeHandler(w http.ResponseWriter, r *http.Request) {
	gatewaysMu.RLock()
	config := gateways["stripe"]
	gatewaysMu.RUnlock()

	if config.CheckRateLimit() {
		simulateError(w, ErrRateLimited, http.StatusTooManyRequests, "STRIPE")
		return
	}

	config.mu.RLock()
	latency := config.LatencyMs
	errorRate := config.ErrorRate
	errorType := config.ErrorType
	statusCode := config.StatusCode
	config.mu.RUnlock()

	time.Sleep(time.Duration(latency) * time.Millisecond)

	body, _ := io.ReadAll(r.Body)
	defer r.Body.Close()

	var req StripeChargeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "invalid_request_error",
			"message": "Invalid JSON body",
		})
		return
	}

	if rand.Float64() < errorRate {
		simulateError(w, errorType, statusCode, "STRIPE")
		return
	}

	resp := StripeChargeResponse{
		ID:       "ch_" + generateID(24),
		Object:   "charge",
		Amount:   req.Amount,
		Currency: req.Currency,
		Status:   "succeeded",
		Paid:     true,
		Created:  time.Now().Unix(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
	log.Printf("[STRIPE] SUCCESS: Charged %d %s", req.Amount, req.Currency)
}

func stripeRefundHandler(w http.ResponseWriter, r *http.Request) {
	time.Sleep(time.Duration(40+rand.Intn(80)) * time.Millisecond)

	resp := StripeRefundResponse{
		ID:     "re_" + generateID(24),
		Object: "refund",
		Amount: 5000,
		Status: "succeeded",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
	log.Println("[STRIPE] Refund processed")
}

// ============================================================================
// RAZORPAY PROVIDER
// ============================================================================

type RazorpayChargeRequest struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	Email    string `json:"email"`
	Contact  string `json:"contact"`
}

type RazorpayChargeResponse struct {
	ID          string `json:"id"`
	Entity      string `json:"entity"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
	Status      string `json:"status"`
	Method      string `json:"method"`
	Description string `json:"description"`
	Captured    bool   `json:"captured"`
	CreatedAt   int64  `json:"created_at"`
}

func razorpayChargeHandler(w http.ResponseWriter, r *http.Request) {
	gatewaysMu.RLock()
	config := gateways["razorpay"]
	gatewaysMu.RUnlock()

	if config.CheckRateLimit() {
		simulateError(w, ErrRateLimited, http.StatusTooManyRequests, "RAZORPAY")
		return
	}

	config.mu.RLock()
	latency := config.LatencyMs
	errorRate := config.ErrorRate
	errorType := config.ErrorType
	statusCode := config.StatusCode
	config.mu.RUnlock()

	time.Sleep(time.Duration(latency) * time.Millisecond)

	body, _ := io.ReadAll(r.Body)
	defer r.Body.Close()

	var req RazorpayChargeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error":       "BAD_REQUEST_ERROR",
			"description": "Invalid JSON",
		})
		return
	}

	if rand.Float64() < errorRate {
		simulateError(w, errorType, statusCode, "RAZORPAY")
		return
	}

	resp := RazorpayChargeResponse{
		ID:          "pay_" + generateID(14),
		Entity:      "payment",
		Amount:      req.Amount,
		Currency:    req.Currency,
		Status:      "captured",
		Method:      "card",
		Description: "Payment",
		Captured:    true,
		CreatedAt:   time.Now().Unix(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
	log.Printf("[RAZORPAY] SUCCESS: Charged %d %s", req.Amount, req.Currency)
}

// ============================================================================
// KLARNA PROVIDER
// ============================================================================

type KlarnaSessionRequest struct {
	PurchaseAmount   int64  `json:"purchase_amount"`
	PurchaseCurrency string `json:"purchase_currency"`
	Locale           string `json:"locale"`
}

type KlarnaSessionResponse struct {
	SessionID      string   `json:"session_id"`
	ClientToken    string   `json:"client_token"`
	PaymentMethods []string `json:"payment_method_categories"`
}

func klarnaSessionHandler(w http.ResponseWriter, r *http.Request) {
	gatewaysMu.RLock()
	config := gateways["klarna"]
	gatewaysMu.RUnlock()

	if config.CheckRateLimit() {
		simulateError(w, ErrRateLimited, http.StatusTooManyRequests, "KLARNA")
		return
	}

	config.mu.RLock()
	latency := config.LatencyMs
	errorRate := config.ErrorRate
	errorType := config.ErrorType
	statusCode := config.StatusCode
	config.mu.RUnlock()

	time.Sleep(time.Duration(latency) * time.Millisecond)

	body, _ := io.ReadAll(r.Body)
	defer r.Body.Close()

	var req KlarnaSessionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error_code":     "BAD_VALUE",
			"error_messages": "Invalid request",
		})
		return
	}

	if rand.Float64() < errorRate {
		simulateError(w, errorType, statusCode, "KLARNA")
		return
	}

	resp := KlarnaSessionResponse{
		SessionID:      "klarna_" + generateID(32),
		ClientToken:    "token_" + generateID(64),
		PaymentMethods: []string{"pay_later", "pay_over_time", "pay_now"},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
	log.Println("[KLARNA] Session created")
}

// ============================================================================
// ONFIDO PROVIDER (KYC/Compliance)
// ============================================================================

type OnfidoCheckRequest struct {
	ApplicantID string   `json:"applicant_id"`
	ReportNames []string `json:"report_names"`
}

type OnfidoCheckResponse struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Result    string `json:"result"`
	SubResult string `json:"sub_result"`
	CreatedAt string `json:"created_at"`
}

func onfidoCheckHandler(w http.ResponseWriter, r *http.Request) {
	gatewaysMu.RLock()
	config := gateways["onfido"]
	gatewaysMu.RUnlock()

	if config.CheckRateLimit() {
		simulateError(w, ErrRateLimited, http.StatusTooManyRequests, "ONFIDO")
		return
	}

	config.mu.RLock()
	latency := config.LatencyMs
	errorRate := config.ErrorRate
	errorType := config.ErrorType
	statusCode := config.StatusCode
	config.mu.RUnlock()

	time.Sleep(time.Duration(latency) * time.Millisecond)

	body, _ := io.ReadAll(r.Body)
	defer r.Body.Close()

	var req OnfidoCheckRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "validation_error",
			"message": "Invalid request body",
		})
		return
	}

	if rand.Float64() < errorRate {
		simulateError(w, errorType, statusCode, "ONFIDO")
		return
	}

	resp := OnfidoCheckResponse{
		ID:        "check_" + generateID(32),
		Status:    "complete",
		Result:    "clear",
		SubResult: "clear",
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
	log.Println("[ONFIDO] KYC APPROVED")
}

// ============================================================================
// TEST GATEWAYS (test1, test2, test3) - Simple Generic Responses
// ============================================================================

func testGatewayHandler(w http.ResponseWriter, r *http.Request, gatewayName string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	gatewaysMu.RLock()
	config, exists := gateways[gatewayName]
	gatewaysMu.RUnlock()

	if !exists {
		http.Error(w, fmt.Sprintf("Gateway '%s' not found", gatewayName), http.StatusNotFound)
		return
	}

	// Read and discard request body
	io.ReadAll(r.Body)
	defer r.Body.Close()

	// Check rate limit
	if config.CheckRateLimit() {
		simulateError(w, ErrRateLimited, http.StatusTooManyRequests, strings.ToUpper(gatewayName))
		return
	}

	// Get current config values
	config.mu.RLock()
	latency := config.LatencyMs
	errorRate := config.ErrorRate
	errorType := config.ErrorType
	statusCode := config.StatusCode
	config.mu.RUnlock()

	// Simulate latency
	time.Sleep(time.Duration(latency) * time.Millisecond)

	// Simulate errors based on error rate
	if rand.Float64() < errorRate {
		simulateError(w, errorType, statusCode, strings.ToUpper(gatewayName))
	} else {
		// Success response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "success",
			"message": "Payment processed successfully",
		})
		log.Printf("[%s] SUCCESS", strings.ToUpper(gatewayName))
	}
}

// ============================================================================
// ERROR SIMULATION
// ============================================================================

func simulateError(w http.ResponseWriter, errorType ErrorCode, statusCode int, gatewayName string) {
	switch errorType {
	case ErrConnectionReset:
		log.Printf("[%s] SIMULATING CONNECTION RESET", gatewayName)
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "webserver doesn't support hijacking", http.StatusInternalServerError)
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		conn.Close()

	case ErrPanic:
		log.Printf("[%s] SIMULATING PANIC", gatewayName)
		panic(fmt.Sprintf("Simulated panic in %s", gatewayName))

	case ErrMalformedResponse, ErrInvalidJSON:
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status": "failed", "error": "malformed`))
		log.Printf("[%s] FAILED (Malformed JSON)", gatewayName)

	case ErrEmptyResponse:
		w.WriteHeader(http.StatusOK)
		log.Printf("[%s] FAILED (Empty Response)", gatewayName)

	case ErrSlowResponse:
		w.Header().Set("Content-Type", "application/json")
		resp := fmt.Sprintf(`{"status": "failed", "error_code": "%s"}`, errorType)
		for i := 0; i < len(resp); i++ {
			w.Write([]byte{resp[i]})
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			time.Sleep(100 * time.Millisecond)
		}
		log.Printf("[%s] FAILED (Slow Response)", gatewayName)

	case ErrRateLimited:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":      "failed",
			"error":       string(errorType),
			"message":     "Rate limit exceeded",
			"retry_after": 60,
		})
		log.Printf("[%s] FAILED (Rate Limited)", gatewayName)

	default:
		if statusCode == 0 {
			statusCode = http.StatusInternalServerError
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "failed",
			"error":   string(errorType),
			"message": getErrorMessage(errorType),
		})
		log.Printf("[%s] FAILED (%s) - Status %d", gatewayName, errorType, statusCode)
	}
}

func getErrorMessage(errorType ErrorCode) string {
	messages := map[ErrorCode]string{
		ErrInsufficientFunds: "Insufficient funds in account",
		ErrCardDeclined:      "Card was declined",
		ErrAuthFailed:        "Authentication failed",
		ErrGatewayTimeout:    "Gateway timeout",
		ErrProviderError:     "Provider error occurred",
		ErrProviderDown:      "Provider is currently down",
		ErrConnectionTimeout: "Connection timeout",
		ErrInternalError:     "Internal server error",
		ErrComplianceFailed:  "Compliance check failed",
		ErrKYCRequired:       "KYC verification required",
	}

	if msg, ok := messages[errorType]; ok {
		return msg
	}
	return string(errorType)
}

func generateID(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[rand.Intn(len(charset))]
	}
	return string(result)
}

// ============================================================================
// CONTROL API - Error Manipulation
// ============================================================================

func controlHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodGet {
		gatewaysMu.RLock()
		configs := make(map[string]interface{})
		for name, config := range gateways {
			config.mu.RLock()
			configs[name] = map[string]interface{}{
				"name":        config.Name,
				"type":        config.Type,
				"latency_ms":  config.LatencyMs,
				"error_rate":  config.ErrorRate,
				"rate_limit":  config.RateLimit,
				"error_type":  config.ErrorType,
				"status_code": config.StatusCode,
			}
			config.mu.RUnlock()
		}
		gatewaysMu.RUnlock()

		json.NewEncoder(w).Encode(map[string]interface{}{
			"gateways": configs,
			"available_error_types": []string{
				string(ErrInvalidRequest),
				string(ErrInsufficientFunds),
				string(ErrCardDeclined),
				string(ErrAuthFailed),
				string(ErrGatewayTimeout),
				string(ErrProviderError),
				string(ErrProviderDown),
				string(ErrConnectionReset),
				string(ErrConnectionTimeout),
				string(ErrMalformedResponse),
				string(ErrEmptyResponse),
				string(ErrSlowResponse),
				string(ErrInvalidJSON),
				string(ErrRateLimited),
				string(ErrInternalError),
				string(ErrPanic),
				string(ErrComplianceFailed),
				string(ErrKYCRequired),
			},
		})
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			Gateway    string    `json:"gateway"`
			LatencyMs  int       `json:"latency_ms"`
			ErrorRate  float64   `json:"error_rate"`
			RateLimit  int       `json:"rate_limit"`
			ErrorType  ErrorCode `json:"error_type"`
			StatusCode int       `json:"status_code"`
		}

		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()

		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		gatewaysMu.RLock()
		config, exists := gateways[req.Gateway]
		gatewaysMu.RUnlock()

		if !exists {
			http.Error(w, fmt.Sprintf("Gateway '%s' not found. Available: stripe, razorpay, klarna, onfido, test1, test2, test3", req.Gateway), http.StatusBadRequest)
			return
		}

		config.UpdateConfig(req.LatencyMs, req.ErrorRate, req.RateLimit, req.ErrorType, req.StatusCode)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Gateway configuration updated",
			"gateway": req.Gateway,
			"config": map[string]interface{}{
				"latency_ms":  config.LatencyMs,
				"error_rate":  config.ErrorRate,
				"rate_limit":  config.RateLimit,
				"error_type":  config.ErrorType,
				"status_code": config.StatusCode,
			},
		})
		log.Printf("Updated %s: latency=%dms, error_rate=%.2f%%, rate_limit=%d/s, error_type=%s",
			req.Gateway, req.LatencyMs, req.ErrorRate*100, req.RateLimit, req.ErrorType)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// ============================================================================
// ROUTER
// ============================================================================

func routeHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Gateway/Provider name required", http.StatusBadRequest)
		return
	}

	gateway := parts[0]

	switch gateway {
	case "stripe":
		if len(parts) > 1 {
			switch parts[1] {
			case "charges":
				stripeChargeHandler(w, r)
			case "refunds":
				stripeRefundHandler(w, r)
			default:
				http.NotFound(w, r)
			}
		} else {
			stripeChargeHandler(w, r)
		}
	case "razorpay":
		razorpayChargeHandler(w, r)
	case "klarna":
		klarnaSessionHandler(w, r)
	case "onfido":
		onfidoCheckHandler(w, r)
	case "test1", "test2", "test3":
		testGatewayHandler(w, r, gateway)
	case "control":
		controlHandler(w, r)
	case "health":
		healthHandler(w, r)
	default:
		http.Error(w, fmt.Sprintf("Unknown gateway/provider: %s", gateway), http.StatusNotFound)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	gatewaysMu.RLock()
	statuses := make(map[string]string)
	for name := range gateways {
		statuses[name] = "operational"
	}
	gatewaysMu.RUnlock()

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"gateways":  statuses,
		"timestamp": time.Now().Unix(),
	})
}

// ============================================================================
// MAIN
// ============================================================================

func main() {
	http.HandleFunc("/", routeHandler)

	log.Println("╔════════════════════════════════════════════════════════════════╗")
	log.Println("║        UNIFIED GATEWAY & PROVIDER SIMULATION SERVER           ║")
	log.Println("╚════════════════════════════════════════════════════════════════╝")
	log.Println("")
	log.Println("🌐 Server running on: http://localhost:3001")
	log.Println("")
	log.Println("📦 REALISTIC PROVIDER APIs:")
	log.Println("  ├─ Stripe:   http://localhost:3001/stripe")
	log.Println("  ├─ Razorpay: http://localhost:3001/razorpay")
	log.Println("  ├─ Klarna:   http://localhost:3001/klarna")
	log.Println("  └─ Onfido:   http://localhost:3001/onfido")
	log.Println("")
	log.Println("🧪 TEST GATEWAYS (Simple APIs):")
	log.Println("  ├─ Test1:    http://localhost:3001/test1")
	log.Println("  ├─ Test2:    http://localhost:3001/test2")
	log.Println("  └─ Test3:    http://localhost:3001/test3")
	log.Println("")
	log.Println("⚙️  CONTROL ENDPOINTS:")
	log.Println("  ├─ GET  /control  → View all configurations")
	log.Println("  ├─ POST /control  → Update gateway config")
	log.Println("  └─ GET  /health   → Health check")
	log.Println("")
	log.Println("📝 Example: Update test1 error rate to 50%")
	log.Println(`  curl -X POST http://localhost:3001/control \`)
	log.Println(`    -H "Content-Type: application/json" \`)
	log.Println(`    -d '{"gateway":"test1","error_rate":0.5,"latency_ms":300}'`)
	log.Println("")
	log.Println("═══════════════════════════════════════════════════════════════")

	if err := http.ListenAndServe(":3001", nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
