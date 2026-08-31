//go:build vuhive_example

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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

			// Step 3: Perform inline assertions using declarative helpers and context methods
			//
			// Check mechanisms:
			// - Direct context methods: ctx.CheckEqual, ctx.CheckTrue, ctx.CheckNoError
			// - Composable assertion generators: vuhive.Equal, vuhive.True, vuhive.NoError, vuhive.Contains, vuhive.InRange
			// - Custom closures: func() string returning "" on pass or failure reason on fail
			// - Failed checks DO NOT abort the iteration.
			// - vuhive automatically records vuhive.checks.passed and vuhive.checks.failed metrics
			//   tagged with the check name, and prints a dedicated CHECKS table in the report.

			// Check 1: Validate HTTP Status Code is 200 using direct context helper
			ctx.CheckEqual("status code is 200", resp.StatusCode, http.StatusOK)

			// Check 2: Validate Content-Type header using composable assertion generator
			ctx.Check("content-type is json", vuhive.Contains(resp.Header.Get("Content-Type"), "application/json"))

			// Check 3: Validate JSON payload unmarshaling without error
			var res apiResponse
			unmarshalErr := json.Unmarshal(bodyBytes, &res)
			ctx.CheckNoError("response body is valid json", unmarshalErr)

			// Check 4: Validate JSON status field value using direct context helper
			ctx.CheckEqual("response status is success", res.Status, "success")

			// Check 5: Validate message is non-empty using direct boolean assertion
			ctx.CheckTrue("response message is present", len(res.Message) > 0)

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
