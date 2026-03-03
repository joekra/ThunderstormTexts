package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client wraps the redis.Client
type Client struct {
	client *redis.Client
}

// NewClient creates a new Redis client
func NewClient(host string, port int, db int) *Client {
	rdb := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%d", host, port),
		DB:   db,
	})

	return &Client{
		client: rdb,
	}
}

// Publish publishes a message to a channel
func (c *Client) Publish(ctx context.Context, channel string, message any) error {
	return c.client.Publish(ctx, channel, message).Err()
}

// Subscribe subscribes to a channel
func (c *Client) Subscribe(ctx context.Context, channel string) *redis.PubSub {
	return c.client.Subscribe(ctx, channel)
}

// Get retrieves the value of a key
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	return c.client.Get(ctx, key).Result()
}

// Set sets the value of a key with an expiration
func (c *Client) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	return c.client.Set(ctx, key, value, expiration).Err()
}

// Ping checks connectivity to Redis
func (c *Client) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// DefaultWaitReadyMaxWait is the default maximum time to wait for Redis to become ready
const DefaultWaitReadyMaxWait = 2 * time.Minute

// DefaultWaitReadyBackoff is the default delay between readiness checks
const DefaultWaitReadyBackoff = 2 * time.Second

// WaitReady blocks until Redis responds to Ping or the context deadline / maxWait is exceeded.
// It retries every backoff. If logFn is non-nil, it is called with a message on each retry (e.g. log.Printf).
// Returns nil when Redis is ready, or the last Ping error after giving up.
func (c *Client) WaitReady(ctx context.Context, maxWait, backoff time.Duration, logFn func(string, ...any)) error {
	deadline := time.Now().Add(maxWait)
	var lastErr error
	for time.Now().Before(deadline) {
		err := c.client.Ping(ctx).Err()
		if err == nil {
			return nil
		}
		lastErr = err
		if logFn != nil {
			logFn("Redis not ready, retrying in %v...", backoff)
		}
		time.Sleep(backoff)
	}
	return lastErr
}

// Close closes the Redis client connection
func (c *Client) Close() error {
	return c.client.Close()
}
