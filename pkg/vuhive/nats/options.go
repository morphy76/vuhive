package nats

import (
	"crypto/tls"
	"time"
)

// Option configures a NATS Publisher, Subscriber, or Client.
type Option func(*clientConfig)

// clientConfig holds the resolved configuration for a NATS client handle.
type clientConfig struct {
	servers               []string
	name                  string
	token                 string
	user                  string
	password              string
	userCredentials       string
	nkey                  string
	nkeySeed              string
	tlsConfig             *tls.Config
	tlsInsecureSkipVerify bool
	timeout               time.Duration
	metricPrefix          string
	jetStreamEnabled      bool
	maxReconnects         int
	reconnectWait         time.Duration
}

func defaultConfig() clientConfig {
	return clientConfig{
		servers:       []string{"nats://127.0.0.1:4222"},
		metricPrefix:  defaultMetricPrefix,
		timeout:       5 * time.Second,
		maxReconnects: 10,
		reconnectWait: 2 * time.Second,
	}
}

// WithURL sets the primary NATS server URL (e.g. "nats://localhost:4222").
func WithURL(url string) Option {
	return func(c *clientConfig) {
		if url != "" {
			c.servers = []string{url}
		}
	}
}

// WithServers sets the list of NATS cluster server seed addresses.
func WithServers(servers ...string) Option {
	return func(c *clientConfig) {
		if len(servers) > 0 {
			c.servers = servers
		}
	}
}

// WithName sets the client connection name reported to NATS server monitoring.
func WithName(name string) Option {
	return func(c *clientConfig) {
		c.name = name
	}
}

// WithToken configures token-based authentication.
func WithToken(token string) Option {
	return func(c *clientConfig) {
		c.token = token
	}
}

// WithUserPassword configures username and password authentication.
func WithUserPassword(user, password string) Option {
	return func(c *clientConfig) {
		c.user = user
		c.password = password
	}
}

// WithUserCredentials configures user credentials file authentication (NATS 2.0+ JWT/User credentials).
func WithUserCredentials(credsFile string) Option {
	return func(c *clientConfig) {
		c.userCredentials = credsFile
	}
}

// WithNKey configures NKey public key and private seed authentication.
func WithNKey(pubKey, seed string) Option {
	return func(c *clientConfig) {
		c.nkey = pubKey
		c.nkeySeed = seed
	}
}

// WithTLS sets custom TLS configuration for encrypted server transport.
func WithTLS(cfg *tls.Config) Option {
	return func(c *clientConfig) {
		c.tlsConfig = cfg
	}
}

// WithTLSInsecureSkipVerify enables TLS transport while skipping server certificate verification.
func WithTLSInsecureSkipVerify() Option {
	return func(c *clientConfig) {
		c.tlsInsecureSkipVerify = true
	}
}

// WithTimeout sets the network I/O and request-reply timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(c *clientConfig) {
		c.timeout = timeout
	}
}

// WithCustomMetricPrefix overrides the default metric name prefix ("vuhive.nats.").
func WithCustomMetricPrefix(prefix string) Option {
	return func(c *clientConfig) {
		c.metricPrefix = prefix
	}
}

// WithJetStream enables or disables JetStream support.
func WithJetStream(enabled bool) Option {
	return func(c *clientConfig) {
		c.jetStreamEnabled = enabled
	}
}

// WithMaxReconnects sets the maximum number of reconnection attempts before failing.
func WithMaxReconnects(max int) Option {
	return func(c *clientConfig) {
		c.maxReconnects = max
	}
}

// WithReconnectWait sets the wait duration between reconnection attempts.
func WithReconnectWait(d time.Duration) Option {
	return func(c *clientConfig) {
		c.reconnectWait = d
	}
}
