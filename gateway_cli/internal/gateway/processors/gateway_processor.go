package processors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/quirck3n/smart-home/gateway_cli/internal/gateway/config"
	"github.com/quirck3n/smart-home/gateway_cli/internal/gateway/models"
	"github.com/quirck3n/smart-home/gateway_cli/pkg/redis"
)

type GatewayProcessor struct {
	config      *config.Config
	redis       *redis.Client
	services    map[string]*config.ServiceInfo
	healthStats map[string]*models.HealthCheckResult
	metrics     *GatewayMetrics
	mu          sync.RWMutex
	stopChan    chan struct{}
	httpClient  *http.Client
}

type GatewayMetrics struct {
	TotalRequests   int64                                `json:"total_requests"`
	SuccessRequests int64                                `json:"success_requests"`
	ErrorRequests   int64                                `json:"error_requests"`
	AverageLatency  float64                              `json:"average_latency_ms"`
	ServiceMetrics  map[string]*ServiceMetrics           `json:"service_metrics"`
	HealthStats     map[string]*models.HealthCheckResult `json:"health_stats"`
	StartTime       time.Time                            `json:"start_time"`
	mu              sync.RWMutex
}

type ServiceMetrics struct {
	TotalRequests   int64     `json:"total_requests"`
	SuccessRequests int64     `json:"success_requests"`
	ErrorRequests   int64     `json:"error_requests"`
	AverageLatency  float64   `json:"average_latency_ms"`
	LastRequest     time.Time `json:"last_request"`
}

