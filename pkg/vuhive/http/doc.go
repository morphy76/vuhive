// Package http provides an instrumented HTTP client and middleware for the vuhive load testing framework.
//
// The Client and Instrument wrapper automatically record latency, status code counters, and error rates for
// every request, eliminating boilerplate metric recording from RunVU hooks.
//
// Basic usage (built-in Client):
//
//	// In Setup: create a shared instrumented client
//	client := vuhivehttp.NewClient(ctx,
//	    vuhivehttp.WithTimeout(5*time.Second),
//	    vuhivehttp.WithHeader("Authorization", "Bearer "+token),
//	)
//
//	// In RunVU: execute requests — metrics are recorded automatically
//	resp, err := client.Get(ctx, serverURL+"/api/checkout")
//	if err != nil {
//	    return err
//	}
//	var result CheckoutResult
//	if err := resp.JSON(&result); err != nil {
//	    return err
//	}
//
// Wrapping standard *http.Client & Third-Party SDKs (vuhivehttp.Instrument):
//
//	// In Setup: instrument any pre-configured standard *http.Client or SDK
//	baseClient := &http.Client{Timeout: 5 * time.Second}
//	client := vuhivehttp.Instrument(baseClient, vuhivehttp.WithMetricPrefix("vuhive.http."))
//
//	// In RunVU: execute standard http requests — telemetry is recorded dynamically from context
//	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.com/data", nil)
//	resp, err := client.Do(req)
//	if err != nil {
//	    return err
//	}
//	defer resp.Body.Close()
//
// Server-Sent Events (SSE) streaming usage:
//
//	// In RunVU: establish SSE stream and consume events iteratively
//	stream, err := client.StreamSSE(ctx, "/v1/chat/completions/stream")
//	if err != nil {
//	    return err
//	}
//	defer stream.Close()
//
//	for stream.Next() {
//	    event := stream.Event()
//	    ctx.Check("valid_event", event.Event == "token" || event.Event == "message")
//	}
//	if err := stream.Err(); err != nil {
//	    return err
//	}
//
// Automatic metrics recorded per request:
//   - vuhive.http.req_duration (Duration): total request latency
//   - vuhive.http.req_failed (Rate): failed vs. total request ratio
//   - vuhive.http.reqs (Counter): total request count
//
// Automatic metrics recorded for SSE streaming:
//   - vuhive.http.sse.connections_total (Counter): total SSE connection attempts
//   - vuhive.http.sse.connect_duration (Duration): handshake/connect latency
//   - vuhive.http.sse.events_total (Counter): total decoded SSE events
//   - vuhive.http.sse.event_latency (Duration): inter-arrival latency between events
//   - vuhive.http.sse.stream_duration (Duration): total active lifespan of stream session
//   - vuhive.http.sse.errors_total (Counter): stream disconnections, read errors, or framing errors
//
// Opt-in phase-breakdown metrics (enabled via WithDetailedTiming or WithInstrumentDetailedTiming):
//   - vuhive.http.req_connecting (Duration): TCP connection establishment time
//   - vuhive.http.req_tls_handshaking (Duration): TLS handshake time
//   - vuhive.http.req_sending (Duration): request write time
//   - vuhive.http.req_receiving (Duration): response read time
package http
