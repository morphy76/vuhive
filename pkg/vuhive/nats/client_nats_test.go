//go:build nats

package nats_test

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	natslib "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/morphy76/vuhive/internal/metric"
	"github.com/morphy76/vuhive/pkg/vuhive"
	"github.com/morphy76/vuhive/pkg/vuhive/nats"
)

type metricsTestAdapter struct {
	collector metric.Collector
}

func (m *metricsTestAdapter) Counter(name string, tags vuhive.Tags) vuhive.Counter {
	return m.collector.Counter(name, metric.Tags(tags))
}

func (m *metricsTestAdapter) Gauge(name string, tags vuhive.Tags) vuhive.Gauge {
	return m.collector.Gauge(name, metric.Tags(tags))
}

func (m *metricsTestAdapter) Duration(name string, tags vuhive.Tags) vuhive.Duration {
	return m.collector.Duration(name, metric.Tags(tags))
}

func (m *metricsTestAdapter) Rate(name string, tags vuhive.Tags) vuhive.Rate {
	return m.collector.Rate(name, metric.Tags(tags))
}

func newTestStore() (*metric.Store, vuhive.MetricsCollector) {
	store := metric.NewStore()
	return store, &metricsTestAdapter{collector: store}
}

func startInProcessNATSServer(t *testing.T, js bool) (*server.Server, string) {
	t.Helper()
	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      server.RANDOM_PORT,
		NoLog:     true,
		NoSigs:    true,
		JetStream: js,
	}
	if js {
		opts.StoreDir = t.TempDir()
	}
	s, err := server.NewServer(opts)
	require.NoError(t, err)
	s.Start()
	if !s.ReadyForConnections(10 * time.Second) {
		t.Fatalf("unable to start NATS server on %s:%d", opts.Host, opts.Port)
	}
	t.Cleanup(func() {
		s.Shutdown()
		s.WaitForShutdown()
	})
	return s, s.ClientURL()
}

func TestNATSPublisher_Publish_SingleMessage(t *testing.T) {
	_, url := startInProcessNATSServer(t, false)
	store, collector := newTestStore()

	pub, err := nats.NewPublisherWithCollector(collector, nats.WithURL(url))
	require.NoError(t, err)
	defer pub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = pub.Publish(ctx, "orders.created", []byte(`{"id":"ord-1"}`))
	require.NoError(t, err)

	assert.Equal(t, int64(1), store.AggregatedCounterValue(vuhive.MetricNATSPubTotal))
	assert.Equal(t, int64(1), store.MergedHistogramSnapshot(vuhive.MetricNATSPubDuration).Count)
	assert.Equal(t, 0.0, store.AggregatedRateValue(vuhive.MetricNATSPubFailed))
	assert.True(t, store.AggregatedCounterValue(vuhive.MetricNATSPubBytes) > 0)
}

func TestNATSPublisher_PublishMsg_WithHeaders(t *testing.T) {
	_, url := startInProcessNATSServer(t, false)
	store, collector := newTestStore()

	client, err := nats.NewClientWithCollector(collector, nats.WithURL(url))
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sub, err := client.Subscribe(ctx, "orders.headers")
	require.NoError(t, err)
	defer sub.Unsubscribe()

	msg := &nats.Message{
		Subject: "orders.headers",
		Data:    []byte("hello world"),
		Header: map[string][]string{
			"Trace-ID": {"trace-999"},
			"Custom":   {"v1", "v2"},
		},
	}

	err = client.PublishMsg(ctx, msg)
	require.NoError(t, err)

	recv, err := sub.NextMsg(ctx, 2*time.Second)
	require.NoError(t, err)
	require.NotNil(t, recv)

	assert.Equal(t, "orders.headers", recv.Subject)
	assert.Equal(t, []byte("hello world"), recv.Data)
	assert.Equal(t, []string{"trace-999"}, recv.Header["Trace-ID"])
	assert.Equal(t, []string{"v1", "v2"}, recv.Header["Custom"])

	assert.Equal(t, int64(1), store.AggregatedCounterValue(vuhive.MetricNATSPubTotal))
	assert.Equal(t, int64(1), store.AggregatedCounterValue(vuhive.MetricNATSSubReceivedTotal))
}

func TestNATSPublisher_RequestReply(t *testing.T) {
	_, url := startInProcessNATSServer(t, false)
	store, collector := newTestStore()

	client, err := nats.NewClientWithCollector(collector, nats.WithURL(url))
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start a responder in background
	sub, err := client.Subscribe(ctx, "service.rpc")
	require.NoError(t, err)
	defer sub.Unsubscribe()

	responderDone := make(chan struct{})
	go func() {
		defer close(responderDone)
		reqMsg, err := sub.NextMsg(context.Background(), 3*time.Second)
		if err == nil && reqMsg != nil && reqMsg.Reply != "" {
			_ = client.Publish(context.Background(), reqMsg.Reply, []byte("pong"))
		}
	}()

	resp, err := client.Request(ctx, "service.rpc", []byte("ping"), 2*time.Second)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, []byte("pong"), resp.Data)

	<-responderDone

	assert.Equal(t, int64(1), store.MergedHistogramSnapshot(vuhive.MetricNATSReqDuration).Count)
	assert.Equal(t, 0.0, store.AggregatedRateValue(vuhive.MetricNATSPubFailed))
}

