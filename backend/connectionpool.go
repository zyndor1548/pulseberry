package main

import (
	"crypto/tls"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

// ConnectionPoolConfig holds configuration for HTTP connection pooling
type ConnectionPoolConfig struct {
	MaxIdleConns        int           // Maximum number of idle connections across all hosts
	MaxIdleConnsPerHost int           // Maximum idle connections per host
	MaxConnsPerHost     int           // Maximum total connections per host
	IdleConnTimeout     time.Duration // How long idle connections are kept alive
	RequestTimeout      time.Duration // Timeout for individual requests
	TLSHandshakeTimeout time.Duration // Timeout for TLS handshake
	DialTimeout         time.Duration // Timeout for TCP connection establishment
	KeepAlive           time.Duration // TCP keep-alive interval
}

// DefaultPoolConfig returns sensible defaults for connection pooling
func DefaultPoolConfig() ConnectionPoolConfig {
	return ConnectionPoolConfig{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     50,
		IdleConnTimeout:     90 * time.Second,
		RequestTimeout:      10 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		DialTimeout:         5 * time.Second,
		KeepAlive:           30 * time.Second,
	}
}

// ProviderConnectionPool manages HTTP connections for a specific provider
type ProviderConnectionPool struct {
	providerName string
	client       *http.Client
	config       ConnectionPoolConfig
	activeConns  atomic.Int32
	totalReqs    atomic.Int64
	reuseCount   atomic.Int64
}

// NewProviderConnectionPool creates a new connection pool for a provider
func NewProviderConnectionPool(providerName string, config ConnectionPoolConfig) *ProviderConnectionPool {
	// Create custom transport with pooling configuration
	transport := &http.Transport{
		// Connection pooling settings
		MaxIdleConns:        config.MaxIdleConns,
		MaxIdleConnsPerHost: config.MaxIdleConnsPerHost,
		MaxConnsPerHost:     config.MaxConnsPerHost,
		IdleConnTimeout:     config.IdleConnTimeout,

		// Timeouts
		TLSHandshakeTimeout:   config.TLSHandshakeTimeout,
		ResponseHeaderTimeout: config.RequestTimeout,
		ExpectContinueTimeout: 1 * time.Second,

		// Dialer settings
		DialContext: (&net.Dialer{
			Timeout:   config.DialTimeout,
			KeepAlive: config.KeepAlive,
		}).DialContext,

		// TLS configuration
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		},

		// Enable HTTP/2
		ForceAttemptHTTP2: true,

		// Disable compression (we handle this at application level)
		DisableCompression: false,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   config.RequestTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Limit redirects to 3
			if len(via) >= 3 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	return &ProviderConnectionPool{
		providerName: providerName,
		client:       client,
		config:       config,
	}
}

// GetClient returns the HTTP client for this pool
func (pcp *ProviderConnectionPool) GetClient() *http.Client {
	return pcp.client
}

// RecordRequest increments request counters
func (pcp *ProviderConnectionPool) RecordRequest(reuseConn bool) {
	pcp.totalReqs.Add(1)
	if reuseConn {
		pcp.reuseCount.Add(1)
	}
}

// IncrementActiveConns increments the active connection counter
func (pcp *ProviderConnectionPool) IncrementActiveConns() {
	pcp.activeConns.Add(1)
}

// DecrementActiveConns decrements the active connection counter
func (pcp *ProviderConnectionPool) DecrementActiveConns() {
	pcp.activeConns.Add(-1)
}

// GetStats returns connection pool statistics
func (pcp *ProviderConnectionPool) GetStats() ConnectionPoolStats {
	totalReqs := pcp.totalReqs.Load()
	reuseCount := pcp.reuseCount.Load()

	reuseRate := 0.0
	if totalReqs > 0 {
		reuseRate = float64(reuseCount) / float64(totalReqs) * 100
	}

	return ConnectionPoolStats{
		ProviderName:     pcp.providerName,
		ActiveConns:      int(pcp.activeConns.Load()),
		TotalRequests:    totalReqs,
		ConnectionReuses: reuseCount,
		ReuseRate:        reuseRate,
		MaxConnsPerHost:  pcp.config.MaxConnsPerHost,
		IdleTimeout:      pcp.config.IdleConnTimeout,
	}
}

// Close closes the connection pool and all idle connections
func (pcp *ProviderConnectionPool) Close() {
	if transport, ok := pcp.client.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
}

// ConnectionPoolStats holds statistics about connection pool usage
type ConnectionPoolStats struct {
	ProviderName     string        `json:"provider_name"`
	ActiveConns      int           `json:"active_connections"`
	TotalRequests    int64         `json:"total_requests"`
	ConnectionReuses int64         `json:"connection_reuses"`
	ReuseRate        float64       `json:"reuse_rate_percent"`
	MaxConnsPerHost  int           `json:"max_conns_per_host"`
	IdleTimeout      time.Duration `json:"idle_timeout_seconds"`
}

// ConnectionPoolManager manages connection pools for all providers
type ConnectionPoolManager struct {
	pools  map[string]*ProviderConnectionPool
	config ConnectionPoolConfig
}

// NewConnectionPoolManager creates a new connection pool manager
func NewConnectionPoolManager(config ConnectionPoolConfig) *ConnectionPoolManager {
	return &ConnectionPoolManager{
		pools:  make(map[string]*ProviderConnectionPool),
		config: config,
	}
}

// GetOrCreatePool retrieves or creates a connection pool for a provider
func (cpm *ConnectionPoolManager) GetOrCreatePool(providerName string) *ProviderConnectionPool {
	if pool, exists := cpm.pools[providerName]; exists {
		return pool
	}

	pool := NewProviderConnectionPool(providerName, cpm.config)
	cpm.pools[providerName] = pool
	return pool
}

// GetPool retrieves a connection pool by provider name
func (cpm *ConnectionPoolManager) GetPool(providerName string) (*ProviderConnectionPool, bool) {
	pool, exists := cpm.pools[providerName]
	return pool, exists
}

// GetAllStats returns statistics for all connection pools
func (cpm *ConnectionPoolManager) GetAllStats() []ConnectionPoolStats {
	stats := make([]ConnectionPoolStats, 0, len(cpm.pools))
	for _, pool := range cpm.pools {
		stats = append(stats, pool.GetStats())
	}
	return stats
}

// CloseAll closes all connection pools
func (cpm *ConnectionPoolManager) CloseAll() {
	for _, pool := range cpm.pools {
		pool.Close()
	}
}

// Global connection pool manager
var connectionPoolManager *ConnectionPoolManager

// InitConnectionPoolManager initializes the global connection pool manager
func InitConnectionPoolManager(config ConnectionPoolConfig) {
	connectionPoolManager = NewConnectionPoolManager(config)
}

// GetConnectionPoolManager returns the global connection pool manager
func GetConnectionPoolManager() *ConnectionPoolManager {
	if connectionPoolManager == nil {
		connectionPoolManager = NewConnectionPoolManager(DefaultPoolConfig())
	}
	return connectionPoolManager
}
