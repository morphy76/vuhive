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

// startMockECommerceServer starts an in-process HTTP mock server simulating
// an e-commerce API with authentication, catalog browsing, cart, and payment endpoints.
func startMockECommerceServer() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","token":"sess-tok-1234"}`))
	})

	mux.HandleFunc("/api/products", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"items":[{"id":"prod-1","name":"mechanical keyboard"}]}`))
	})

	mux.HandleFunc("/api/cart", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(4 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","cart_id":"cart-987"}`))
	})

	mux.HandleFunc("/api/payment", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(8 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","transaction_id":"tx-5544"}`))
	})

	return httptest.NewServer(mux)
}

func main() {
	ts := startMockECommerceServer()
	defer ts.Close()

	suite := vuhive.NewSuite("E-Commerce Multi-Step Journey Suite")

	suite.RegisterScenario("ecommerce_flow", vuhive.Scenario{
		Setup: func(ctx vuhive.SetupContext) (map[string]any, error) {
			client := &http.Client{
				Timeout: 2 * time.Second,
			}
			return map[string]any{
				"client":     client,
				"server_url": ts.URL,
			}, nil
		},

		RunVU: func(ctx vuhive.VUContext) error {
			client := vuhive.MustState[*http.Client](ctx, "client")
			serverURL := vuhive.MustState[string](ctx, "server_url")

			// Step 1: Top-level transaction group for user authentication
			err := ctx.Group("01_Login", func(ctx vuhive.VUContext) error {
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/api/login", nil)
				if err != nil {
					return err
				}
				resp, err := client.Do(req)
				if err != nil {
					return err
				}
				defer resp.Body.Close()

				ctx.Check("login status 200", func() string {
					if resp.StatusCode != http.StatusOK {
						return fmt.Sprintf("expected 200, got %d", resp.StatusCode)
					}
					return ""
				})
				return nil
			})
			if err != nil {
				return fmt.Errorf("login failed: %w", err)
			}

			_ = ctx.Sleep(2 * time.Millisecond) // Inter-step pacing / think time

			// Step 2: Top-level transaction group for catalog browsing
			err = ctx.Group("02_Browse_Catalog", func(ctx vuhive.VUContext) error {
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/api/products", nil)
				if err != nil {
					return err
				}
				resp, err := client.Do(req)
				if err != nil {
					return err
				}
				defer resp.Body.Close()

				ctx.Check("browse status 200", func() string {
					if resp.StatusCode != http.StatusOK {
						return fmt.Sprintf("expected 200, got %d", resp.StatusCode)
					}
					return ""
				})
				return nil
			})
			if err != nil {
				return fmt.Errorf("browse failed: %w", err)
			}

			_ = ctx.Sleep(2 * time.Millisecond)

			// Step 3: Top-level transaction group with nested child steps
			err = ctx.Group("03_Checkout", func(ctx vuhive.VUContext) error {
				// Nested Group: Add to Cart
				err := ctx.Group("Add_To_Cart", func(ctx vuhive.VUContext) error {
					req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/api/cart", nil)
					if err != nil {
						return err
					}
					resp, err := client.Do(req)
					if err != nil {
						return err
					}
					defer resp.Body.Close()

					ctx.Check("cart status 200", func() string {
						if resp.StatusCode != http.StatusOK {
							return fmt.Sprintf("expected 200, got %d", resp.StatusCode)
						}
						return ""
					})
					return nil
				})
				if err != nil {
					return err
				}

				_ = ctx.Sleep(2 * time.Millisecond)

				// Nested Group: Submit Payment
				return ctx.Group("Submit_Payment", func(ctx vuhive.VUContext) error {
					req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/api/payment", nil)
					if err != nil {
						return err
					}
					resp, err := client.Do(req)
					if err != nil {
						return err
					}
					defer resp.Body.Close()

					ctx.Check("payment status 200", func() string {
						if resp.StatusCode != http.StatusOK {
							return fmt.Sprintf("expected 200, got %d", resp.StatusCode)
						}
						return ""
					})
					return nil
				})
			})
			if err != nil {
				return fmt.Errorf("checkout failed: %w", err)
			}

			return nil
		},
	})

	res := suite.Execute()
	if res.Error != nil {
		fmt.Fprintf(os.Stderr, "Execution error: %v\n", res.Error)
	}
	os.Exit(res.ExitCode())
}
