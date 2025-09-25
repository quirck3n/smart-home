package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/quirck3n/smart-home/gateway_cli/pkg/models"
)

type Config struct {
	Server    ServerConfig
	Redis     models.RedisConfig
	Services  ServicesConfig
	RateLimit RateLimitConfig
}

type ServerConfig struct {
	Port         string
	ReadTimeout  int
	WriteTimeout int
}

type RedisConfig struct {
	URL      string
	Password string
	DB       int
}

type ServicesConfig struct {
	Registry map[string]ServiceInfo
}

type ServiceInfo struct {
	URL         string
	HealthCheck string
	Timeout     int
}

type RateLimitConfig struct {
	RequestsPerMinute int
	BurstSize         int
}

func Load() (*Config, error) {
	// Load .env file if exists
	godotenv.Load()

	return &Config{
		Server: ServerConfig{
			Port:         getEnv("GATEWAY_PORT", "8080"),
			ReadTimeout:  getEnvInt("SERVER_READ_TIMEOUT", 10),
			WriteTimeout: getEnvInt("SERVER_WRITE_TIMEOUT", 10),
		},
		Redis: models.RedisConfig{
			URL:      getEnv("REDIS_URL", "redis://localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		Services: ServicesConfig{
			Registry: parseServices(),
		},
		RateLimit: RateLimitConfig{
			RequestsPerMinute: getEnvInt("RATE_LIMIT_RPM", 100),
			BurstSize:         getEnvInt("RATE_LIMIT_BURST", 20),
		},
	}, nil
}

func parseServices() map[string]ServiceInfo {
	services := make(map[string]ServiceInfo)

	// Parse services from env: SERVICES=auth:http://localhost:8081,device-registry:http://localhost:8082
	servicesEnv := getEnv("SERVICES", "")
	if servicesEnv == "" {
		// Default services for development
		services["auth"] = ServiceInfo{
			URL:         "http://localhost:8081",
			HealthCheck: "http://localhost:8081/health",
			Timeout:     5,
		}
		services["device-registry"] = ServiceInfo{
			URL:         "http://localhost:8082",
			HealthCheck: "http://localhost:8082/health",
			Timeout:     5,
		}
		services["analytics"] = ServiceInfo{
			URL:         "http://localhost:8083",
			HealthCheck: "http://localhost:8083/health",
			Timeout:     5,
		}
		return services
	}

	for _, serviceStr := range strings.Split(servicesEnv, ",") {
		parts := strings.Split(serviceStr, ":")
		if len(parts) >= 3 {
			name := parts[0]
			url := strings.Join(parts[1:], ":")
			services[name] = ServiceInfo{
				URL:         url,
				HealthCheck: url + "/health",
				Timeout:     5,
			}
		}
	}

	return services
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
