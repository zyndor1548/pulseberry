package main

import (
	"errors"
	"log"
	"math/rand"
	"sync"
	"time"
)

// ServerPool manages multiple payment gateway servers with health scoring
type ServerPool struct {
	servers      map[string]*ServerMetrics
	config       *ScoringConfig
	mu           sync.RWMutex
	updateTicker *time.Ticker
	stopChan     chan bool
	isRunning    bool
}

// NewServerPool creates a new server pool with the given configuration
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

// AddServer adds a new server to the pool
func (sp *ServerPool) AddServer(serverURL string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	if _, exists := sp.servers[serverURL]; !exists {
		sp.servers[serverURL] = NewServerMetrics(serverURL)
		log.Printf("Added server to pool: %s (initial score: %.2f)", serverURL, 100.0)
	}
}

// RemoveServer removes a server from the pool
func (sp *ServerPool) RemoveServer(serverURL string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	if _, exists := sp.servers[serverURL]; exists {
		delete(sp.servers, serverURL)
		log.Printf("Removed server from pool: %s", serverURL)
	}
}

// GetServer returns the metrics for a specific server
func (sp *ServerPool) GetServer(serverURL string) (*ServerMetrics, error) {
	sp.mu.RLock()
	defer sp.mu.RUnlock()

	server, exists := sp.servers[serverURL]
	if !exists {
		return nil, errors.New("server not found")
	}
	return server, nil
}

// SelectServer selects a server using weighted random selection based on scores
func (sp *ServerPool) SelectServer() (*ServerMetrics, error) {
	sp.mu.RLock()
	defer sp.mu.RUnlock()

	if len(sp.servers) == 0 {
		return nil, errors.New("no servers available")
	}

	// Calculate total score
	totalScore := 0.0
	serverList := make([]*ServerMetrics, 0, len(sp.servers))

	for _, server := range sp.servers {
		score := server.GetScore()
		// Only consider servers with score > 0
		if score > 0 {
			totalScore += score
			serverList = append(serverList, server)
		}
	}

	// If all servers have score 0, fall back to random selection
	if totalScore == 0 || len(serverList) == 0 {
		log.Println("Warning: All servers have score 0, using fallback selection")
		// Get any server
		for _, server := range sp.servers {
			return server, nil
		}
		return nil, errors.New("no healthy servers available")
	}

	// Weighted random selection
	randomValue := rand.Float64() * totalScore
	currentSum := 0.0

	for _, server := range serverList {
		currentSum += server.GetScore()
		if currentSum >= randomValue {
			return server, nil
		}
	}

	// Fallback (should not reach here)
	return serverList[0], nil
}

// RecordRequestResult records the result of a request to a specific server
func (sp *ServerPool) RecordRequestResult(serverURL string, latency time.Duration, success bool, errorType *ErrorType, errorMsg string) {
	server, err := sp.GetServer(serverURL)
	if err != nil {
		log.Printf("Error recording request result: %v", err)
		return
	}

	server.RecordRequest(latency, success)

	if !success && errorType != nil {
		server.RecordError(*errorType, errorMsg)
	}
}

// StartPeriodicScoreUpdate starts the periodic score recalculation
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
		log.Printf("Started periodic score updates (interval: %v)", sp.config.ScoreUpdatePeriod)
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

// StopPeriodicScoreUpdate stops the periodic score recalculation
func (sp *ServerPool) StopPeriodicScoreUpdate() {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	if sp.isRunning {
		sp.stopChan <- true
		sp.isRunning = false
	}
}

// updateAllScores recalculates scores for all servers
func (sp *ServerPool) updateAllScores() {
	sp.mu.RLock()
	defer sp.mu.RUnlock()

	log.Println("=== Updating server scores ===")
	for _, server := range sp.servers {
		oldScore := server.GetScore()
		server.CalculateScore(sp.config)
		newScore := server.GetScore()

		if oldScore != newScore {
			log.Printf("Server %s: score changed %.2f -> %.2f", server.ServerURL, oldScore, newScore)
		}
	}
}

// GetAllServersStatus returns status of all servers
func (sp *ServerPool) GetAllServersStatus() []map[string]interface{} {
	sp.mu.RLock()
	defer sp.mu.RUnlock()

	status := make([]map[string]interface{}, 0, len(sp.servers))
	for _, server := range sp.servers {
		status = append(status, server.GetMetricsSummary())
	}
	return status
}

// GetBestServer returns the server with the highest score
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

// GetServerCount returns the number of servers in the pool
func (sp *ServerPool) GetServerCount() int {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	return len(sp.servers)
}
