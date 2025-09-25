package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/quirck3n/smart-home/gateway_cli/internal/gateway/config"
	"github.com/quirck3n/smart-home/gateway_cli/pkg/response"
)

type RateLimiter struct {
	clients map[string]*ClientLimiter
	mu      sync.RWMutex
	rpm     int
	burst   int
}

type ClientLimiter struct {
	tokens     int
	lastRefill time.Time
	mu         sync.Mutex
}

func NewRateLimiter(cfg config.RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		clients: make(map[string]*ClientLimiter),
		rpm:     cfg.RequestsPerMinute,
		burst:   cfg.BurstSize,
	}

	// Start cleanup routine
	go rl.cleanup()

	return rl
}

func RateLimit(cfg config.RateLimitConfig) func(http.Handler) http.Handler {
	limiter := NewRateLimiter(cfg)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := getClientIP(r)

			if !limiter.Allow(clientIP) {
				response.Error(w, http.StatusTooManyRequests, "rate limit exceeded", map[string]interface{}{
					"retry_after": "60s",
					"client_ip":   clientIP,
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (rl *RateLimiter) Allow(clientID string) bool {
	rl.mu.RLock()
	client, exists := rl.clients[clientID]
	rl.mu.RUnlock()

	if !exists {
		client = &ClientLimiter{
			tokens:     rl.burst,
			lastRefill: time.Now(),
		}

		rl.mu.Lock()
		rl.clients[clientID] = client
		rl.mu.Unlock()
	}

	return client.allow(rl.rpm, rl.burst)
}

func (cl *ClientLimiter) allow(rpm, burst int) bool {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(cl.lastRefill)

	// Refill tokens based on elapsed time
	tokensToAdd := int(elapsed.Seconds() * float64(rpm) / 60.0)
	cl.tokens += tokensToAdd

	if cl.tokens > burst {
		cl.tokens = burst
	}

	cl.lastRefill = now

	// Check if request is allowed
	if cl.tokens > 0 {
		cl.tokens--
		return true
	}

	return false
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()

		for clientID, client := range rl.clients {
			client.mu.Lock()
			if now.Sub(client.lastRefill) > 10*time.Minute {
				delete(rl.clients, clientID)
			}
			client.mu.Unlock()
		}

		rl.mu.Unlock()
	}
}
