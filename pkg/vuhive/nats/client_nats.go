//go:build nats

package nats

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/morphy76/vuhive/pkg/vuhive"
)

type natsPublisher struct {
	conn    *nats.Conn
	cfg     clientConfig
	metrics vuhive.MetricsCollector
}

func (p *natsPublisher) Publish(ctx context.Context, subject string, data []byte) error {
	if subject == "" {
		return ErrSubjectRequired
	}

	start := time.Now()
	err := p.conn.Publish(subject, data)
	duration := time.Since(start)

	recordPubMetrics(p.metrics, p.cfg.metricPrefix, subject, duration, len(data), err)

	if err != nil {
		return fmt.Errorf("vuhive/nats: publish failed: %w", err)
	}
	return nil
}

func (p *natsPublisher) PublishMsg(ctx context.Context, msg *Message) error {
	if msg == nil {
		return ErrNilMessage
	}
	if msg.Subject == "" {
		return ErrSubjectRequired
	}

	nMsg := &nats.Msg{
		Subject: msg.Subject,
		Data:    msg.Data,
		Reply:   msg.Reply,
	}
	if len(msg.Header) > 0 {
		nMsg.Header = make(nats.Header, len(msg.Header))
		for k, v := range msg.Header {
			nMsg.Header[k] = v
		}
	}

	start := time.Now()
	err := p.conn.PublishMsg(nMsg)
	duration := time.Since(start)

	recordPubMetrics(p.metrics, p.cfg.metricPrefix, msg.Subject, duration, len(msg.Data), err)

	if err != nil {
		return fmt.Errorf("vuhive/nats: publish message failed: %w", err)
	}
	return nil
}

