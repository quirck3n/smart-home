package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/quirck3n/smart-home/gateway_cli/pkg/redis"
)

// Logger middleware
func Logger(redisClient *redis.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap ResponseWriter to capture status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(wrapped, r)

			duration := time.Since(start)

			// Log to console
			fmt.Printf("[%s] %s %s %d - %v\n",
				start.Format("2006-01-02 15:04:05"),
				r.Method,
				r.URL.Path,
				wrapped.statusCode,
				duration,
			)

			// Log to Redis
			redisClient.PublishLog("info", "gateway", fmt.Sprintf("%s %s", r.Method, r.URL.Path), map[string]interface{}{
				"method":      r.Method,
				"path":        r.URL.Path,
				"status":      wrapped.statusCode,
				"duration_ms": duration.Milliseconds(),
				"remote_addr": getClientIP(r),
				"user_agent":  r.UserAgent(),
				"request_id":  r.Header.Get("X-Request-ID"),
			})
		})
	}
}
