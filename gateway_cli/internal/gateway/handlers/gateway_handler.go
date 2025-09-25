package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/quirck3n/smart-home/gateway_cli/internal/gateway/processors"
	"github.com/quirck3n/smart-home/gateway_cli/pkg/response"
)

type GatewayHandler struct {
	processor *processors.GatewayProcessor
}

func NewGatewayHandler(processor *processors.GatewayProcessor) *GatewayHandler {
	return &GatewayHandler{
		processor: processor,
	}
}

func (h *GatewayHandler) Proxy(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	service := vars["service"]

	if service == "" {
		response.Error(w, http.StatusBadRequest, "service not specified", nil)
		return
	}

	// Extract path after /api/proxy/{service}
	path := strings.TrimPrefix(r.URL.Path, "/api/proxy/"+service)
	if path == "" {
		path = "/"
	}

	// Get user context
	userID := getUserID(r)

	// Extract headers
	headers := make(map[string]string)
	for key, values := range r.Header {
		if len(values) > 0 && !isSystemHeader(key) {
			headers[key] = values[0]
		}
	}

	// Proxy the request
	proxyResp, err := h.processor.ProxyRequest(service, path, r.Method, r.Body, headers, userID)
	if err != nil {
		response.Error(w, http.StatusBadGateway, "proxy failed", map[string]interface{}{
			"service": service,
			"error":   err.Error(),
		})
		return
	}

	// Copy response headers
	for key, value := range proxyResp.Headers {
		w.Header().Set(key, value)
	}

	// Set status and write response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(proxyResp.StatusCode)

	if proxyResp.Body != nil {
		json.NewEncoder(w).Encode(proxyResp.Body)
	}
}

func (h *GatewayHandler) ProxyToService(serviceName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get user context
		userID := getUserID(r)

		// Extract headers
		headers := make(map[string]string)
		for key, values := range r.Header {
			if len(values) > 0 && !isSystemHeader(key) {
				headers[key] = values[0]
			}
		}

		// Use original path without /api prefix
		path := strings.TrimPrefix(r.URL.Path, "/api")

		// Proxy the request
		proxyResp, err := h.processor.ProxyRequest(serviceName, path, r.Method, r.Body, headers, userID)
		if err != nil {
			response.Error(w, http.StatusBadGateway, "service unavailable", map[string]interface{}{
				"service": serviceName,
				"error":   err.Error(),
			})
			return
		}

		// Copy response headers
		for key, value := range proxyResp.Headers {
			w.Header().Set(key, value)
		}

		// Set status and write response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(proxyResp.StatusCode)

		if proxyResp.Body != nil {
			json.NewEncoder(w).Encode(proxyResp.Body)
		}
	}
}

func (h *GatewayHandler) ListServices(w http.ResponseWriter, r *http.Request) {
	services := h.processor.GetServicesStatus()
	response.Success(w, "services retrieved", services)
}

func (h *GatewayHandler) CheckServiceHealth(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	service := vars["service"]

	if service == "" {
		response.Error(w, http.StatusBadRequest, "service not specified", nil)
		return
	}

	health, err := h.processor.CheckServiceHealth(service)
	if err != nil {
		response.Error(w, http.StatusNotFound, "service not found", map[string]interface{}{
			"service": service,
			"error":   err.Error(),
		})
		return
	}

	response.Success(w, "health check completed", health)
}

func (h *GatewayHandler) RestartService(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	service := vars["service"]

	// This would integrate with Docker or systemd to restart services
	// For now, just return a placeholder
	response.Success(w, "restart initiated", map[string]interface{}{
		"service": service,
		"status":  "initiated",
	})
}

// Helper functions
func getUserID(r *http.Request) string {
	if userID := r.Header.Get("X-User-ID"); userID != "" {
		return userID
	}

	// Extract from JWT context if available
	if ctx := r.Context(); ctx != nil {
		if userID, ok := ctx.Value("user_id").(string); ok {
			return userID
		}
	}

	return ""
}

func isSystemHeader(header string) bool {
	systemHeaders := []string{
		"Authorization", "Content-Length", "Content-Type", "Host",
		"User-Agent", "Accept-Encoding", "Connection",
	}

	header = strings.ToLower(header)
	for _, sys := range systemHeaders {
		if strings.ToLower(sys) == header {
			return true
		}
	}

	return false
}
