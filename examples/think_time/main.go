//go:build vuhive_example

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"github.com/morphy76/vuhive/pkg/vuhive"
)

// startMockECommerceServer starts an in-process HTTP mock server simulating catalog, cart, and checkout endpoints.
func startMockECommerceServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/catalog":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"products":[{"id":"p1","name":"Wireless Headphones","price":99.99}]}`))
		case "/cart/add":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"item_added","cart_id":"cart-101"}`))
		case "/checkout":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"confirmed","order_id":"ord-999"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		}
	}))
}

func main() {
	// 1. Start mock e-commerce backend
	ts := startMockECommerceServer()
	defer ts.Close()

	// 2. Initialize vuhive suite
	suite := vuhive.NewSuite("Thinking Time & User Delay Demo Suite")

	// 3. Register multi-step scenario with thinking time pauses
	suite.RegisterScenario("user_journey_with_think_time", vuhive.Scenario{
		Setup: func(ctx vuhive.SetupContext) (map[string]any, error) {
			client := &http.Client{Timeout: 3 * time.Second}

			// Pedagogical Note:
			// Initialize custom mathematical delay generators (e.g. ExpoDelay, GaussianDelay, RangeDelay)
			// once during Setup so that distribution generators can be reused efficiently across VUs.
			expoGen := vuhive.ExpoDelay(25*time.Millisecond, 10*time.Millisecond, 50*time.Millisecond)

			return map[string]any{
				"client":     client,
				"server_url": ts.URL,
				"expo_delay": expoGen,
			}, nil
		},

		PreTest: func(ctx vuhive.VUContext) error {
			ctx.Log().Debug().Int64("vu", ctx.VUID()).Msg("initiating user journey iteration")
			return nil
		},

		RunVU: func(ctx vuhive.VUContext) error {
			client := vuhive.MustState[*http.Client](ctx, "client")
			serverURL := vuhive.MustState[string](ctx, "server_url")
			expoGen := vuhive.MustState[vuhive.DelayGenerator](ctx, "expo_delay")

			// Step 1: Browse catalog
			startCatalog := time.Now()
			req1, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/catalog", nil)
			if err != nil {
				return fmt.Errorf("failed to create catalog request: %w", err)
			}
			resp1, err := client.Do(req1)
			if err != nil {
				ctx.Metrics().Rate("user_flow_success_rate", vuhive.Tags{}).Add(0, 1)
				return fmt.Errorf("catalog request failed: %w", err)
			}
			_ = resp1.Body.Close()
			ctx.Metrics().Duration("catalog_view_duration", vuhive.Tags{}).Observe(time.Since(startCatalog))

			// Pause 1: Declarative thinking time configured in vuhive.yaml
			// Calling ctx.Sleep() with NO arguments automatically evaluates the scenario's
			// configured interaction_delay strategy (e.g. range, fixed, expo, gaussian).
			// It actively respects ctx.Done() for instantaneous cancellation during shutdown.
			thinkStart1 := time.Now()
			if err := ctx.Sleep(); err != nil {
				return fmt.Errorf("think time aborted: %w", err)
			}
			ctx.Metrics().Duration("think_time_catalog", vuhive.Tags{}).Observe(time.Since(thinkStart1))

			// Step 2: Add item to cart
			startCart := time.Now()
			req2, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/cart/add", nil)
			if err != nil {
				return fmt.Errorf("failed to build cart request: %w", err)
			}
			resp2, err := client.Do(req2)
			if err != nil {
				ctx.Metrics().Rate("user_flow_success_rate", vuhive.Tags{}).Add(0, 1)
				return fmt.Errorf("add-to-cart request failed: %w", err)
			}
			_ = resp2.Body.Close()
			ctx.Metrics().Duration("add_to_cart_duration", vuhive.Tags{}).Observe(time.Since(startCart))

			// Pause 2: Programmatic pause using exponential delay generator
			// Calling ctx.Sleep(duration) pauses for an explicit duration, still respecting ctx.Done().
			customPause := expoGen.Next()
			if err := ctx.Sleep(customPause); err != nil {
				return fmt.Errorf("custom pause aborted: %w", err)
			}

			// Step 3: Checkout order
			startCheckout := time.Now()
			req3, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/checkout", nil)
			if err != nil {
				return fmt.Errorf("failed to build checkout request: %w", err)
			}
			resp3, err := client.Do(req3)
			if err != nil {
				ctx.Metrics().Rate("user_flow_success_rate", vuhive.Tags{}).Add(0, 1)
				return fmt.Errorf("checkout request failed: %w", err)
			}
			_ = resp3.Body.Close()
			ctx.Metrics().Duration("checkout_duration", vuhive.Tags{}).Observe(time.Since(startCheckout))

			// Record overall journey metrics
			ctx.Metrics().Rate("user_flow_success_rate", vuhive.Tags{}).Add(1, 1)
			ctx.Metrics().Counter("user_journeys_completed_total", vuhive.Tags{}).Inc()

			return nil
		},

		AfterTest: func(ctx vuhive.VUContext) error {
			ctx.Log().Debug().Int64("vu", ctx.VUID()).Msg("completed user journey iteration")
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
