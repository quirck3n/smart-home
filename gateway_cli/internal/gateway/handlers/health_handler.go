package handlers

import (
	"net/http"

	"github.com/quirck3n/smart-home/gateway_cli/internal/gateway/processors"
	"github.com/quirck3n/smart-home/gateway_cli/pkg/response"
)

type HealthHandler struct {
	// fields
}

func NewHealthHandler(processor *processors.GatewayProcessor) *HealthHandler {
	return &HealthHandler{}
}

func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	response.Success(w, "gateway healthy", map[string]interface{}{
		"status": "healthy",
	})
}

func (h *HealthHandler) ServiceHealth(w http.ResponseWriter, r *http.Request) {
	// implementation
}
