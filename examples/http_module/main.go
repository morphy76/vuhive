//go:build vuhive_example

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/morphy76/vuhive/pkg/vuhive"
	vuhivehttp "github.com/morphy76/vuhive/pkg/vuhive/http"
)

// --- Mock Backend (Test Infrastructure) ---
// In production load tests, this would be replaced by your actual target system URL
// configured via ctx.Param("base_url") in vuhive.yaml.

type checkoutResponse struct {
	Status  string `json:"status"`
	OrderID string `json:"order_id"`
}

func startMockCheckoutServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond) // Simulate backend processing
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","order_id":"ord-1001"}`))
	}))
}

// --- Load Test Scenario ---

func main() {
	// 1. Start mock backend
	ts := startMockCheckoutServer()
	defer ts.Close()

	// 2. Execute single scenario using the streamlined vuhive.Run shorthand with lifecycle options
	vuhive.Run("http_module_demo",
		// RunVU: execute HTTP requests with automatic instrumentation
		func(ctx vuhive.VUContext) error {
			// Step 1: Retrieve scenario's default instrumented HTTP client (zero Setup boilerplate)
			client := vuhivehttp.Default(ctx)
			serverURL := vuhive.MustState[string](ctx, "server_url")

			checkoutPath := ctx.Param("checkout_path")
			if checkoutPath == "" {
				checkoutPath = "/checkout"
			}

			// Step 2: Execute request — metrics (duration, counter, failed rate) recorded automatically
			resp, err := client.Get(ctx, serverURL+checkoutPath)
			if err != nil {
				return fmt.Errorf("checkout request failed: %w", err)
			}

			// Step 3: Validate response using inline checks
			ctx.Check("status_200", func() string {
				if resp.StatusCode != http.StatusOK {
					return fmt.Sprintf("expected 200, got %d", resp.StatusCode)
				}
				return ""
			})

			// Step 4: Parse JSON response body
			var result checkoutResponse
			if err := resp.JSON(&result); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			ctx.Check("order_created", func() string {
				if result.OrderID == "" {
					return "missing order_id in response"
				}
				return ""
			})

			return nil
		},
		// Setup: optional scenario lifecycle hook (e.g. sharing dynamic runtime state)
		vuhive.WithSetup(func(ctx vuhive.SetupContext) (map[string]any, error) {
			return map[string]any{
				"server_url": ts.URL,
			}, nil
		}),
		// Teardown: optional scenario lifecycle hook executed after all VUs exit
		vuhive.WithTeardown(func(ctx vuhive.TeardownContext, _ map[string]any) error {
			ctx.Log().Info().Msg("HTTP module demo completed")
			return nil
		}),
	)
}