func (p *natsPublisher) Request(ctx context.Context, subject string, data []byte, timeout time.Duration) (*Message, error) {
	if subject == "" {
		return nil, ErrSubjectRequired
	}
	if timeout <= 0 {
		timeout = p.cfg.timeout
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	res, err := p.conn.RequestWithContext(reqCtx, subject, data)
	duration := time.Since(start)

	if err != nil {
		recordReqMetrics(p.metrics, p.cfg.metricPrefix, subject, duration, 0, err)
		if errors.Is(err, nats.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrTimeout
		}
		return nil, fmt.Errorf("vuhive/nats: request failed: %w", err)
	}

	recordReqMetrics(p.metrics, p.cfg.metricPrefix, subject, duration, len(res.Data), nil)

	var headers map[string][]string
	if len(res.Header) > 0 {
		headers = make(map[string][]string, len(res.Header))
		for k, v := range res.Header {
			headers[k] = v
		}
	}

	return &Message{
		Subject: res.Subject,
		Data:    res.Data,
		Header:  headers,
		Reply:   res.Reply,
	}, nil
}

func (p *natsPublisher) Close() error {
	p.conn.Close()
	return nil
}

type natsSubscription struct {
	sub          *nats.Subscription
	metrics      vuhive.MetricsCollector
	metricPrefix string
}

func (s *natsSubscription) NextMsg(ctx context.Context, timeout time.Duration) (*Message, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	msgCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	res, err := s.sub.NextMsgWithContext(msgCtx)
	if err != nil {
		recordSubMetrics(s.metrics, s.metricPrefix, s.sub.Subject, 0, err)
		if errors.Is(err, nats.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrTimeout
		}
		return nil, fmt.Errorf("vuhive/nats: next msg failed: %w", err)
	}

	recordSubMetrics(s.metrics, s.metricPrefix, res.Subject, len(res.Data), nil)

	var headers map[string][]string
	if len(res.Header) > 0 {
		headers = make(map[string][]string, len(res.Header))
		for k, v := range res.Header {
			headers[k] = v
		}
	}

	return &Message{
		Subject: res.Subject,
		Data:    res.Data,
		Header:  headers,
		Reply:   res.Reply,
	}, nil
}

func (s *natsSubscription) Unsubscribe() error {
	return s.sub.Unsubscribe()
}

type natsSubscriber struct {
	conn    *nats.Conn
	cfg     clientConfig
	metrics vuhive.MetricsCollector
}

func (s *natsSubscriber) Subscribe(ctx context.Context, subject string) (Subscription, error) {
	if subject == "" {
		return nil, ErrSubjectRequired
	}

	sub, err := s.conn.SubscribeSync(subject)
	if err != nil {
		return nil, fmt.Errorf("vuhive/nats: subscribe failed: %w", err)
	}

	return &natsSubscription{
		sub:          sub,
		metrics:      s.metrics,
		metricPrefix: s.cfg.metricPrefix,
	}, nil
}

func (s *natsSubscriber) QueueSubscribe(ctx context.Context, subject, queue string) (Subscription, error) {
	if subject == "" {
		return nil, ErrSubjectRequired
	}

	sub, err := s.conn.QueueSubscribeSync(subject, queue)
	if err != nil {
		return nil, fmt.Errorf("vuhive/nats: queue subscribe failed: %w", err)
	}

	return &natsSubscription{
		sub:          sub,
		metrics:      s.metrics,
		metricPrefix: s.cfg.metricPrefix,
	}, nil
}

func (s *natsSubscriber) Close() error {
	s.conn.Close()
	return nil
}

type natsJetStreamClient struct {
	js      nats.JetStreamContext
	cfg     clientConfig
	metrics vuhive.MetricsCollector
}

func (j *natsJetStreamClient) Publish(ctx context.Context, subject string, data []byte) error {
	if subject == "" {
		return ErrSubjectRequired
	}

	start := time.Now()
	_, err := j.js.Publish(subject, data, nats.Context(ctx))
	duration := time.Since(start)

	recordPubMetrics(j.metrics, j.cfg.metricPrefix, subject, duration, len(data), err)

	if err != nil {
		return fmt.Errorf("vuhive/nats: jetstream publish failed: %w", err)
	}
	return nil
}

func (j *natsJetStreamClient) PublishMsg(ctx context.Context, msg *Message) error {
	if msg == nil {
		return ErrNilMessage
	}
	if msg.Subject == "" {
		return ErrSubjectRequired
	}

	nMsg := &nats.Msg{
		Subject: msg.Subject,
		Data:    msg.Data,
		Reply:   msg.Reply,
	}
	if len(msg.Header) > 0 {
		nMsg.Header = make(nats.Header, len(msg.Header))
		for k, v := range msg.Header {
			nMsg.Header[k] = v
		}
	}

	start := time.Now()
	_, err := j.js.PublishMsg(nMsg, nats.Context(ctx))
	duration := time.Since(start)

	recordPubMetrics(j.metrics, j.cfg.metricPrefix, msg.Subject, duration, len(msg.Data), err)

	if err != nil {
		return fmt.Errorf("vuhive/nats: jetstream publish message failed: %w", err)
	}
	return nil
}

func (j *natsJetStreamClient) Close() error {
	return nil
}

type natsClient struct {
	publisher  *natsPublisher
	subscriber *natsSubscriber
	conn       *nats.Conn
	cfg        clientConfig
	metrics    vuhive.MetricsCollector
}

func (c *natsClient) Publish(ctx context.Context, subject string, data []byte) error {
	return c.publisher.Publish(ctx, subject, data)
}

func (c *natsClient) PublishMsg(ctx context.Context, msg *Message) error {
	return c.publisher.PublishMsg(ctx, msg)
}

func (c *natsClient) Request(ctx context.Context, subject string, data []byte, timeout time.Duration) (*Message, error) {
	return c.publisher.Request(ctx, subject, data, timeout)
}

func (c *natsClient) Subscribe(ctx context.Context, subject string) (Subscription, error) {
	return c.subscriber.Subscribe(ctx, subject)
}

func (c *natsClient) QueueSubscribe(ctx context.Context, subject, queue string) (Subscription, error) {
	return c.subscriber.QueueSubscribe(ctx, subject, queue)
}

func (c *natsClient) JetStream() (JetStreamClient, error) {
	js, err := c.conn.JetStream()
	if err != nil {
		return nil, fmt.Errorf("vuhive/nats: failed to create JetStream context: %w", err)
	}
	return &natsJetStreamClient{
		js:      js,
		cfg:     c.cfg,
		metrics: c.metrics,
	}, nil
}

func (c *natsClient) Close() error {
	c.conn.Close()
	return nil
}

func buildNATSOptions(cfg clientConfig) []nats.Option {
	var opts []nats.Option

	if cfg.name != "" {
		opts = append(opts, nats.Name(cfg.name))
	}
	if cfg.token != "" {
		opts = append(opts, nats.Token(cfg.token))
	}
	if cfg.user != "" || cfg.password != "" {
		opts = append(opts, nats.UserInfo(cfg.user, cfg.password))
	}
	if cfg.userCredentials != "" {
		opts = append(opts, nats.UserCredentials(cfg.userCredentials))
	}
	if cfg.tlsConfig != nil {
		opts = append(opts, nats.Secure(cfg.tlsConfig))
	} else if cfg.tlsInsecureSkipVerify {
		opts = append(opts, nats.Secure(&tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // user explicitly requested insecure skip verify
		}))
	}
	if cfg.timeout > 0 {
		opts = append(opts, nats.Timeout(cfg.timeout))
	}
	if cfg.maxReconnects > 0 {
		opts = append(opts, nats.MaxReconnects(cfg.maxReconnects))
	}
	if cfg.reconnectWait > 0 {
		opts = append(opts, nats.ReconnectWait(cfg.reconnectWait))
	}

	return opts
}

