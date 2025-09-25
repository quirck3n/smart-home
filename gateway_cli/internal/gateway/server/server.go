package server

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/quirck3n/smart-home/gateway_cli/pkg/redis"

	"github.com/quirck3n/smart-home/gateway_cli/internal/gateway/config"
	"github.com/quirck3n/smart-home/gateway_cli/internal/gateway/handlers"
	"github.com/quirck3n/smart-home/gateway_cli/internal/gateway/middleware"
	"github.com/quirck3n/smart-home/gateway_cli/internal/gateway/processors"
)

type Server struct {
	config     *config.Config
	router     *mux.Router
	httpServer *http.Server
	processor  *processors.GatewayProcessor
}

func New(cfg *config.Config, redisClient *redis.Client) *Server {
	// Initialize processor with dependencies
	processor := processors.NewGatewayProcessor(cfg, redisClient)

	// Setup router
	router := setupRouter(cfg, processor, redisClient)

	return &Server{
		config:    cfg,
		router:    router,
		processor: processor,
		httpServer: &http.Server{
			Addr:         ":" + cfg.Server.Port,
			Handler:      router,
			ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
			WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
			IdleTimeout:  120 * time.Second,
		},
	}
}

func (s *Server) Start() error {
	// Start background services
	go s.processor.StartHealthChecker()
	go s.processor.StartMetricsCollector()

	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.processor.Stop()
	return s.httpServer.Shutdown(ctx)
}

func setupRouter(cfg *config.Config, processor *processors.GatewayProcessor, redisClient *redis.Client) *mux.Router {
	r := mux.NewRouter()

	// Global middleware chain
	r.Use(middleware.Logger(redisClient))
	r.Use(middleware.Recovery())
	r.Use(middleware.CORS())
	r.Use(middleware.RequestID())
	r.Use(middleware.RateLimit(cfg.RateLimit))

	// Initialize handlers
	gatewayHandler := handlers.NewGatewayHandler(processor)
	healthHandler := handlers.NewHealthHandler(processor)
	metricsHandler := handlers.NewMetricsHandler(processor)

	// API routes
	api := r.PathPrefix("/api").Subrouter()

	// Public endpoints
	api.HandleFunc("/health", healthHandler.Health).Methods("GET")
	api.HandleFunc("/health/{service}", healthHandler.ServiceHealth).Methods("GET")
	api.HandleFunc("/services", gatewayHandler.ListServices).Methods("GET")

	// Protected endpoints
	protected := api.PathPrefix("").Subrouter()
	protected.Use(middleware.Auth(cfg.Auth))

	// Proxy routes - catch all for service forwarding
	protected.PathPrefix("/proxy/{service}").HandlerFunc(gatewayHandler.Proxy)

	// Direct service routes (more RESTful)
	protected.HandleFunc("/devices", gatewayHandler.ProxyToService("device-registry")).Methods("GET", "POST")
	protected.HandleFunc("/devices/{id}", gatewayHandler.ProxyToService("device-registry")).Methods("GET", "PUT", "DELETE")
	protected.HandleFunc("/auth/login", gatewayHandler.ProxyToService("auth")).Methods("POST")
	protected.HandleFunc("/auth/refresh", gatewayHandler.ProxyToService("auth")).Methods("POST")

	// Admin endpoints
	admin := protected.PathPrefix("/admin").Subrouter()
	admin.Use(middleware.RequireRole("admin"))
	admin.HandleFunc("/metrics", metricsHandler.GetMetrics).Methods("GET")
	admin.HandleFunc("/services/{service}/health", gatewayHandler.CheckServiceHealth).Methods("POST")
	admin.HandleFunc("/services/{service}/restart", gatewayHandler.RestartService).Methods("POST")

	return r
}
