package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateQuota defines rate limit configuration
type RateQuota struct {
	RequestsPerMinute int
	BurstSize         int
}

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	redis  *redis.Client
	quotas map[string]RateQuota
	mu     sync.RWMutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(redisClient *redis.Client) *RateLimiter {
	return &RateLimiter{
		redis:  redisClient,
		quotas: make(map[string]RateQuota),
	}
}

// SetQuota sets rate limit quota for an API key
func (rl *RateLimiter) SetQuota(apiKey string, quota RateQuota) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.quotas[apiKey] = quota
}

// GetQuota retrieves rate limit quota for an API key
func (rl *RateLimiter) GetQuota(apiKey string) RateQuota {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	if quota, exists := rl.quotas[apiKey]; exists {
		return quota
	}

	// Default quota
	return RateQuota{
		RequestsPerMinute: 100,
		BurstSize:         10,
	}
}

// Allow checks if a request should be allowed (token bucket algorithm)
func (rl *RateLimiter) Allow(ctx context.Context, apiKey string) (bool, time.Duration, error) {
	quota := rl.GetQuota(apiKey)

	// Redis key for this API key's token bucket
	key := fmt.Sprintf("ratelimit:%s", apiKey)

	// Use Redis INCR with expiry for simple rate limiting
	// More sophisticated: use sorted sets or Lua scripts for true token bucket

	count, err := rl.redis.Get(ctx, key).Int()
	if err == redis.Nil {
		// First request in this window
		err = rl.redis.Set(ctx, key, 1, 60*time.Second).Err()
		if err != nil {
			return false, 0, err
		}
		return true, 0, nil
	} else if err != nil {
		// Redis error - fail open (allow request)
		return true, 0, err
	}

	// Check if we've exceeded the quota
	if count >= quota.RequestsPerMinute {
		// Get TTL to inform client when they can retry
		ttl, err := rl.redis.TTL(ctx, key).Result()
		if err != nil {
			ttl = 60 * time.Second
		}
		return false, ttl, nil
	}

	// Increment the counter
	err = rl.redis.Incr(ctx, key).Err()
	if err != nil {
		// Fail open
		return true, 0, err
	}

	return true, 0, nil
}

// AllowIP checks rate limit by IP address
func (rl *RateLimiter) AllowIP(ctx context.Context, ip string) (bool, time.Duration, error) {
	// IP-based rate limiting (stricter)
	key := fmt.Sprintf("ratelimit:ip:%s", ip)

	ipQuota := RateQuota{
		RequestsPerMinute: 200, // Higher limit for IP
		BurstSize:         20,
	}

	count, err := rl.redis.Get(ctx, key).Int()
	if err == redis.Nil {
		err = rl.redis.Set(ctx, key, 1, 60*time.Second).Err()
		if err != nil {
			return false, 0, err
		}
		return true, 0, nil
	} else if err != nil {
		return true, 0, err
	}

	if count >= ipQuota.RequestsPerMinute {
		ttl, err := rl.redis.TTL(ctx, key).Result()
		if err != nil {
			ttl = 60 * time.Second
		}
		return false, ttl, nil
	}

	err = rl.redis.Incr(ctx, key).Err()
	if err != nil {
		return true, 0, err
	}

	return true, 0, nil
}

// RateLimitMiddleware enforces rate limiting
func RateLimitMiddleware(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Get API key from context (set by AuthMiddleware)
			apiKey, ok := ctx.Value("api_key").(string)
			if !ok || apiKey == "" {
				// If no API key, use IP-based rate limiting
				ip := getClientIP(r)
				allowed, retryAfter, err := limiter.AllowIP(ctx, ip)

				if err != nil {
					// Log error but allow request (fail open)
					log.Printf("[RateLimit] Error checking IP rate limit: %v", err)
				}

				if !allowed {
					w.Header().Set("X-RateLimit-Limit", "200")
					w.Header().Set("X-RateLimit-Remaining", "0")
					w.Header().Set("Retry-After", fmt.Sprintf("%d", int(retryAfter.Seconds())))
					http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
					return
				}

				next.ServeHTTP(w, r)
				return
			}

			// API key rate limiting
			allowed, retryAfter, err := limiter.Allow(ctx, apiKey)

			if err != nil {
				log.Printf("[RateLimit] Error checking API key rate limit: %v", err)
			}

			quota := limiter.GetQuota(apiKey)

			if !allowed {
				w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", quota.RequestsPerMinute))
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("Retry-After", fmt.Sprintf("%d", int(retryAfter.Seconds())))
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			// Add rate limit headers to successful responses
			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", quota.RequestsPerMinute))

			next.ServeHTTP(w, r)
		})
	}
}

// getClientIP extracts client IP from request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// Take the first IP in the chain
		ips := strings.Split(xff, ",")
		return strings.TrimSpace(ips[0])
	}

	// Check X-Real-IP header
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	// Remove port if present
	if colonIdx := strings.LastIndex(ip, ":"); colonIdx != -1 {
		ip = ip[:colonIdx]
	}

	return ip
}
