package main

import (
	"errors"
	"log"
	"math/rand"
	"sync"
	"time"
)

type ServerPool struct {
	servers      map[string]*ServerMetrics
	config       *ScoringConfig
	mu           sync.RWMutex
	updateTicker *time.Ticker
	stopChan     chan bool
	isRunning    bool
}

func NewServerPool(config *ScoringConfig) *ServerPool {
	if config == nil {
		config = DefaultScoringConfig()
	}

	return &ServerPool{
		servers:  make(map[string]*ServerMetrics),
		config:   config,
		stopChan: make(chan bool),
	}
}

func (sp *ServerPool) AddServer(serverURL string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	if _, exists := sp.servers[serverURL]; !exists {
		sp.servers[serverURL] = NewServerMetrics(serverURL)
		log.Printf("Added server to pool: %s (initial score: %.2f)", serverURL, 100.0)
	}
}

func (sp *ServerPool) RemoveServer(serverURL string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	if _, exists := sp.servers[serverURL]; exists {
		delete(sp.servers, serverURL)
		log.Printf("Removed server from pool: %s", serverURL)
	}
}

func (sp *ServerPool) GetServer(serverURL string) (*ServerMetrics, error) {
	sp.mu.RLock()
	defer sp.mu.RUnlock()

	server, exists := sp.servers[serverURL]
	if !exists {
		return nil, errors.New("server not found")
	}
	return server, nil
}

func (sp *ServerPool) SelectServer() (*ServerMetrics, error) {
	sp.mu.RLock()
	defer sp.mu.RUnlock()

	if len(sp.servers) == 0 {
		return nil, errors.New("no servers available")
	}

	totalScore := 0.0
	serverList := make([]*ServerMetrics, 0, len(sp.servers))

	for _, server := range sp.servers {
		score := server.GetScore()
		if score > 0 {
			totalScore += score
			serverList = append(serverList, server)
		}
	}

	if totalScore == 0 || len(serverList) == 0 {
		log.Println("Warning: All servers have score 0, using fallback selection")
		for _, server := range sp.servers {
			return server, nil
		}
		return nil, errors.New("no healthy servers available")
	}

	randomValue := rand.Float64() * totalScore
	currentSum := 0.0

	for _, server := range serverList {
		currentSum += server.GetScore()
		if currentSum >= randomValue {
			return server, nil
		}
	}
	return serverList[0], nil
}

func (sp *ServerPool) RecordRequestResult(paymentID, serverURL string, latency time.Duration, success bool, errorType *ErrorType, errorMsg string) {
	server, err := sp.GetServer(serverURL)
	if err != nil {
		log.Printf("Error recording request result: %v", err)
		return
	}

	server.RecordRequest(latency, success)

	if !success && errorType != nil {
		server.RecordError(*errorType, errorMsg)
	}

	currentScore := server.GetScore()

	errorTypeStr := ""
	if errorType != nil {
		switch *errorType {
		case ErrorTypeGateway:
			errorTypeStr = "GATEWAY"
		case ErrorTypeBank:
			errorTypeStr = "BANK"
		case ErrorTypeNetwork:
			errorTypeStr = "NETWORK"
		case ErrorTypeClient:
			errorTypeStr = "CLIENT"
		}
	}

	latencyMs := latency.Milliseconds()
	if err := LogRequestMetrics(paymentID, serverURL, latencyMs, success, currentScore, errorTypeStr, errorMsg); err != nil {
		log.Printf("Failed to log request metrics to database: %v", err)
	}
}

func (sp *ServerPool) StartPeriodicScoreUpdate() {
	sp.mu.Lock()
	if sp.isRunning {
		sp.mu.Unlock()
		return
	}
	sp.isRunning = true
	sp.updateTicker = time.NewTicker(sp.config.ScoreUpdatePeriod)
	sp.mu.Unlock()

	go func() {
		log.Printf("score update interval: %v", sp.config.ScoreUpdatePeriod)
		for {
			select {
			case <-sp.updateTicker.C:
				sp.updateAllScores()
			case <-sp.stopChan:
				sp.updateTicker.Stop()
				log.Println("Stopped periodic score updates")
				return
			}
		}
	}()
}

func (sp *ServerPool) StopPeriodicScoreUpdate() {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	if sp.isRunning {
		sp.stopChan <- true
		sp.isRunning = false
	}
}
func (sp *ServerPool) updateAllScores() {
	sp.mu.RLock()
	defer sp.mu.RUnlock()

	for _, server := range sp.servers {
		oldScore := server.GetScore()
		server.CalculateScore(sp.config)
		newScore := server.GetScore()

		if oldScore != newScore {
			log.Printf("Server %s: score changed %.2f -> %.2f", server.ServerURL, oldScore, newScore)
		}
	}
}

func (sp *ServerPool) GetAllServersStatus() []map[string]interface{} {
	sp.mu.RLock()
	defer sp.mu.RUnlock()

	status := make([]map[string]interface{}, 0, len(sp.servers))
	for _, server := range sp.servers {
		status = append(status, server.GetMetricsSummary())
	}
	return status
}

func (sp *ServerPool) GetBestServer() (*ServerMetrics, error) {
	sp.mu.RLock()
	defer sp.mu.RUnlock()

	if len(sp.servers) == 0 {
		return nil, errors.New("no servers available")
	}

	var bestServer *ServerMetrics
	bestScore := -1.0

	for _, server := range sp.servers {
		score := server.GetScore()
		if score > bestScore {
			bestScore = score
			bestServer = server
		}
	}

	if bestServer == nil {
		return nil, errors.New("no healthy servers found")
	}

	return bestServer, nil
}

func (sp *ServerPool) GetServerCount() int {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	return len(sp.servers)
}
