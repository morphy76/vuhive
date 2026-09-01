// Package nats provides auto-instrumented NATS Publisher and Subscriber clients
// for load and stress testing messaging systems and event-driven architectures with vuhive.
//
// # Conditional Compilation
//
// To prevent binary bloat and external dependency trees in standard builds, the concrete
// NATS client is conditionally compiled using the "nats" build tag:
//
//   - Default builds (go build . / go test ./...): Uses a lightweight, zero-dependency no-op
//     driver. Client constructors succeed safely, but invocations of operations return
//     ErrNATSDisabled to inform developers to recompile with the tag.
//   - NATS builds (go build -tags nats .): Compiles the full high-throughput driver powered
//     by the official Go NATS client (github.com/nats-io/nats.go).
//
// # Automatic Telemetry Metrics
//
// Operations automatically emit metrics into the scenario's metric collector:
//   - vuhive.nats.pub_duration (Duration): Publish latency histogram
//   - vuhive.nats.pub_total (Counter): Total messages published
//   - vuhive.nats.pub_bytes (Counter): Total payload bytes published
//   - vuhive.nats.pub_failed (Rate): Ratio of failed publish attempts
//   - vuhive.nats.req_duration (Duration): Request-reply round-trip latency histogram
//   - vuhive.nats.sub_received_total (Counter): Total messages received
//   - vuhive.nats.sub_bytes (Counter): Total payload bytes received
//   - vuhive.nats.sub_failed (Rate): Ratio of failed receive attempts
//
// # Basic Usage
//
//	// In Setup: initialize shared publisher, subscriber, or unified client
//	client, err := nats.NewClient(ctx,
//	    nats.WithURL("nats://localhost:4222"),
//	)
//
//	// In RunVU: publish or request — metrics are recorded automatically
//	err := client.Publish(ctx, "orders.created", []byte(`{"order_id":"ord-1001"}`))
//
//	// Request-reply RPC
//	reply, err := client.Request(ctx, "orders.process", []byte(`{"order_id":"ord-1001"}`), 2*time.Second)
package nats
