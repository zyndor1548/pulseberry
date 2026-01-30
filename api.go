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

var ctx context.Context
var rdb *redis.Client
var serverPool *ServerPool

func PaymentKey(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error reading body: %v", err), http.StatusBadRequest)
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
			http.Error(w, "Failed to cache payment ID", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"payment_id": paymentID,
		})

	case http.MethodDelete:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error reading body: %v", err), http.StatusBadRequest)
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
	switch r.Method {
	case http.MethodPost:
		w.Header().Set("Content-Type", "application/json")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":  fmt.Sprintf("Error reading body: %v", err),
				"status": FAILED.String(),
			})
			return
		}
		defer r.Body.Close()
		type PaymentRequest struct {
			Id        string `json:"id"`
			Amount    int    `json:"amount"`
			PaymentID string `json:"payment_id"`
		}
		var req PaymentRequest
		err = json.Unmarshal(body, &req)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":  fmt.Sprintf("Invalid JSON: %v", err),
				"status": GetState(req.PaymentID).String(),
			})
			return
		}

		if req.PaymentID == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":  "payment_id is required",
				"status": FAILED.String(),
			})
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
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":  "payment key not found",
				"status": "FAILED",
			})
			return
		}
		if req.PaymentID != cachedPaymentID {
			SetState(req.PaymentID, FAILED)
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":  "payment_id mismatch",
				"status": GetState(req.PaymentID).String(),
			})
			return
		}

		SetState(req.PaymentID, INITIATED)
		SetState(req.PaymentID, PROCESSING)

		selectedServer, err := serverPool.SelectServer()
		if err != nil {
			SetState(req.PaymentID, FAILED)
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":  "No healthy payment gateway servers available",
				"status": GetState(req.PaymentID).String(),
			})
			return
		}

		log.Printf("Selected server %s (score: %.2f) for payment %s", selectedServer.ServerURL, selectedServer.GetScore(), req.PaymentID)

		paymentData := map[string]interface{}{
			"id":     req.Id,
			"amount": req.Amount,
		}
		jsonData, err := json.Marshal(paymentData)
		if err != nil {
			SetState(req.PaymentID, FAILED)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":  "Failed to marshal JSON",
				"status": GetState(req.PaymentID).String(),
			})
			return
		}

		startTime := time.Now()
		gatewayURL := selectedServer.ServerURL
		response, err := http.Post(gatewayURL, "application/json", bytes.NewBuffer(jsonData))
		latency := time.Since(startTime)

		if err != nil {
			errorType := ErrorTypeNetwork
			serverPool.RecordRequestResult(selectedServer.ServerURL, latency, false, &errorType, err.Error())

			SetState(req.PaymentID, FAILED)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":  "Failed to send request to payment gateway",
				"status": GetState(req.PaymentID).String(),
			})
			return
		}
		defer response.Body.Close()

		var dat map[string]interface{}
		if err := json.NewDecoder(response.Body).Decode(&dat); err != nil {
			errorType := ErrorTypeGateway
			serverPool.RecordRequestResult(selectedServer.ServerURL, latency, false, &errorType, "Failed to decode response")

			SetState(req.PaymentID, FAILED)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":  "Failed to decode payment gateway response",
				"status": GetState(req.PaymentID).String(),
			})
			return
		}

		var success bool
		var errorType *ErrorType

		if responseStatus, ok := dat["status"].(string); ok {
			if responseStatus == "success" {
				SetState(req.PaymentID, SUCCESS)
				success = true
			} else {
				SetState(req.PaymentID, FAILED)
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
			SetState(req.PaymentID, FAILED)
			success = false
			et := ErrorTypeGateway
			errorType = &et
			log.Printf("Warning: Payment gateway response did not contain 'status'. Defaulting to FAILED for payment ID: %s", req.PaymentID)
		}

		errorMsg := ""
		if !success && errorType != nil {
			if errMsgVal, ok := dat["error"].(string); ok {
				errorMsg = errMsgVal
			}
		}
		serverPool.RecordRequestResult(selectedServer.ServerURL, latency, success, errorType, errorMsg)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":     GetState(req.PaymentID).String(),
			"payment_id": cachedPaymentID,
		})
		log.Printf("Payment %s completed with status: %v (latency: %v, server: %s)", req.PaymentID, GetState(req.PaymentID).String(), latency, selectedServer.ServerURL)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func MetricsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	metrics := map[string]interface{}{
		"servers":      serverPool.GetAllServersStatus(),
		"server_count": serverPool.GetServerCount(),
		"timestamp":    time.Now().Format(time.RFC3339),
	}

	json.NewEncoder(w).Encode(metrics)
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	ctx = context.Background()

	rdb = redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_ADDR"),
		Username: "default",
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})

	serverPool = NewServerPool(DefaultScoringConfig())

	gatewayServers := []string{
		"http://localhost:3001",
		// "http://localhost:3002",
		// "http://localhost:3003",
	}

	for _, server := range gatewayServers {
		serverPool.AddServer(server)
	}

	serverPool.StartPeriodicScoreUpdate()

	defer serverPool.StopPeriodicScoreUpdate()

	http.HandleFunc("/payment", Payment)
	http.HandleFunc("/paymentKey", PaymentKey)
	http.HandleFunc("/metrics", MetricsHandler)

	log.Println("Server starting on port 3000...")
	if err := http.ListenAndServe(":3000", nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
