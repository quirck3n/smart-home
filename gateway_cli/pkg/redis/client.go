package redis

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/quirck3n/smart-home/gateway_cli/pkg/models"
	"github.com/redis/go-redis/v9"
)

type Client struct {
	*redis.Client
}

func NewClient(cfg models.RedisConfig) (*Client, error) {
	// Parse Redis URL
	options, err := parseRedisURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	// Override with config values
	if cfg.Password != "" {
		options.Password = cfg.Password
	}
	if cfg.DB != 0 {
		options.DB = cfg.DB
	}

	client := redis.NewClient(options)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &Client{Client: client}, nil
}

func (c *Client) PublishEvent(stream string, data map[string]interface{}) error {
	ctx := context.Background()

	_, err := c.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: data,
	}).Result()

	return err
}

func (c *Client) PublishLog(level, service, message string, extra map[string]interface{}) error {
	logData := map[string]interface{}{
		"level":     level,
		"service":   service,
		"message":   message,
		"timestamp": time.Now().Unix(),
	}

	// Add extra fields
	for k, v := range extra {
		logData[k] = v
	}

	return c.PublishEvent("logs-stream", logData)
}

func (c *Client) PublishMetrics(eventType, service string, metrics map[string]interface{}) error {
	metricsData := map[string]interface{}{
		"type":      eventType,
		"service":   service,
		"timestamp": time.Now().Unix(),
	}

	// Add metrics fields
	for k, v := range metrics {
		metricsData[k] = v
	}

	return c.PublishEvent("metrics-stream", metricsData)
}

func parseRedisURL(redisURL string) (*redis.Options, error) {
	u, err := url.Parse(redisURL)
	if err != nil {
		return nil, err
	}

	options := &redis.Options{
		Addr: u.Host,
	}

	// Extract password
	if u.User != nil {
		if password, ok := u.User.Password(); ok {
			options.Password = password
		}
	}

	// Extract database number
	if len(u.Path) > 1 {
		db, err := strconv.Atoi(u.Path[1:])
		if err != nil {
			return nil, fmt.Errorf("invalid database number: %s", u.Path[1:])
		}
		options.DB = db
	}

	// Default timeouts
	options.DialTimeout = 5 * time.Second
	options.ReadTimeout = 3 * time.Second
	options.WriteTimeout = 3 * time.Second

	return options, nil
}