func connectNATS(cfg clientConfig) (*nats.Conn, error) {
	if len(cfg.servers) == 0 {
		return nil, ErrServersRequired
	}
	servers := strings.Join(cfg.servers, ",")
	opts := buildNATSOptions(cfg)
	return nats.Connect(servers, opts...)
}

// NewPublisher creates a new NATS Publisher backed by nats.go.
func NewPublisher(ctx vuhive.SetupContext, opts ...Option) (Publisher, error) {
	return NewPublisherWithCollector(ctx.Metrics(), opts...)
}

// NewPublisherFromVU creates a per-VU NATS Publisher.
func NewPublisherFromVU(ctx vuhive.VUContext, opts ...Option) (Publisher, error) {
	return NewPublisherWithCollector(ctx.Metrics(), opts...)
}

// NewPublisherWithCollector creates a NATS Publisher using the given metrics collector.
func NewPublisherWithCollector(metrics vuhive.MetricsCollector, opts ...Option) (Publisher, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	nc, err := connectNATS(cfg)
	if err != nil {
		return nil, fmt.Errorf("vuhive/nats: failed to connect to nats: %w", err)
	}

	return &natsPublisher{
		conn:    nc,
		cfg:     cfg,
		metrics: metrics,
	}, nil
}

// NewSubscriber creates a new NATS Subscriber backed by nats.go.
func NewSubscriber(ctx vuhive.SetupContext, opts ...Option) (Subscriber, error) {
	return NewSubscriberWithCollector(ctx.Metrics(), opts...)
}

// NewSubscriberFromVU creates a per-VU NATS Subscriber.
func NewSubscriberFromVU(ctx vuhive.VUContext, opts ...Option) (Subscriber, error) {
	return NewSubscriberWithCollector(ctx.Metrics(), opts...)
}

// NewSubscriberWithCollector creates a NATS Subscriber using the given metrics collector.
func NewSubscriberWithCollector(metrics vuhive.MetricsCollector, opts ...Option) (Subscriber, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	nc, err := connectNATS(cfg)
	if err != nil {
		return nil, fmt.Errorf("vuhive/nats: failed to connect to nats: %w", err)
	}

	return &natsSubscriber{
		conn:    nc,
		cfg:     cfg,
		metrics: metrics,
	}, nil
}

// NewClient creates a unified NATS Client (Publisher + Subscriber + JetStream).
func NewClient(ctx vuhive.SetupContext, opts ...Option) (Client, error) {
	return NewClientWithCollector(ctx.Metrics(), opts...)
}

// NewClientFromVU creates a per-VU unified NATS Client.
func NewClientFromVU(ctx vuhive.VUContext, opts ...Option) (Client, error) {
	return NewClientWithCollector(ctx.Metrics(), opts...)
}

// NewClientWithCollector creates a unified NATS Client using the given metrics collector.
func NewClientWithCollector(metrics vuhive.MetricsCollector, opts ...Option) (Client, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	nc, err := connectNATS(cfg)
	if err != nil {
		return nil, fmt.Errorf("vuhive/nats: failed to connect to nats: %w", err)
	}

	pub := &natsPublisher{conn: nc, cfg: cfg, metrics: metrics}
	sub := &natsSubscriber{conn: nc, cfg: cfg, metrics: metrics}

	return &natsClient{
		publisher:  pub,
		subscriber: sub,
		conn:       nc,
		cfg:        cfg,
		metrics:    metrics,
	}, nil
}

// Compile-time interface satisfaction checks.
var (
	_ Publisher       = (*natsPublisher)(nil)
	_ Subscriber      = (*natsSubscriber)(nil)
	_ Subscription    = (*natsSubscription)(nil)
	_ JetStreamClient = (*natsJetStreamClient)(nil)
	_ Client          = (*natsClient)(nil)
)
