package nats

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrNATSDisabled is returned when NATS operations are invoked on a binary built without the 'nats' tag.
	ErrNATSDisabled = errors.New("vuhive: nats module disabled (recompile with '-tags nats')")

	// ErrSubjectRequired is returned when an operation requires a subject but none was provided.
	ErrSubjectRequired = errors.New("vuhive: nats subject is required")

	// ErrNilMessage is returned when attempting to publish a nil message.
	ErrNilMessage = errors.New("vuhive: message cannot be nil")

	// ErrServersRequired is returned when a client is created without specifying any server addresses.
	ErrServersRequired = errors.New("vuhive: at least one nats server address is required")

	// ErrJetStreamDisabled is returned when JetStream operations are invoked but JetStream is not available or enabled.
	ErrJetStreamDisabled = errors.New("vuhive: jetstream is not enabled or available")

	// ErrTimeout is returned when a synchronous NATS operation times out.
	ErrTimeout = errors.New("vuhive: nats operation timed out")
)

const defaultMetricPrefix = "vuhive.nats."

// Built-in metric name suffixes for NATS telemetry.
const (
	MetricSuffixPubDuration      = "pub_duration"
	MetricSuffixPubTotal         = "pub_total"
	MetricSuffixPubBytes         = "pub_bytes"
	MetricSuffixPubFailed        = "pub_failed"
	MetricSuffixReqDuration      = "req_duration"
	MetricSuffixSubReceivedTotal = "sub_received_total"
	MetricSuffixSubBytes         = "sub_bytes"
	MetricSuffixSubFailed        = "sub_failed"
)

// Message represents a NATS message for publishing, consuming, or request-reply.
type Message struct {
	// Subject is the destination subject name.
	Subject string

	// Data is the message payload.
	Data []byte

	// Header contains optional key-value metadata headers.
	Header map[string][]string

	// Reply is the optional reply subject (for request-reply workflows).
	Reply string
}

// Publisher provides message publishing and request-reply capabilities.
// Publisher implementations are safe for concurrent use across multiple VU goroutines.
type Publisher interface {
	// Publish publishes a message with the given subject and data payload.
	Publish(ctx context.Context, subject string, data []byte) error

	// PublishMsg publishes a structured Message containing headers and/or reply subject.
	PublishMsg(ctx context.Context, msg *Message) error

	// Request sends a request message with payload and waits for a reply within the specified timeout.
	Request(ctx context.Context, subject string, data []byte, timeout time.Duration) (*Message, error)

	// Close closes the publisher connection and flushes any buffered data.
	Close() error
}

// Subscriber provides synchronous subscription capabilities.
// Subscriber implementations are safe for concurrent use across multiple VU goroutines.
type Subscriber interface {
	// Subscribe creates a synchronous subscription to the specified subject.
	Subscribe(ctx context.Context, subject string) (Subscription, error)

	// QueueSubscribe creates a synchronous queue subscription for distributed load balancing across workers.
	QueueSubscribe(ctx context.Context, subject, queue string) (Subscription, error)

	// Close closes the subscriber and its active subscriptions.
	Close() error
}

// Subscription represents an active subscription handle.
type Subscription interface {
	// NextMsg retrieves the next message on the subscription, waiting up to timeout duration.
	// If the context is canceled or the timeout expires, an error is returned.
	NextMsg(ctx context.Context, timeout time.Duration) (*Message, error)

	// Unsubscribe cancels the active subscription.
	Unsubscribe() error
}

// JetStreamClient provides streaming message publication over NATS JetStream.
type JetStreamClient interface {
	// Publish publishes a message to a JetStream-enabled stream.
	Publish(ctx context.Context, subject string, data []byte) error

	// PublishMsg publishes a structured message to a JetStream-enabled stream.
	PublishMsg(ctx context.Context, msg *Message) error

	// Close closes the JetStream context or releases resources.
	Close() error
}

// Client combines Publisher, Subscriber, and JetStream capabilities into a unified handle.
type Client interface {
	Publisher
	Subscriber

	// JetStream returns a JetStreamClient instance for stream-oriented operations.
	JetStream() (JetStreamClient, error)
}
