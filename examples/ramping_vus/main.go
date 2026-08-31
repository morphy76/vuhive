//go:build vuhive_example

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"time"

	"github.com/morphy76/vuhive/pkg/vuhive"
)

// startMockTargetServer starts an in-process HTTP mock server simulating an API backend with request counters.
func startMockTargetServer() *httptest.Server {
	var requestCount int64

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","timestamp":"` + time.Now().UTC().Format(time.RFC3339) + `"}`))
	}))
}

func main() {
	// 1. Launch in-process HTTP mock target server
	ts := startMockTargetServer()
	defer ts.Close()

	// 2. Initialize vuhive suite
	suite := vuhive.NewSuite("Ramping VUs Spike Test Demo Suite")

	// 3. Register scenario configured with ramping_vus pacing in vuhive.yaml
	suite.RegisterScenario("spike_test", vuhive.Scenario{
		Setup: func(ctx vuhive.SetupContext) (map[string]any, error) {
			client := &http.Client{Timeout: 3 * time.Second}
			return map[string]any{
				"client":     client,
				"server_url": ts.URL,
			}, nil
		},

		PreTest: func(ctx vuhive.VUContext) error {
			ctx.Log().Debug().Int64("vu_id", ctx.VUID()).Msg("initializing virtual user for ramping stage")
			return nil
		},

		RunVU: func(ctx vuhive.VUContext) error {
			// Step 1: Extract shared client and server URL
			client := vuhive.MustState[*http.Client](ctx, "client")
			serverURL := vuhive.MustState[string](ctx, "server_url")

			// Step 2: Build HTTP request with context
			start := time.Now()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL, nil)
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}

			// Step 3: Execute request
			resp, err := client.Do(req)
			if err != nil {
				ctx.Metrics().Rate("api_success_rate", vuhive.Tags{}).Add(0, 1)
				return fmt.Errorf("http request failed: %w", err)
			}
			_ = resp.Body.Close()

			// Step 4: Record duration, success rate, and request counters
			ctx.Metrics().Duration("api_response_time", vuhive.Tags{}).Observe(time.Since(start))
			ctx.Metrics().Rate("api_success_rate", vuhive.Tags{}).Add(1, 1)
			ctx.Metrics().Counter("api_requests_total", vuhive.Tags{}).Inc()

			return nil
		},

		AfterTest: func(ctx vuhive.VUContext) error {
			ctx.Log().Debug().Int64("vu_id", ctx.VUID()).Msg("virtual user ramping lifecycle finished")
			return nil
		},
	})

	// 4. Run the suite
	res := suite.Execute()
	if res.Error != nil {
		fmt.Fprintf(os.Stderr, "Execution error: %v\n", res.Error)
	}
	os.Exit(res.ExitCode())
}
