package handlers

import (
	"net/http"

	"github.com/quirck3n/smart-home/gateway_cli/internal/gateway/processors"
	"github.com/quirck3n/smart-home/gateway_cli/pkg/response"
)

type MetricshHandler struct {
	// fields
}

func NewMetricsHandler(processor *processors.GatewayProcessor) *MetricshHandler {
	return &MetricshHandler{}
}

func (h *MetricshHandler) Metric(w http.ResponseWriter, r *http.Request) {
	response.Success(w, "gateway healthy", map[string]interface{}{
		"status": "healthy",
	})
}

func (h *HealthHandler) ServiceMetric(w http.ResponseWriter, r *http.Request) {
	// implementation
}
