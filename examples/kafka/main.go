//go:build vuhive_example && kafka

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/twmb/franz-go/pkg/kfake"

	"github.com/morphy76/vuhive/pkg/vuhive"
	"github.com/morphy76/vuhive/pkg/vuhive/kafka"
)

// --- In-Process Simulation Harness ---
// For demonstration and standalone testing, we launch a lightweight in-process
// Kafka cluster using kfake. In production load tests, point WithBrokers() to your
// real Kafka broker seed addresses.

func startMockCluster() (*kfake.Cluster, []string, error) {
	cluster, err := kfake.NewCluster(
		kfake.AllowAutoTopicCreation(),
		kfake.SeedTopics(1, "orders_stream"),
	)
	if err != nil {
		return nil, nil, err
	}
	return cluster, cluster.ListenAddrs(), nil
}

func main() {
	// 1. Start mock cluster for standalone execution
	cluster, brokers, err := startMockCluster()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start mock Kafka cluster: %v\n", err)
		os.Exit(1)
	}
	defer cluster.Close()

	// 2. Create vuhive test suite
	suite := vuhive.NewSuite("Kafka Event Stream Load Test")

	// 3. Register scenario demonstrating publisher and consumer workflows
	suite.RegisterScenario("kafka_event_stream", vuhive.Scenario{
		// Setup: Initialize shared Kafka Client
		Setup: func(ctx vuhive.SetupContext) (map[string]any, error) {
			topic := ctx.Param("topic")
			if topic == "" {
				topic = "orders_stream"
			}

			client, err := kafka.NewClient(ctx,
				kafka.WithBrokers(brokers...),
				kafka.WithTopic(topic),
				kafka.WithGroupID("vuhive_order_processors"),
				kafka.WithTimeout(5*time.Second),
				kafka.WithStartOffset(-2), // read from oldest
			)
			if err != nil {
				return nil, fmt.Errorf("failed to create kafka client: %w", err)
			}

			return map[string]any{
				"kafka_client": client,
				"topic":        topic,
			}, nil
		},

		// RunVU: Produce event, validate, then consume and commit
		RunVU: func(ctx vuhive.VUContext) error {
			client := vuhive.MustState[kafka.Client](ctx, "kafka_client")
			topic := vuhive.MustState[string](ctx, "topic")

			// Step 1: Publish an event record
			orderID := fmt.Sprintf("order-%d-%d", ctx.VUID(), ctx.Iteration())
			payload := fmt.Sprintf(`{"order_id":"%s","amount":99.95,"currency":"EUR"}`, orderID)

			msg := &kafka.Message{
				Topic: topic,
				Key:   []byte(orderID),
				Value: []byte(payload),
				Headers: map[string][]byte{
					"source":    []byte("vuhive-vu"),
					"timestamp": []byte(time.Now().Format(time.RFC3339Nano)),
				},
			}

			if err := client.Publish(ctx, msg); err != nil {
				return fmt.Errorf("publish failed: %w", err)
			}

			// Step 2: Validate publish with an inline check
			ctx.Check("publish_successful", func() string {
				// Publish returned no error
				return ""
			})

			// Step 3: Consume event from the stream
			recvMsg, err := client.Consume(ctx)
			if err != nil {
				return fmt.Errorf("consume failed: %w", err)
			}

			if recvMsg != nil {
				// Validate message topic and key
				ctx.Check("message_topic_matches", func() string {
					if recvMsg.Topic != topic {
						return fmt.Sprintf("expected topic %s, got %s", topic, recvMsg.Topic)
					}
					return ""
				})

				// Step 4: Commit consumer offset
				if err := client.Commit(ctx, recvMsg); err != nil {
					return fmt.Errorf("commit failed: %w", err)
				}
			}

			return nil
		},

		// Teardown: Close client connections
		Teardown: func(ctx vuhive.TeardownContext, state map[string]any) error {
			if client, ok := state["kafka_client"].(kafka.Client); ok && client != nil {
				return client.Close()
			}
			return nil
		},
	})

	// 4. Execute suite
	res := suite.Execute()
	if res.Error != nil {
		fmt.Fprintf(os.Stderr, "Execution error: %v\n", res.Error)
	}
	os.Exit(res.ExitCode())
}
