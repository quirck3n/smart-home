package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/quirck3n/smart-home/gateway_cli/pkg/redis"
	"github.com/quirck3n/smart-home/gateway_cli/pkg/response"
)

// Recovery middleware
func Recovery(redisClient *redis.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					stack := string(debug.Stack())

					// Log panic to console
					fmt.Printf("Panic recovered: %v\n%s\n", err, stack)

					// Log panic to Redis
					redisClient.PublishLog("error", "gateway", fmt.Sprintf("Panic recovered: %v", err), map[string]interface{}{
						"error":      fmt.Sprintf("%v", err),
						"stack":      stack,
						"method":     r.Method,
						"path":       r.URL.Path,
						"request_id": r.Header.Get("X-Request-ID"),
					})

					response.Error(w, http.StatusInternalServerError, "internal server error", nil)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
