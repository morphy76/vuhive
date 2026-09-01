//go:build !nats

package nats

import (
	"context"
	"time"

	"github.com/morphy76/vuhive/pkg/vuhive"
)

type noopPublisher struct {
	cfg clientConfig
}

func (p *noopPublisher) Publish(ctx context.Context, subject string, data []byte) error {
	return ErrNATSDisabled
}

func (p *noopPublisher) PublishMsg(ctx context.Context, msg *Message) error {
	return ErrNATSDisabled
}

func (p *noopPublisher) Request(ctx context.Context, subject string, data []byte, timeout time.Duration) (*Message, error) {
	return nil, ErrNATSDisabled
}

func (p *noopPublisher) Close() error {
	return nil
}

type noopSubscription struct{}

func (s *noopSubscription) NextMsg(ctx context.Context, timeout time.Duration) (*Message, error) {
	return nil, ErrNATSDisabled
}

func (s *noopSubscription) Unsubscribe() error {
	return nil
}

type noopSubscriber struct {
	cfg clientConfig
}

func (s *noopSubscriber) Subscribe(ctx context.Context, subject string) (Subscription, error) {
	return nil, ErrNATSDisabled
}

func (s *noopSubscriber) QueueSubscribe(ctx context.Context, subject, queue string) (Subscription, error) {
	return nil, ErrNATSDisabled
}

func (s *noopSubscriber) Close() error {
	return nil
}

type noopJetStreamClient struct{}

func (j *noopJetStreamClient) Publish(ctx context.Context, subject string, data []byte) error {
	return ErrNATSDisabled
}

func (j *noopJetStreamClient) PublishMsg(ctx context.Context, msg *Message) error {
	return ErrNATSDisabled
}

func (j *noopJetStreamClient) Close() error {
	return nil
}

type noopClient struct {
	noopPublisher
	noopSubscriber
}

func (c *noopClient) JetStream() (JetStreamClient, error) {
	return nil, ErrNATSDisabled
}

func (c *noopClient) Close() error {
	return nil
}

// NewPublisher creates a new NATS Publisher. When built without the 'nats' tag,
// a no-op implementation is returned and calls will return ErrNATSDisabled.
func NewPublisher(ctx vuhive.SetupContext, opts ...Option) (Publisher, error) {
	return NewPublisherWithCollector(ctx.Metrics(), opts...)
}

// NewPublisherFromVU creates a per-VU NATS Publisher. When built without the 'nats' tag,
// a no-op implementation is returned and calls will return ErrNATSDisabled.
func NewPublisherFromVU(ctx vuhive.VUContext, opts ...Option) (Publisher, error) {
	return NewPublisherWithCollector(ctx.Metrics(), opts...)
}

// NewPublisherWithCollector creates a NATS Publisher using the given metrics collector.
func NewPublisherWithCollector(metrics vuhive.MetricsCollector, opts ...Option) (Publisher, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	return &noopPublisher{cfg: cfg}, nil
}

// NewSubscriber creates a new NATS Subscriber. When built without the 'nats' tag,
// a no-op implementation is returned and calls will return ErrNATSDisabled.
func NewSubscriber(ctx vuhive.SetupContext, opts ...Option) (Subscriber, error) {
	return NewSubscriberWithCollector(ctx.Metrics(), opts...)
}

// NewSubscriberFromVU creates a per-VU NATS Subscriber. When built without the 'nats' tag,
// a no-op implementation is returned and calls will return ErrNATSDisabled.
func NewSubscriberFromVU(ctx vuhive.VUContext, opts ...Option) (Subscriber, error) {
	return NewSubscriberWithCollector(ctx.Metrics(), opts...)
}

// NewSubscriberWithCollector creates a NATS Subscriber using the given metrics collector.
func NewSubscriberWithCollector(metrics vuhive.MetricsCollector, opts ...Option) (Subscriber, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	return &noopSubscriber{cfg: cfg}, nil
}

// NewClient creates a unified NATS Client (Publisher + Subscriber + JetStream). When built without the 'nats' tag,
// a no-op implementation is returned and operations will return ErrNATSDisabled.
func NewClient(ctx vuhive.SetupContext, opts ...Option) (Client, error) {
	return NewClientWithCollector(ctx.Metrics(), opts...)
}

// NewClientFromVU creates a per-VU unified NATS Client. When built without the 'nats' tag,
// a no-op implementation is returned and operations will return ErrNATSDisabled.
func NewClientFromVU(ctx vuhive.VUContext, opts ...Option) (Client, error) {
	return NewClientWithCollector(ctx.Metrics(), opts...)
}

// NewClientWithCollector creates a unified NATS Client using the given metrics collector.
func NewClientWithCollector(metrics vuhive.MetricsCollector, opts ...Option) (Client, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	return &noopClient{
		noopPublisher:  noopPublisher{cfg: cfg},
		noopSubscriber: noopSubscriber{cfg: cfg},
	}, nil
}

// Compile-time interface satisfaction checks.
var (
	_ Publisher       = (*noopPublisher)(nil)
	_ Subscriber      = (*noopSubscriber)(nil)
	_ Subscription    = (*noopSubscription)(nil)
	_ JetStreamClient = (*noopJetStreamClient)(nil)
	_ Client          = (*noopClient)(nil)
)
