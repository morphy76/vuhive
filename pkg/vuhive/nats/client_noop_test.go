//go:build !nats

package nats_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/morphy76/vuhive/internal/metric"
	"github.com/morphy76/vuhive/pkg/vuhive"
	"github.com/morphy76/vuhive/pkg/vuhive/nats"
)

type metricsAdapter struct {
	collector metric.Collector
}

func (m *metricsAdapter) Counter(name string, tags vuhive.Tags) vuhive.Counter {
	return m.collector.Counter(name, metric.Tags(tags))
}

func (m *metricsAdapter) Gauge(name string, tags vuhive.Tags) vuhive.Gauge {
	return m.collector.Gauge(name, metric.Tags(tags))
}

func (m *metricsAdapter) Duration(name string, tags vuhive.Tags) vuhive.Duration {
	return m.collector.Duration(name, metric.Tags(tags))
}

func (m *metricsAdapter) Rate(name string, tags vuhive.Tags) vuhive.Rate {
	return m.collector.Rate(name, metric.Tags(tags))
}

func newTestCollector() vuhive.MetricsCollector {
	return &metricsAdapter{collector: metric.NewStore()}
}

func TestNoopPublisher_ReturnsErrNATSDisabled(t *testing.T) {
	metrics := newTestCollector()
	pub, err := nats.NewPublisherWithCollector(metrics, nats.WithURL("nats://localhost:4222"))
	require.NoError(t, err)
	require.NotNil(t, pub)

	ctx := context.Background()

	err = pub.Publish(ctx, "orders", []byte("payload"))
	assert.ErrorIs(t, err, nats.ErrNATSDisabled)

	err = pub.PublishMsg(ctx, &nats.Message{Subject: "orders", Data: []byte("payload")})
	assert.ErrorIs(t, err, nats.ErrNATSDisabled)

	resp, err := pub.Request(ctx, "orders", []byte("req"), time.Second)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, nats.ErrNATSDisabled)

	err = pub.Close()
	assert.NoError(t, err)
}

func TestNoopSubscriber_ReturnsErrNATSDisabled(t *testing.T) {
	metrics := newTestCollector()
	sub, err := nats.NewSubscriberWithCollector(metrics, nats.WithURL("nats://localhost:4222"))
	require.NoError(t, err)
	require.NotNil(t, sub)

	ctx := context.Background()

	subscription, err := sub.Subscribe(ctx, "orders")
	assert.Nil(t, subscription)
	assert.ErrorIs(t, err, nats.ErrNATSDisabled)

	queueSub, err := sub.QueueSubscribe(ctx, "orders", "workers")
	assert.Nil(t, queueSub)
	assert.ErrorIs(t, err, nats.ErrNATSDisabled)

	err = sub.Close()
	assert.NoError(t, err)
}

func TestNoopSubscription_ReturnsErrNATSDisabled(t *testing.T) {
	metrics := newTestCollector()
	sub, err := nats.NewSubscriberWithCollector(metrics)
	require.NoError(t, err)

	ctx := context.Background()
	subscription, err := sub.Subscribe(ctx, "orders")
	assert.Nil(t, subscription)
	assert.ErrorIs(t, err, nats.ErrNATSDisabled)
}

func TestNoopClient_ReturnsErrNATSDisabled(t *testing.T) {
	metrics := newTestCollector()
	client, err := nats.NewClientWithCollector(metrics, nats.WithURL("nats://localhost:4222"))
	require.NoError(t, err)
	require.NotNil(t, client)

	ctx := context.Background()

	err = client.Publish(ctx, "orders", []byte("payload"))
	assert.ErrorIs(t, err, nats.ErrNATSDisabled)

	err = client.PublishMsg(ctx, &nats.Message{Subject: "orders", Data: []byte("payload")})
	assert.ErrorIs(t, err, nats.ErrNATSDisabled)

	resp, err := client.Request(ctx, "orders", []byte("req"), time.Second)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, nats.ErrNATSDisabled)

	subscription, err := client.Subscribe(ctx, "orders")
	assert.Nil(t, subscription)
	assert.ErrorIs(t, err, nats.ErrNATSDisabled)

	queueSub, err := client.QueueSubscribe(ctx, "orders", "workers")
	assert.Nil(t, queueSub)
	assert.ErrorIs(t, err, nats.ErrNATSDisabled)

	js, err := client.JetStream()
	assert.Nil(t, js)
	assert.ErrorIs(t, err, nats.ErrNATSDisabled)

	err = client.Close()
	assert.NoError(t, err)
}
