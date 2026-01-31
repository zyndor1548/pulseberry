package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

// Global variables
var (
	ctx              context.Context
	rdb              *redis.Client
	serverPool       *ServerPool       // Legacy - kept for backward compatibility
	providerRegistry *ProviderRegistry // New provider registry
	apiKeyStore      *APIKeyStore
	rateLimiter      *RateLimiter
	appLogger        *StructuredLogger
)

// ComplianceThreshold defines the amount above which compliance checks are required
const ComplianceThreshold = 1000000 // $10,000 in cents

func PaymentKey(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(NewErrorResponse(
				ErrInvalidRequest,
				"Failed to read request body",
				"FAILED",
				err.Error(),
			))
			return
		}
		defer r.Body.Close()
		type CheckRequest struct {
			Id     string `json:"id"`
			Amount int    `json:"amount"`
		}
		var req CheckRequest
		err = json.Unmarshal(body, &req)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
			return
		}

		hashData := map[string]interface{}{
			"id":     req.Id,
			"amount": req.Amount,
		}
		hashJSON, _ := json.Marshal(hashData)
		requestHash := SHA256Hash(string(hashJSON))

		cachedPaymentID, err := rdb.Get(ctx, requestHash).Result()
		if err == nil && cachedPaymentID != "" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"payment_id": cachedPaymentID,
			})
			return
		}
		paymentID := "pay_" + uuid.NewString()
		err = rdb.Set(ctx, requestHash, paymentID, 0).Err()
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(NewErrorResponse(
				ErrInternalError,
				"Failed to cache payment ID",
				"FAILED",
				err.Error(),
			))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"payment_id": paymentID,
		})

	case http.MethodDelete:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(NewErrorResponse(
				ErrInvalidRequest,
				"Failed to read request body",
				"FAILED",
				err.Error(),
			))
			return
		}
		defer r.Body.Close()
		type CheckRequest struct {
			Id     string `json:"id"`
			Amount int    `json:"amount"`
		}
		var req CheckRequest
		err = json.Unmarshal(body, &req)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
			return
		}
		hashData := map[string]interface{}{
			"id":     req.Id,
			"amount": req.Amount,
		}
		hashJSON, _ := json.Marshal(hashData)
		requestHash := SHA256Hash(string(hashJSON))
		cachedPaymentID, err := rdb.Get(ctx, requestHash).Result()
		if err != nil {
			http.Error(w, "Payment key not found", http.StatusNotFound)
			return
		}

		err = rdb.Del(ctx, requestHash).Err()
		if err != nil {
			http.Error(w, "Failed to delete payment key", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message":    "Payment key deleted successfully",
			"payment_id": cachedPaymentID,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func Payment(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	r = r.WithContext(ctx)

	// Get correlation ID from context (set by middleware)
	correlationID, _ := r.Context().Value("correlation_id").(string)

	switch r.Method {
	case http.MethodPost:
		w.Header().Set("Content-Type", "application/json")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(NewErrorResponse(
				ErrInvalidRequest,
				"Failed to read request body",
				FAILED.String(),
				err.Error(),
			))
			return
		}
		defer r.Body.Close()
		type PaymentRequest struct {
			Id        string `json:"id"`
			Amount    int    `json:"amount"`
			PaymentID string `json:"payment_id"`
			Currency  string `json:"currency"`
			UserID    string `json:"user_id"`
		}
		var req PaymentRequest
		err = json.Unmarshal(body, &req)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(NewErrorResponse(
				ErrInvalidRequest,
				"Invalid JSON format",
				FAILED.String(),
				err.Error(),
			))
			return
		}

		// Set default currency if not provided
		if req.Currency == "" {
			req.Currency = "USD"
		}

		if req.PaymentID == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(NewErrorResponse(
				ErrPaymentIDRequired,
				"Payment ID is required",
				FAILED.String(),
				"",
			))
			return
		}

		hashData := map[string]interface{}{
			"id":     req.Id,
			"amount": req.Amount,
		}

		hashJSON, _ := json.Marshal(hashData)
		requestHash := SHA256Hash(string(hashJSON))

		cachedPaymentID, err := rdb.Get(ctx, requestHash).Result()
		if err != nil || cachedPaymentID == "" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(NewErrorResponse(
				ErrPaymentKeyNotFound,
				"Payment key not found or expired",
				"FAILED",
				"Please generate a new payment key",
			))
			return
		}
		if req.PaymentID != cachedPaymentID {
			SetState(req.PaymentID, FAILED)
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(NewErrorResponse(
				ErrPaymentIDMismatch,
				"Payment ID does not match",
				GetState(req.PaymentID).String(),
				"The provided payment ID does not match the cached value",
			))
			return
		}

		currentState := GetState(req.PaymentID)

		if currentState == SUCCESS || currentState == FAILED {

			cachedResult, err := rdb.Get(ctx, "payment_result:"+req.PaymentID).Result()
			if err == nil && cachedResult != "" {
				w.Header().Set("X-Idempotent-Replay", "true")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(cachedResult))
				return
			}

			w.Header().Set("X-Idempotent-Replay", "true")
			json.NewEncoder(w).Encode(NewSuccessResponse(
				currentState.String(),
				req.PaymentID,
				map[string]interface{}{
					"message": "Payment already processed",
				},
			))
			return
		}

		if currentState == PROCESSING {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(NewErrorResponse(
				ErrInternalError,
				"Payment is currently being processed",
				currentState.String(),
				"Please wait for the current payment to complete",
			))
			return
		}

		// Check if compliance check is required
		if int64(req.Amount) >= ComplianceThreshold && req.UserID != "" {
			appLogger.Info("High-value transaction detected, performing compliance check", map[string]interface{}{
				"correlation_id": correlationID,
				"payment_id":     req.PaymentID,
				"amount":         req.Amount,
				"user_id":        req.UserID,
			})

			// Perform compliance check
			complianceReq := &ComplianceCheckRequest{
				UserID:         req.UserID,
				CheckType:      ComplianceCheckKYC,
				IdempotencyKey: req.PaymentID + "_kyc",
			}

			complianceResp, err := providerRegistry.PerformComplianceCheck(ctx, complianceReq)
			if err != nil || (complianceResp != nil && complianceResp.Status != ComplianceStatusApproved) {
				SetState(req.PaymentID, FAILED)
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(NewErrorResponse(
					ErrKYCRequired,
					"Compliance check failed or required",
					FAILED.String(),
					"KYC verification is required for high-value transactions",
				))
				return
			}

			appLogger.Info("Compliance check passed", map[string]interface{}{
				"correlation_id": correlationID,
				"payment_id":     req.PaymentID,
				"check_id":       complianceResp.CheckID,
			})
		}

		SetState(req.PaymentID, INITIATED)
		SetState(req.PaymentID, PROCESSING)

		json.NewEncoder(w).Encode(NewSuccessResponse(
			PROCESSING.String(),
			req.PaymentID,
			map[string]interface{}{
				"message": "Payment processing started",
			},
		))

		go processPaymentAsync(req.Id, req.Amount, req.PaymentID, req.Currency, correlationID)
		return
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func processPaymentAsync(id string, amount int, paymentID, currency, correlationID string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in processPaymentAsync for %s: %v", paymentID, r)
			notifyClient(paymentID, FAILED, fmt.Errorf("internal processing error"))
		}
	}()

	paymentData := map[string]interface{}{
		"id":       id,
		"amount":   amount,
		"currency": currency,
	}
	jsonData, err := json.Marshal(paymentData)
	if err != nil {
		SetState(paymentID, FAILED)
		notifyClient(paymentID, FAILED, nil)
		return
	}

	// Try legacy serverPool first (for backward compatibility during transition)
	maxRetries := serverPool.GetServerCount()
	if maxRetries == 0 {
		SetState(paymentID, FAILED)
		notifyClient(paymentID, FAILED, fmt.Errorf("no healthy servers"))
		return
	}

	var lastError error
	var selectedServer *ServerMetrics
	var response *http.Response
	var latency time.Duration
	var responseBody []byte
	var dat map[string]interface{}

	for attempt := 0; attempt < maxRetries; attempt++ {
		selectedServer, err = serverPool.SelectServer()
		if err != nil {
			lastError = err
			break
		}

		startTime := time.Now()
		gatewayURL := selectedServer.ServerURL

		appLogger.Info("Routing payment to gateway", map[string]interface{}{
			"correlation_id": correlationID,
			"payment_id":     paymentID,
			"gateway":        gatewayURL,
			"attempt":        attempt + 1,
		})

		response, err = http.Post(gatewayURL, "application/json", bytes.NewBuffer(jsonData))
		latency = time.Since(startTime)

		if err != nil {
			errorType := ErrorTypeNetwork
			serverPool.RecordRequestResult(paymentID, selectedServer.ServerURL, latency, false, &errorType, err.Error())

			appLogger.Error("Gateway request failed", map[string]interface{}{
				"correlation_id": correlationID,
				"payment_id":     paymentID,
				"gateway":        gatewayURL,
				"error":          err.Error(),
				"latency_ms":     latency.Milliseconds(),
			})

			lastError = err
			continue
		}
		responseBody, err = io.ReadAll(response.Body)
		response.Body.Close()

		if err != nil {
			errorType := ErrorTypeGateway
			serverPool.RecordRequestResult(paymentID, selectedServer.ServerURL, latency, false, &errorType, "Failed to read response body")
			lastError = err
			continue
		}

		dat = make(map[string]interface{})
		if err := json.Unmarshal(responseBody, &dat); err != nil {
			errorType := ErrorTypeGateway
			serverPool.RecordRequestResult(paymentID, selectedServer.ServerURL, latency, false, &errorType, "Invalid JSON response")
			lastError = err
			continue
		}

		var success bool
		var errorType *ErrorType

		if responseStatus, ok := dat["status"].(string); ok {
			if responseStatus == "success" {
				SetState(paymentID, SUCCESS)
				success = true

				appLogger.Info("Payment successful", map[string]interface{}{
					"correlation_id": correlationID,
					"payment_id":     paymentID,
					"gateway":        gatewayURL,
					"latency_ms":     latency.Milliseconds(),
				})
			} else {
				SetState(paymentID, FAILED)
				success = false
				if response.StatusCode >= 500 {
					et := ErrorTypeGateway
					errorType = &et
				} else {
					et := ErrorTypeBank
					errorType = &et
				}
			}
		} else {
			SetState(paymentID, FAILED)
			success = false
			et := ErrorTypeGateway
			errorType = &et
		}

		errorMsg := ""
		if !success && errorType != nil {
			if errMsgVal, ok := dat["error"].(string); ok {
				errorMsg = errMsgVal
			}
		}
		serverPool.RecordRequestResult(paymentID, selectedServer.ServerURL, latency, success, errorType, errorMsg)

		responseStatus, ok := dat["status"].(string)
		if success || (ok && responseStatus == "failed") {
			break
		}

		if response != nil && response.StatusCode >= 500 {
			continue
		}
		break
	}

	if GetState(paymentID) != SUCCESS && GetState(paymentID) != FAILED {
		SetState(paymentID, FAILED)
	}

	finalStatus := GetState(paymentID)
	paymentResponse := NewSuccessResponse(
		finalStatus.String(),
		paymentID,
		map[string]interface{}{
			"gateway":    nil,
			"latency_ms": latency.Milliseconds(),
		},
	)

	if selectedServer != nil {
		paymentResponse.Data = map[string]interface{}{
			"gateway":    selectedServer.ServerURL,
			"latency_ms": latency.Milliseconds(),
		}
	}

	responseJSON, jsonErr := json.Marshal(paymentResponse)
	if jsonErr == nil {
		rdb.Set(ctx, "payment_result:"+paymentID, string(responseJSON), 24*time.Hour)
	}

	wsManager.Notify(paymentID, paymentResponse)

	if finalStatus == FAILED && lastError != nil {
		log.Printf("Background process for %s failed after all retries. Last error: %v", paymentID, lastError)
	} else {
		log.Printf("Background process for %s completed with status: %v", paymentID, finalStatus)
	}
}

