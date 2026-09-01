//go:build vuhive_example && nats

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/nats-io/nats-server/v2/server"

	"github.com/morphy76/vuhive/pkg/vuhive"
	"github.com/morphy76/vuhive/pkg/vuhive/nats"
)

// --- In-Process Simulation Harness ---
// For demonstration and standalone testing, we launch a lightweight in-process
// NATS server. In production load tests, point WithURL() to your real NATS cluster address.

func startMockServer() (*server.Server, string, error) {
	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      server.RANDOM_PORT,
		NoLog:     true,
		NoSigs:    true,
		JetStream: false,
	}
	s, err := server.NewServer(opts)
	if err != nil {
		return nil, "", err
	}
	s.Start()
	if !s.ReadyForConnections(10 * time.Second) {
		s.Shutdown()
		return nil, "", fmt.Errorf("in-process NATS server failed to become ready")
	}
	return s, s.ClientURL(), nil
}

func main() {
	// 1. Start in-process mock NATS server for standalone execution
	mockServer, natsURL, err := startMockServer()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start mock NATS server: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		mockServer.Shutdown()
		mockServer.WaitForShutdown()
	}()

	// 2. Create vuhive test suite
	suite := vuhive.NewSuite("NATS Messaging & RPC Load Test")

	// 3. Register scenario demonstrating publisher, subscriber, and request-reply workflows
	suite.RegisterScenario("nats_messaging_flow", vuhive.Scenario{
		// Setup: Initialize shared NATS Client and background responder
		Setup: func(ctx vuhive.SetupContext) (map[string]any, error) {
			client, err := nats.NewClient(ctx,
				nats.WithURL(natsURL),
				nats.WithName("vuhive-nats-loadtest"),
				nats.WithTimeout(5*time.Second),
			)
			if err != nil {
				return nil, fmt.Errorf("failed to create nats client: %w", err)
			}

			// Background responder for Request-Reply RPC pattern
			rpcSub, err := client.Subscribe(ctx, "orders.rpc.process")
			if err != nil {
				_ = client.Close()
				return nil, fmt.Errorf("failed to subscribe to RPC subject: %w", err)
			}

			stopChan := make(chan struct{})
			go func() {
				for {
					select {
					case <-stopChan:
						return
					default:
						req, err := rpcSub.NextMsg(context.Background(), 200*time.Millisecond)
						if err != nil {
							continue
						}
						if req != nil && req.Reply != "" {
							_ = client.Publish(context.Background(), req.Reply, []byte(`{"status":"ACCEPTED"}`))
						}
					}
				}
			}()

			return map[string]any{
				"nats_client": client,
				"stop_chan":   stopChan,
			}, nil
		},

		// RunVU: Publish notification event and invoke Request-Reply RPC
		RunVU: func(ctx vuhive.VUContext) error {
			client := vuhive.MustState[nats.Client](ctx, "nats_client")

			orderID := fmt.Sprintf("order-%d-%d", ctx.VUID(), ctx.Iteration())
			payload := fmt.Sprintf(`{"order_id":"%s","amount":149.90,"currency":"USD"}`, orderID)

			// Step 1: Fire-and-forget notification publish with headers
			msg := &nats.Message{
				Subject: "orders.events.created",
				Data:    []byte(payload),
				Header: map[string][]string{
					"Source":    {"vuhive-load-runner"},
					"Timestamp": {time.Now().Format(time.RFC3339Nano)},
				},
			}

			if err := client.PublishMsg(ctx, msg); err != nil {
				return fmt.Errorf("publish failed: %w", err)
			}

			ctx.Check("publish_succeeded", func() string {
				return ""
			})

			// Step 2: Synchronous Request-Reply RPC round-trip
			reply, err := client.Request(ctx, "orders.rpc.process", []byte(payload), 3*time.Second)
			if err != nil {
				return fmt.Errorf("rpc request failed: %w", err)
			}

			// Step 3: Validate RPC response payload
			ctx.Check("rpc_response_accepted", func() string {
				if string(reply.Data) != `{"status":"ACCEPTED"}` {
					return fmt.Sprintf("unexpected reply: %s", string(reply.Data))
				}
				return ""
			})

			return nil
		},

		// Teardown: Stop background worker and close client
		Teardown: func(ctx vuhive.TeardownContext, state map[string]any) error {
			if stopChan, ok := state["stop_chan"].(chan struct{}); ok && stopChan != nil {
				close(stopChan)
			}
			if client, ok := state["nats_client"].(nats.Client); ok && client != nil {
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
