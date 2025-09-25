package models

import (
	"time"
)

type ProxyRequest struct {
	Service   string            `json:"service"`
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Body      interface{}       `json:"body,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	UserID    string            `json:"user_id,omitempty"`
	RequestID string            `json:"request_id"`
	Timestamp time.Time         `json:"timestamp"`
}

type ProxyResponse struct {
	StatusCode int               `json:"status_code"`
	Body       interface{}       `json:"body,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Duration   time.Duration     `json:"duration"`
	Error      string            `json:"error,omitempty"`
}

type HealthCheckResult struct {
	Service   string        `json:"service"`
	Status    string        `json:"status"` // "healthy", "unhealthy"
	URL       string        `json:"url"`
	Duration  time.Duration `json:"duration"`
	Error     string        `json:"error,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
}

type MetricsEvent struct {
	Type      string                 `json:"type"` // "request", "health_check", "error"
	Service   string                 `json:"service"`
	Method    string                 `json:"method,omitempty"`
	Path      string                 `json:"path,omitempty"`
	Duration  time.Duration          `json:"duration"`
	Status    int                    `json:"status,omitempty"`
	UserID    string                 `json:"user_id,omitempty"`
	RequestID string                 `json:"request_id,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type AuthValidationRequest struct {
	RequestID string `json:"request_id"`
	Token     string `json:"token"`
	Timestamp int64  `json:"timestamp"`
}

type AuthValidationResponse struct {
	RequestID string `json:"request_id"`
	Valid     bool   `json:"valid"`
	User      *User  `json:"user,omitempty"`
	Error     string `json:"error,omitempty"`
}