func NewGatewayProcessor(cfg *config.Config, redisClient *redis.Client) *GatewayProcessor {
	return &GatewayProcessor{
		config:      cfg,
		redis:       redisClient,
		services:    make(map[string]*config.ServiceInfo),
		healthStats: make(map[string]*models.HealthCheckResult),
		metrics: &GatewayMetrics{
			ServiceMetrics: make(map[string]*ServiceMetrics),
			HealthStats:    make(map[string]*models.HealthCheckResult),
			StartTime:      time.Now(),
		},
		stopChan: make(chan struct{}),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

func (gp *GatewayProcessor) Start() {
	// Initialize services from config
	for name, serviceInfo := range gp.config.Services.Registry {
		gp.mu.Lock()
		service := serviceInfo // Copy to avoid pointer issues
		gp.services[name] = &service

		// Initialize service metrics
		gp.metrics.ServiceMetrics[name] = &ServiceMetrics{}
		gp.mu.Unlock()
	}

	// Log startup
	gp.redis.PublishLog("info", "gateway", "Gateway processor started", map[string]interface{}{
		"services_count": len(gp.services),
		"start_time":     time.Now().Format(time.RFC3339),
	})
}

func (gp *GatewayProcessor) ProxyRequest(service, path, method string, body io.Reader, headers map[string]string, userID string) (*models.ProxyResponse, error) {
	startTime := time.Now()
	requestID := uuid.New().String()

	// Update metrics
	gp.updateRequestMetrics(service, true)

	// Log request start
	gp.logRequest(models.ProxyRequest{
		Service:   service,
		Method:    method,
		Path:      path,
		UserID:    userID,
		RequestID: requestID,
		Headers:   headers,
		Timestamp: startTime,
	})

	// Get service info
	gp.mu.RLock()
	serviceInfo, exists := gp.services[service]
	gp.mu.RUnlock()

	if !exists {
		gp.updateRequestMetrics(service, false)
		return nil, fmt.Errorf("service %s not found", service)
	}

	// Read body if present
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			gp.updateRequestMetrics(service, false)
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
	}

	// Create HTTP request
	fullURL := serviceInfo.URL + path
	req, err := http.NewRequest(method, fullURL, bytes.NewReader(bodyBytes))
	if err != nil {
		gp.updateRequestMetrics(service, false)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Add tracing headers
	req.Header.Set("X-Request-ID", requestID)
	req.Header.Set("X-User-ID", userID)
	req.Header.Set("X-Gateway-Timestamp", startTime.Format(time.RFC3339))
	req.Header.Set("X-Service-Name", service)

	// Execute request with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(serviceInfo.Timeout)*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := gp.httpClient.Do(req)
	duration := time.Since(startTime)

	if err != nil {
		gp.updateRequestMetrics(service, false)
		gp.updateLatencyMetrics(service, duration)
		gp.logMetrics("request", service, method, path, duration, 0, userID, requestID, map[string]interface{}{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		gp.updateRequestMetrics(service, false)
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Update metrics based on status code
	success := resp.StatusCode >= 200 && resp.StatusCode < 400
	if !success {
		gp.updateRequestMetrics(service, false)
	}
	gp.updateLatencyMetrics(service, duration)

	// Log successful request metrics
	gp.logMetrics("request", service, method, path, duration, resp.StatusCode, userID, requestID, map[string]interface{}{
		"response_size": len(responseBody),
		"success":       success,
	})

	// Parse JSON response if possible
	var bodyInterface interface{}
	if len(responseBody) > 0 {
		if json.Unmarshal(responseBody, &bodyInterface) != nil {
			// If not valid JSON, return as string
			bodyInterface = string(responseBody)
		}
	}

	// Convert response headers
	responseHeaders := make(map[string]string)
	for key, values := range resp.Header {
		if len(values) > 0 {
			responseHeaders[key] = values[0]
		}
	}

	return &models.ProxyResponse{
		StatusCode: resp.StatusCode,
		Body:       bodyInterface,
		Headers:    responseHeaders,
		Duration:   duration,
	}, nil
}

func (gp *GatewayProcessor) CheckServiceHealth(service string) (*models.HealthCheckResult, error) {
	gp.mu.RLock()
	serviceInfo, exists := gp.services[service]
	gp.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("service %s not found", service)
	}

	return gp.performHealthCheck(service, serviceInfo)
}

func (gp *GatewayProcessor) performHealthCheck(service string, serviceInfo *config.ServiceInfo) (*models.HealthCheckResult, error) {
	startTime := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(serviceInfo.Timeout)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", serviceInfo.HealthCheck, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create health check request: %w", err)
	}

	// Add gateway headers
	req.Header.Set("X-Health-Check", "true")
	req.Header.Set("X-Gateway-Service", "gateway")

	resp, err := gp.httpClient.Do(req)
	duration := time.Since(startTime)

	result := &models.HealthCheckResult{
		Service:   service,
		URL:       serviceInfo.HealthCheck,
		Duration:  duration,
		Timestamp: startTime,
	}

	if err != nil {
		result.Status = "unhealthy"
		result.Error = err.Error()
	} else {
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			result.Status = "healthy"
		} else {
			result.Status = "unhealthy"
			result.Error = fmt.Sprintf("status code: %d", resp.StatusCode)
		}
	}

	// Store result
	gp.mu.Lock()
	gp.healthStats[service] = result
	gp.metrics.HealthStats[service] = result
	gp.mu.Unlock()

	// Log health check metrics
	status := 0
	if result.Status == "healthy" {
		status = 1
	}

	gp.logMetrics("health_check", service, "GET", "/health", duration, status, "", "", map[string]interface{}{
		"health_status": result.Status,
		"health_error":  result.Error,
	})

	return result, nil
}

func (gp *GatewayProcessor) GetServicesStatus() map[string]*models.HealthCheckResult {
	gp.mu.RLock()
	defer gp.mu.RUnlock()

	result := make(map[string]*models.HealthCheckResult)
	for service, health := range gp.healthStats {
		// Create copy to avoid race conditions
		healthCopy := *health
		result[service] = &healthCopy
	}

	// Add services without health data
	for service := range gp.services {
		if _, exists := result[service]; !exists {
			result[service] = &models.HealthCheckResult{
				Service:   service,
				Status:    "unknown",
				Timestamp: time.Now(),
			}
		}
	}

	return result
}

func (gp *GatewayProcessor) GetMetrics() *GatewayMetrics {
	gp.metrics.mu.RLock()
	defer gp.metrics.mu.RUnlock()

	// Create a copy of metrics
	result := &GatewayMetrics{
		TotalRequests:   gp.metrics.TotalRequests,
		SuccessRequests: gp.metrics.SuccessRequests,
		ErrorRequests:   gp.metrics.ErrorRequests,
		AverageLatency:  gp.metrics.AverageLatency,
		ServiceMetrics:  make(map[string]*ServiceMetrics),
		HealthStats:     make(map[string]*models.HealthCheckResult),
		StartTime:       gp.metrics.StartTime,
	}

	// Copy service metrics
	for service, metrics := range gp.metrics.ServiceMetrics {
		result.ServiceMetrics[service] = &ServiceMetrics{
			TotalRequests:   metrics.TotalRequests,
			SuccessRequests: metrics.SuccessRequests,
			ErrorRequests:   metrics.ErrorRequests,
			AverageLatency:  metrics.AverageLatency,
			LastRequest:     metrics.LastRequest,
		}
	}

	// Copy health stats
	for service, health := range gp.metrics.HealthStats {
		healthCopy := *health
		result.HealthStats[service] = &healthCopy
	}

	return result
}

func (gp *GatewayProcessor) StartHealthChecker() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Initial health check
	gp.checkAllServices()

	gp.redis.PublishLog("info", "gateway", "Health checker started", map[string]interface{}{
		"interval_seconds": 30,
	})

	for {
		select {
		case <-ticker.C:
			gp.checkAllServices()
		case <-gp.stopChan:
			gp.redis.PublishLog("info", "gateway", "Health checker stopped", nil)
			return
		}
	}
}