func notifyClient(paymentID string, state State, err error) {
	msg := NewErrorResponse(ErrInternalError, "Payment failed", state.String(), "")
	if err != nil {
		msg.Details = err.Error()
	}

	if responseJSON, jsonErr := json.Marshal(msg); jsonErr == nil {
		rdb.Set(ctx, "payment_result:"+paymentID, string(responseJSON), 24*time.Hour)
	}

	wsManager.Notify(paymentID, msg)
}

func MetricsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	metrics := map[string]interface{}{
		"servers":           serverPool.GetAllServersStatus(),
		"server_count":      serverPool.GetServerCount(),
		"provider_registry": providerRegistry.GetAllProviderStatus(),
		"timestamp":         time.Now().Format(time.RFC3339),
	}

	json.NewEncoder(w).Encode(metrics)
}

func LogsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	logs, err := GetLogs()
	if err != nil {
		http.Error(w, "Failed to fetch logs", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	ctx = context.Background()

	// Initialize structured logger
	InitLogger(LogLevelInfo, true)
	appLogger = GetLogger()
	appLogger.Info("Starting FinTech Integration Mesh", map[string]interface{}{
		"version": "2.0.0-mvp",
	})

	// Initialize Redis
	rdb = redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_ADDR"),
		Username: "default",
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})

	// Initialize Database
	_, err = ConnectDatabase()
	if err != nil {
		log.Printf("Warning: Failed to connect to database: %v", err)
		log.Println("Continuing without database logging...")
	} else {
		log.Println("Database connected successfully")
		CreateDatabases()
		defer DisconnectDatabase()
	}

	// Initialize legacy server pool (for backward compatibility)
	serverPool = NewServerPool(DefaultScoringConfig())

	gatewayServers := []string{
		"http://localhost:3001/stripe",
		"http://localhost:3001/razorpay",
		"http://localhost:3001/klarna",
		"http://localhost:3001/test1",
		"http://localhost:3001/test2",
	}

	for _, server := range gatewayServers {
		serverPool.AddServer(server)
	}

	serverPool.StartPeriodicScoreUpdate()
	defer serverPool.StopPeriodicScoreUpdate()

	// Initialize provider registry
	providerRegistry = NewProviderRegistry()

	// Register payment providers
	providerRegistry.RegisterPaymentProvider(&ProviderConfig{
		Provider: NewMockStripeProvider("http://localhost:3001/stripe"),
		Enabled:  true,
		Priority: PriorityPrimary,
		SLA: SLAConfig{
			MaxLatencyP95Ms: 500,
			MinSuccessRate:  0.95,
		},
	})

	providerRegistry.RegisterPaymentProvider(&ProviderConfig{
		Provider: NewMockRazorpayProvider("http://localhost:3001/razorpay"),
		Enabled:  true,
		Priority: PrioritySecondary,
		SLA: SLAConfig{
			MaxLatencyP95Ms: 600,
			MinSuccessRate:  0.90,
		},
	})

	providerRegistry.RegisterPaymentProvider(&ProviderConfig{
		Provider: NewMockKlarnaProvider("http://localhost:3001/klarna"),
		Enabled:  true,
		Priority: PriorityTertiary,
		SLA: SLAConfig{
			MaxLatencyP95Ms: 700,
			MinSuccessRate:  0.85,
		},
	})

	// Register compliance provider
	providerRegistry.RegisterComplianceProvider(&ComplianceProviderConfig{
		Provider: NewMockOnfidoProvider("http://localhost:3001/onfido"),
		Enabled:  true,
	})

	appLogger.Info("Provider registry initialized", map[string]interface{}{
		"payment_providers":    3,
		"compliance_providers": 1,
	})

	// Initialize API key store (for demo purposes)
	apiKeyStore = NewAPIKeyStore()
	apiKeyStore.AddKey(&APIKey{
		Key:       "demo_key_12345",
		Secret:    "demo_secret_abcdef",
		Name:      "Demo API Key",
		Enabled:   true,
		CreatedAt: time.Now(),
	})

	// Initialize rate limiter
	rateLimiter = NewRateLimiter(rdb)
	rateLimiter.SetQuota("demo_key_12345", RateQuota{
		RequestsPerMinute: 100,
		BurstSize:         10,
	})

	// Setup middleware chain
	mux := http.NewServeMux()
	mux.HandleFunc("/payment", Payment)
	mux.HandleFunc("/paymentKey", PaymentKey)
	mux.HandleFunc("/metrics", MetricsHandler)
	mux.HandleFunc("/logs", LogsHandler)
	mux.HandleFunc("/ws", wsManager.HandleWS)

	// Admin endpoints
	mux.HandleFunc("/admin/providers", AdminProvidersHandler)
	mux.HandleFunc("/admin/providers/enable", AdminProviderEnableHandler)
	mux.HandleFunc("/admin/providers/disable", AdminProviderDisableHandler)
	mux.HandleFunc("/admin/circuit-breaker/reset", AdminCircuitBreakerResetHandler)
	mux.HandleFunc("/health", HealthCheckHandler)

	// Apply middleware (order matters!)
	handler := CorrelationIDMiddleware(mux)        // 1. Add correlation ID
	handler = RequestValidationMiddleware(handler) // 2. Validate request size/format
	// Note: Auth and RateLimit middleware disabled for backward compatibility
	// To enable: uncomment the lines below
	// handler = RateLimitMiddleware(rateLimiter)(handler)   // 3. Rate limiting
	// handler = AuthMiddleware(apiKeyStore)(handler)         // 4. Authentication
	handler = TimeoutMiddleware(30 * time.Second)(handler) // 5. Global timeout

	appLogger.Info("Server starting", map[string]interface{}{
		"port": 3000,
		"features": []string{
			"provider_registry",
			"circuit_breakers",
			"structured_logging",
			"latency_percentiles",
			"compliance_checks",
		},
	})

	log.Println("Server starting on port 3000...")
	if err := http.ListenAndServe(":3000", handler); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
