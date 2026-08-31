//go:build vuhive_example

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"github.com/morphy76/vuhive/pkg/vuhive"
	vuhivehttp "github.com/morphy76/vuhive/pkg/vuhive/http"
)

// startMockCheckoutServer launches an in-process HTTP test server simulating a checkout backend API.
// In real-world load testing, your Setup hook targets external staging or production URLs configured
// declaratively via ctx.Param("base_url") rather than an in-process mock server.
func startMockCheckoutServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond) // Simulate lightweight backend processing latency
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","order_id":"ord-1001"}`))
	}))
}

func main() {
	// 1. Start the target mock backend server
	ts := startMockCheckoutServer()
	defer ts.Close()

	// 2. Initialize the vuhive test suite
	suite := vuhive.NewSuite("HTTP Checkout Flow Suite")

	// 3. Register the scenario with lifecycle hooks
	suite.RegisterScenario("http_checkout_flow", vuhive.Scenario{
		// Setup runs ONCE per scenario execution before any Virtual Users (VUs) are spawned.
		// Best practice: Initialize shared reusable resources (HTTP clients with connection pooling,
		// auth tokens, datasets) here and return them in the state map.
		Setup: func(ctx vuhive.SetupContext) (map[string]any, error) {
			// Configure a standard HTTP client and wrap it with vuhive telemetry instrumentation
			baseClient := &http.Client{
				Timeout: 2 * time.Second,
			}
			client := vuhivehttp.Instrument(baseClient)
			return map[string]any{
				"client":     client,
				"server_url": ts.URL,
			}, nil
		},

		// PreTest runs ONCE per VU before its iteration loop begins.
		// Useful for per-VU initializations, user session logins, or logging VU start events.
		PreTest: func(ctx vuhive.VUContext) error {
			ctx.Log().Debug().Int64("vu", ctx.VUID()).Msg("preparing checkout iteration loop")
			return nil
		},

		// RunVU is invoked repeatedly by each Virtual User for every load iteration during run_period.
		RunVU: func(ctx vuhive.VUContext) error {
			// Step 1: Retrieve shared instrumented client and configuration from GlobalState and YAML params
			client := ctx.GlobalState("client").(*http.Client)
			serverURL := ctx.GlobalState("server_url").(string)

			checkoutPath := ctx.Param("checkout_path")
			if checkoutPath == "" {
				checkoutPath = "/checkout"
			}

			// Step 2: Construct the HTTP request with VU context to respect iteration timeouts (vu_timeout)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+checkoutPath, nil)
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}

			// Step 3: Execute the HTTP request — metrics (req_duration, reqs, req_failed)
			// are automatically recorded to vuhive telemetry via the instrumented transport
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("http request failed: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("http request failed with status %d", resp.StatusCode)
			}
			return nil
		},

		// AfterTest runs ONCE per VU when the iteration loop finishes (guaranteed via defer).
		AfterTest: func(ctx vuhive.VUContext) error {
			ctx.Log().Debug().Int64("vu", ctx.VUID()).Msg("completed checkout iteration loop")
			return nil
		},

		// Teardown runs ONCE after ALL Virtual Users have finished and exited.
		// Use it to clean up global test fixtures, close persistent database connections, or flush logs.
		Teardown: func(ctx vuhive.TeardownContext, state map[string]any) error {
			ctx.Log().Info().Msg("cleaning up checkout test fixtures")
			return nil
		},
	})

	// 4. Execute the suite and terminate with the appropriate exit code (0 for pass, 1 for SLA failure)
	res := suite.Execute()
	if res.Error != nil {
		fmt.Fprintf(os.Stderr, "Execution error: %v\n", res.Error)
	}
	os.Exit(res.ExitCode())
}