func TestNATSSubscriber_QueueSubscribe(t *testing.T) {
	_, url := startInProcessNATSServer(t, false)
	store, collector := newTestStore()

	client, err := nats.NewClientWithCollector(collector, nats.WithURL(url))
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sub1, err := client.QueueSubscribe(ctx, "jobs", "workers")
	require.NoError(t, err)
	defer sub1.Unsubscribe()

	sub2, err := client.QueueSubscribe(ctx, "jobs", "workers")
	require.NoError(t, err)
	defer sub2.Unsubscribe()

	// Publish 4 messages
	for i := 0; i < 4; i++ {
		err = client.Publish(ctx, "jobs", []byte("task"))
		require.NoError(t, err)
	}

	receivedCount := 0
	readChan := make(chan *nats.Message, 4)

	go func() {
		for {
			m, err := sub1.NextMsg(context.Background(), 200*time.Millisecond)
			if err != nil {
				return
			}
			readChan <- m
		}
	}()

	go func() {
		for {
			m, err := sub2.NextMsg(context.Background(), 200*time.Millisecond)
			if err != nil {
				return
			}
			readChan <- m
		}
	}()

	timeout := time.After(2 * time.Second)
	for receivedCount < 4 {
		select {
		case <-readChan:
			receivedCount++
		case <-timeout:
			t.Fatalf("timed out waiting for 4 queue messages, got %d", receivedCount)
		}
	}

	assert.Equal(t, 4, receivedCount)
	assert.Equal(t, int64(4), store.AggregatedCounterValue(vuhive.MetricNATSSubReceivedTotal))
}

func TestNATSSubscription_Timeout(t *testing.T) {
	_, url := startInProcessNATSServer(t, false)
	store, collector := newTestStore()

	sub, err := nats.NewSubscriberWithCollector(collector, nats.WithURL(url))
	require.NoError(t, err)
	defer sub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	subscription, err := sub.Subscribe(ctx, "empty.topic")
	require.NoError(t, err)
	defer subscription.Unsubscribe()

	msg, err := subscription.NextMsg(ctx, 50*time.Millisecond)
	assert.Nil(t, msg)
	assert.ErrorIs(t, err, nats.ErrTimeout)

	assert.Equal(t, 1.0, store.AggregatedRateValue(vuhive.MetricNATSSubFailed))
}

func TestNATSJetStream_Publish(t *testing.T) {
	_, url := startInProcessNATSServer(t, true)
	store, collector := newTestStore()

	// Pre-create stream
	nc, err := natslib.Connect(url)
	require.NoError(t, err)
	jsSetup, err := nc.JetStream()
	require.NoError(t, err)
	_, err = jsSetup.AddStream(&natslib.StreamConfig{
		Name:     "TEST_STREAM",
		Subjects: []string{"js.>"},
	})
	require.NoError(t, err)
	nc.Close()

	client, err := nats.NewClientWithCollector(collector,
		nats.WithURL(url),
		nats.WithJetStream(true),
	)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	js, err := client.JetStream()
	require.NoError(t, err)
	require.NotNil(t, js)
	defer js.Close()

	err = js.Publish(ctx, "js.events", []byte("stream-event-1"))
	require.NoError(t, err)

	err = js.PublishMsg(ctx, &nats.Message{
		Subject: "js.events",
		Data:    []byte("stream-event-2"),
		Header:  map[string][]string{"Event-Type": {"audit"}},
	})
	require.NoError(t, err)

	assert.Equal(t, int64(2), store.AggregatedCounterValue(vuhive.MetricNATSPubTotal))
}

func TestNATSPublisher_ValidationErrors(t *testing.T) {
	_, url := startInProcessNATSServer(t, false)
	_, collector := newTestStore()

	pub, err := nats.NewPublisherWithCollector(collector, nats.WithURL(url))
	require.NoError(t, err)
	defer pub.Close()

	ctx := context.Background()

	// Missing subject
	err = pub.Publish(ctx, "", []byte("test"))
	assert.ErrorIs(t, err, nats.ErrSubjectRequired)

	// Nil message
	err = pub.PublishMsg(ctx, nil)
	assert.ErrorIs(t, err, nats.ErrNilMessage)

	// Nil message subject
	err = pub.PublishMsg(ctx, &nats.Message{Subject: ""})
	assert.ErrorIs(t, err, nats.ErrSubjectRequired)
}

func TestNATSOptions_CustomPrefix(t *testing.T) {
	_, url := startInProcessNATSServer(t, false)
	store, collector := newTestStore()

	pub, err := nats.NewPublisherWithCollector(collector,
		nats.WithURL(url),
		nats.WithCustomMetricPrefix("custom.nats."),
	)
	require.NoError(t, err)
	defer pub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = pub.Publish(ctx, "custom.topic", []byte("payload"))
	require.NoError(t, err)

	assert.Equal(t, int64(1), store.AggregatedCounterValue("custom.nats.pub_total"))
}
