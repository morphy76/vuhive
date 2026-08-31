//go:build vuhive_example

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/morphy76/vuhive/pkg/vuhive"
	"github.com/morphy76/vuhive/pkg/vuhive/data"
)

// Sample CSV dataset: user credentials and roles
const sampleCSV = `username,user_id,role
alice,u101,admin
bob,u102,user
charlie,u103,user
david,u104,tester
`

// Sample JSON array dataset: product inventory catalog
const sampleJSON = `[
  {"sku": "SKU-A100", "category": "electronics", "price": "299.99"},
  {"sku": "SKU-B200", "category": "books", "price": "19.99"},
  {"sku": "SKU-C300", "category": "apparel", "price": "49.50"},
  {"sku": "SKU-D400", "category": "home", "price": "89.00"}
]`

// Sample JSONL (newline-delimited JSON) dataset: coupon codes and discounts
const sampleJSONL = `{"code": "SUMMER10", "discount_pct": "10", "max_uses": "100"}
{"code": "VIP25", "discount_pct": "25", "max_uses": "50"}
{"code": "FLASH50", "discount_pct": "50", "max_uses": "10"}
{"code": "WELCOME5", "discount_pct": "5", "max_uses": "500"}
`

// startMockDataServer creates an in-process HTTP mock server simulating user, product, and coupon lookup endpoints.
func startMockDataServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user":
			userID := r.URL.Query().Get("user_id")
			if userID == "" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"missing user_id"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, `{"status":"ok","user_id":"%s"}`, userID)

		case "/api/product":
			sku := r.URL.Query().Get("sku")
			if sku == "" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"missing sku"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, `{"status":"ok","sku":"%s"}`, sku)

		case "/api/coupon":
			code := r.URL.Query().Get("code")
			if code == "" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"missing coupon code"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, `{"status":"ok","code":"%s"}`, code)

		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		}
	}))
}

func main() {
	// 1. Launch in-process API server
	ts := startMockDataServer()
	defer ts.Close()

	// 2. Initialize vuhive suite
	suite := vuhive.NewSuite("Data Parameterization Demo Suite")

	// 3. Register scenario demonstrating dataset parameterization
	suite.RegisterScenario("data_parameterization_flow", vuhive.Scenario{
		Setup: func(ctx vuhive.SetupContext) (map[string]any, error) {
			// Pedagogical Note:
			// Load datasets once in Setup. The pkg/vuhive/data module provides thread-safe
			// dataset distribution strategies:
			//
			// 1. data.Sequential: Deterministic round-robin per VU + iteration index ((vu-1+iter)%N).
			// 2. data.Random: Lock-free, uniform random selection across rows.
			// 3. data.SharedQueue: Atomic single-consumption FIFO queue across concurrent VUs.

			// 1. Load CSV dataset with Sequential strategy
			csvDS, err := data.LoadCSV(strings.NewReader(sampleCSV), data.Sequential)
			if err != nil {
				return nil, fmt.Errorf("failed to load CSV dataset: %w", err)
			}

			// 2. Load JSON dataset with Random strategy
			jsonDS, err := data.LoadJSON(strings.NewReader(sampleJSON), data.Random)
			if err != nil {
				return nil, fmt.Errorf("failed to load JSON dataset: %w", err)
			}

			// 3. Load JSONL dataset with SharedQueue strategy
			jsonlDS, err := data.LoadJSONL(strings.NewReader(sampleJSONL), data.SharedQueue)
			if err != nil {
				return nil, fmt.Errorf("failed to load JSONL dataset: %w", err)
			}

			client := &http.Client{Timeout: 2 * time.Second}
			return map[string]any{
				"csv_dataset":   csvDS,
				"json_dataset":  jsonDS,
				"jsonl_dataset": jsonlDS,
				"client":        client,
				"server_url":    ts.URL,
			}, nil
		},

		PreTest: func(ctx vuhive.VUContext) error {
			ctx.Log().Debug().Int64("vu", ctx.VUID()).Msg("starting parameterized iteration")
			return nil
		},

		RunVU: func(ctx vuhive.VUContext) error {
			csvDS := vuhive.MustState[*data.DataSet](ctx, "csv_dataset")
			jsonDS := vuhive.MustState[*data.DataSet](ctx, "json_dataset")
			client := vuhive.MustState[*http.Client](ctx, "client")
			serverURL := vuhive.MustState[string](ctx, "server_url")

			// Step 1: Ingest next record from CSV (Sequential strategy) and query User API
			userRec, err := csvDS.Next(ctx)
			if err != nil {
				return fmt.Errorf("failed to get CSV record: %w", err)
			}
			userID := userRec["user_id"]
			username := userRec["username"]

			reqUser, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/user?user_id=%s", serverURL, userID), nil)
			respUser, err := client.Do(reqUser)
			if err != nil {
				ctx.Metrics().Rate("dataset_success_rate", vuhive.Tags{}).Add(0, 1)
				return fmt.Errorf("user query failed for %s (%s): %w", username, userID, err)
			}
			_ = respUser.Body.Close()

			ctx.Check("user request status is 200", func() string {
				if respUser.StatusCode != http.StatusOK {
					return fmt.Sprintf("expected 200, got %d", respUser.StatusCode)
				}
				return ""
			})

			// Step 2: Ingest next record from JSON (Random strategy) and query Product API
			productRec, err := jsonDS.Next(ctx)
			if err != nil {
				return fmt.Errorf("failed to get JSON record: %w", err)
			}
			sku := productRec["sku"]

			reqProd, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/product?sku=%s", serverURL, sku), nil)
			respProd, err := client.Do(reqProd)
			if err != nil {
				ctx.Metrics().Rate("dataset_success_rate", vuhive.Tags{}).Add(0, 1)
				return fmt.Errorf("product query failed for %s: %w", sku, err)
			}
			_ = respProd.Body.Close()

			ctx.Check("product request status is 200", func() string {
				if respProd.StatusCode != http.StatusOK {
					return fmt.Sprintf("expected 200, got %d", respProd.StatusCode)
				}
				return ""
			})

			// Step 3: Record aggregated dataset execution metrics
			ctx.Metrics().Rate("dataset_success_rate", vuhive.Tags{}).Add(1, 1)
			ctx.Metrics().Counter("parameterized_requests_total", vuhive.Tags{"format": "csv_and_json"}).Inc()

			return nil
		},

		AfterTest: func(ctx vuhive.VUContext) error {
			ctx.Log().Debug().Int64("vu", ctx.VUID()).Msg("completed parameterized iteration")
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