func (gp *GatewayProcessor) StartMetricsCollector() {
	ticker := time.NewTicker(60 * time.Second) // Collect metrics every minute
	defer ticker.Stop()

	gp.redis.PublishLog("info", "gateway", "Metrics collector started", map[string]interface{}{
		"interval_seconds": 60,
	})

	for {
		select {
		case <-ticker.C:
			gp.collectAndPublishMetrics()
		case <-gp.stopChan:
			gp.redis.PublishLog("info", "gateway", "Metrics collector stopped", nil)
			return
		}
	}
}

func (gp *GatewayProcessor) Stop() {
	gp.redis.PublishLog("info", "gateway", "Gateway processor stopping", nil)
	close(gp.stopChan)
}

// Private helper methods
func (gp *GatewayProcessor) checkAllServices() {
	var wg sync.WaitGroup

	gp.mu.RLock()
	services := make(map[string]*config.ServiceInfo)
	for k, v := range gp.services {
		services[k] = v
	}
	gp.mu.RUnlock()

	for service, serviceInfo := range services {
		wg.Add(1)
		go func(s string, si *config.ServiceInfo) {
			defer wg.Done()
			gp.performHealthCheck(s, si)
		}(service, serviceInfo)
	}

	wg.Wait()

	// Log health check summary
	healthy := 0
	total := len(services)

	gp.mu.RLock()
	for _, health := range gp.healthStats {
		if health.Status == "healthy" {
			healthy++
		}
	}
	gp.mu.RUnlock()

	gp.redis.PublishLog("info", "gateway", fmt.Sprintf("Health check completed: %d/%d services healthy", healthy, total), map[string]interface{}{
		"healthy_count": healthy,
		"total_count":   total,
	})
}

