package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/quirck3n/smart-home/gateway_cli/internal/gateway/models"
	redisClient "github.com/quirck3n/smart-home/gateway_cli/pkg/redis"
	"github.com/quirck3n/smart-home/gateway_cli/pkg/response"
)

// Auth middleware - validates token via Redis Streams
func Auth(redisClient *redisClient.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				response.Error(w, http.StatusUnauthorized, "authorization header required", nil)
				return
			}

			// Extract token from "Bearer <token>"
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				response.Error(w, http.StatusUnauthorized, "invalid authorization format", nil)
				return
			}

			token := parts[1]

			// Validate token via Redis Streams
			user, err := validateTokenViaRedis(redisClient, token)
			if err != nil {
				response.Error(w, http.StatusUnauthorized, "invalid token", map[string]interface{}{
					"error": err.Error(),
				})
				return
			}

			// Add user context
			ctx := context.WithValue(r.Context(), "user_id", user.ID)
			ctx = context.WithValue(ctx, "role", user.Role)
			ctx = context.WithValue(ctx, "email", user.Email)
			r = r.WithContext(ctx)

			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole middleware
func RequireRole(requiredRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userRole, ok := r.Context().Value("role").(string)
			if !ok || userRole != requiredRole {
				response.Error(w, http.StatusForbidden, "insufficient permissions", map[string]interface{}{
					"required_role": requiredRole,
					"user_role":     userRole,
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// validateTokenViaRedis sends token validation request via Redis Streams
func validateTokenViaRedis(redisClient *redisClient.Client, token string) (*models.User, error) {
	ctx := context.Background()
	requestID := uuid.New().String()

	// Send validation request to auth-requests stream
	request := models.AuthValidationRequest{
		RequestID: requestID,
		Token:     token,
		Timestamp: time.Now().Unix(),
	}

	requestData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send to auth-requests stream
	_, err = redisClient.XAdd(ctx, &redis.XAddArgs{
		Stream: "auth-requests",
		Values: map[string]interface{}{
			"data": string(requestData),
		},
	}).Result()

	if err != nil {
		return nil, fmt.Errorf("failed to send auth request: %w", err)
	}

	// Listen for response on auth-responses stream with specific consumer group
	timeout := 5 * time.Second
	consumerGroup := "gateway-auth"
	consumerName := "gateway-" + requestID[:8]

	// Create consumer group if it doesn't exist
	redisClient.XGroupCreateMkStream(ctx, "auth-responses", consumerGroup, "0")

	// Read response
	streams, err := redisClient.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    consumerGroup,
		Consumer: consumerName,
		Streams:  []string{"auth-responses", ">"},
		Count:    1,
		Block:    timeout,
	}).Result()

	if err != nil {
		// Check if it's a timeout or actual error
		if err == redis.Nil {
			return nil, fmt.Errorf("timeout waiting for auth response")
		}
		return nil, fmt.Errorf("failed to read auth response: %w", err)
	}

	// Parse response messages
	for _, stream := range streams {
		for _, message := range stream.Messages {
			data, ok := message.Values["data"].(string)
			if !ok {
				continue
			}

			var response models.AuthValidationResponse
			if err := json.Unmarshal([]byte(data), &response); err != nil {
				continue
			}

			// Check if this is our response
			if response.RequestID == requestID {
				// Acknowledge the message
				redisClient.XAck(ctx, "auth-responses", consumerGroup, message.ID)

				if !response.Valid {
					return nil, fmt.Errorf("invalid token: %s", response.Error)
				}
				return response.User, nil
			}
		}
	}

	return nil, fmt.Errorf("no response received for token validation")
}
