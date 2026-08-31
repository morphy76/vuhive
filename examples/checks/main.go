//go:build vuhive_example

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/morphy76/vuhive/pkg/vuhive"
)

type apiResponse struct {
	Status  string `json:"status"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// startMockAPIServer starts an in-process HTTP server simulating an API backend with JSON responses.
func startMockAPIServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success","code":200,"message":"operation completed"}`))
	}))
}

func main() {
	// 1. Launch in-process API server
	ts := startMockAPIServer()
	defer ts.Close()

	// 2. Initialize vuhive suite
	suite := vuhive.NewSuite("Checks Demo Suite")

	// 3. Register scenario demonstrating inline assertions (checks)
	suite.RegisterScenario("checks_demo", vuhive.Scenario{
		Setup: func(ctx vuhive.SetupContext) (map[string]any, error) {
			client := &http.Client{Timeout: 2 * time.Second}
			return map[string]any{
				"client":     client,
				"server_url": ts.URL,
			}, nil
		},

		RunVU: func(ctx vuhive.VUContext) error {
			// Step 1: Extract client and server URL from global state
			client := vuhive.MustState[*http.Client](ctx, "client")
			serverURL := vuhive.MustState[string](ctx, "server_url")

			// Step 2: Execute HTTP request
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/api/resource", nil)
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}

			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("http request failed: %w", err)
			}
			defer resp.Body.Close()

			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("failed to read response body: %w", err)
			}

			// Step 3: Perform inline assertions using ctx.Check()
			//
			// Check contract:
			// - Return "" (empty string) to indicate the check PASSED.
			// - Return a descriptive failure reason string to indicate the check FAILED.
			// - Unlike returning an error from RunVU, failed checks DO NOT abort the iteration.
			// - vuhive automatically records vuhive.checks.passed and vuhive.checks.failed metrics
			//   tagged with the check name, and prints a dedicated CHECKS table in the report.

			// Check 1: Validate HTTP Status Code is 200
			ctx.Check("status code is 200", func() string {
				if resp.StatusCode != http.StatusOK {
					return fmt.Sprintf("expected HTTP 200, got %d", resp.StatusCode)
				}
				return ""
			})

			// Check 2: Validate Content-Type header is JSON
			ctx.Check("content-type is json", func() string {
				ct := resp.Header.Get("Content-Type")
				if !strings.Contains(ct, "application/json") {
					return fmt.Sprintf("expected application/json, got %q", ct)
				}
				return ""
			})

			// Check 3: Validate JSON payload structure and status field value
			var res apiResponse
			if err := json.Unmarshal(bodyBytes, &res); err != nil {
				ctx.Check("response body is valid json", func() string {
					return fmt.Sprintf("invalid json payload: %v", err)
				})
			} else {
				ctx.Check("response status is success", func() string {
					if res.Status != "success" {
						return fmt.Sprintf("expected status 'success', got %q", res.Status)
					}
					return ""
				})
			}

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