func (gp *GatewayProcessor) collectAndPublishMetrics() {
	// Get current metrics
	metrics := gp.GetMetrics()

	// Publish to Redis
	gp.redis.PublishMetrics("gateway_summary", "gateway", map[string]interface{}{
		"total_requests":   metrics.TotalRequests,
		"success_requests": metrics.SuccessRequests,
		"error_requests":   metrics.ErrorRequests,
		"average_latency":  metrics.AverageLatency,
		"uptime_seconds":   time.Since(metrics.StartTime).Seconds(),
		"services_count":   len(metrics.ServiceMetrics),
		"healthy_services": gp.countHealthyServices(),
	})

	// Publish per-service metrics
	for service, serviceMetrics := range metrics.ServiceMetrics {
		gp.redis.PublishMetrics("service_summary", service, map[string]interface{}{
			"total_requests":   serviceMetrics.TotalRequests,
			"success_requests": serviceMetrics.SuccessRequests,
			"error_requests":   serviceMetrics.ErrorRequests,
			"average_latency":  serviceMetrics.AverageLatency,
			"last_request":     serviceMetrics.LastRequest.Unix(),
		})
	}
}

func (gp *GatewayProcessor) countHealthyServices() int {
	gp.mu.RLock()
	defer gp.mu.RUnlock()

	count := 0
	for _, health := range gp.healthStats {
		if health.Status == "healthy" {
			count++
		}
	}
	return count
}

func (gp *GatewayProcessor) updateRequestMetrics(service string, success bool) {
	gp.metrics.mu.Lock()
	defer gp.metrics.mu.Unlock()

	// Update global metrics
	gp.metrics.TotalRequests++
	if success {
		gp.metrics.SuccessRequests++
	} else {
		gp.metrics.ErrorRequests++
	}

	// Update service metrics
	if serviceMetrics, exists := gp.metrics.ServiceMetrics[service]; exists {
		serviceMetrics.TotalRequests++
		serviceMetrics.LastRequest = time.Now()
		if success {
			serviceMetrics.SuccessRequests++
		} else {
			serviceMetrics.ErrorRequests++
		}
	}
}

func (gp *GatewayProcessor) updateLatencyMetrics(service string, duration time.Duration) {
	gp.metrics.mu.Lock()
	defer gp.metrics.mu.Unlock()

	latencyMs := float64(duration.Milliseconds())

	// Update global average latency (simple moving average)
	if gp.metrics.TotalRequests == 1 {
		gp.metrics.AverageLatency = latencyMs
	} else {
		gp.metrics.AverageLatency = (gp.metrics.AverageLatency*float64(gp.metrics.TotalRequests-1) + latencyMs) / float64(gp.metrics.TotalRequests)
	}

	// Update service average latency
	if serviceMetrics, exists := gp.metrics.ServiceMetrics[service]; exists {
		if serviceMetrics.TotalRequests == 1 {
			serviceMetrics.AverageLatency = latencyMs
		} else {
			serviceMetrics.AverageLatency = (serviceMetrics.AverageLatency*float64(serviceMetrics.TotalRequests-1) + latencyMs) / float64(serviceMetrics.TotalRequests)
		}
	}
}

func (gp *GatewayProcessor) logRequest(req models.ProxyRequest) {
	gp.redis.PublishLog("info", "gateway", fmt.Sprintf("%s %s proxied to %s", req.Method, req.Path, req.Service), map[string]interface{}{
		"service":    req.Service,
		"method":     req.Method,
		"path":       req.Path,
		"user_id":    req.UserID,
		"request_id": req.RequestID,
	})
}

func (gp *GatewayProcessor) logMetrics(eventType, service, method, path string, duration time.Duration, status int, userID, requestID string, metadata map[string]interface{}) {
	metrics := map[string]interface{}{
		"method":      method,
		"path":        path,
		"duration_ms": duration.Milliseconds(),
		"status":      status,
		"user_id":     userID,
		"request_id":  requestID,
	}

	// Add metadata
	for k, v := range metadata {
		metrics[k] = v
	}

	gp.redis.PublishMetrics(eventType, service, metrics)
}
