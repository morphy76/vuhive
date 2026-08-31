//go:build vuhive_example

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"github.com/morphy76/vuhive/pkg/vuhive"
)

// startMockServiceAndWebhookServer starts an in-process mock server serving both the target API and a webhook endpoint.
func startMockServiceAndWebhookServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/task":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"completed","task_id":"task-42"}`))

		case "/webhook/alerts":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"bad payload"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"notification_received"}`))

		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		}
	}))
}

func main() {
	// 1. Launch in-process mock service & webhook receiver
	ts := startMockServiceAndWebhookServer()
	defer ts.Close()

	// 2. Initialize vuhive suite
	suite := vuhive.NewSuite("Execution Summary Hook Demo Suite")

	// 3. Register scenario with HandleSummary hook
	suite.RegisterScenario("summary_hook_demo", vuhive.Scenario{
		Setup: func(ctx vuhive.SetupContext) (map[string]any, error) {
			client := &http.Client{Timeout: 2 * time.Second}
			return map[string]any{
				"client":      client,
				"server_url":  ts.URL,
				"webhook_url": ts.URL + "/webhook/alerts",
			}, nil
		},

		RunVU: func(ctx vuhive.VUContext) error {
			client := vuhive.MustState[*http.Client](ctx, "client")
			serverURL := vuhive.MustState[string](ctx, "server_url")

			// Step 1: Execute task request and measure latency
			start := time.Now()
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/api/task", nil)
			if err != nil {
				ctx.Metrics().Rate("task_success_rate", vuhive.Tags{}).Add(0, 1)
				return fmt.Errorf("failed to create task request: %w", err)
			}

			resp, err := client.Do(req)
			latency := time.Since(start)
			ctx.Metrics().Duration("task_latency", vuhive.Tags{}).Observe(latency)

			// Step 2: Validate response and record metrics
			if err != nil || resp.StatusCode != http.StatusOK {
				ctx.Metrics().Rate("task_success_rate", vuhive.Tags{}).Add(0, 1)
				if resp != nil {
					_ = resp.Body.Close()
				}
				return fmt.Errorf("task request failed: %v", err)
			}
			_ = resp.Body.Close()

			ctx.Metrics().Rate("task_success_rate", vuhive.Tags{}).Add(1, 1)
			ctx.Metrics().Counter("tasks_completed_total", vuhive.Tags{}).Inc()
			return nil
		},

		// HandleSummary executes ONCE post-test run after all reports (console & JSON) have been generated.
		// It receives the full SummaryData model with metadata, metric aggregates, and threshold outcomes.
		// Use this hook for:
		// - Posting results or alerts to Slack, Discord, MS Teams, or Datadog
		// - Generating custom CSV, Markdown, or HTML test summary artifacts
		// - Triggering downstream CI/CD pipelines
		HandleSummary: func(ctx vuhive.SummaryContext, summary vuhive.SummaryData) error {
			fmt.Println("\n--- [HandleSummary Hook Invoked] ---")
			fmt.Printf("Suite:       %s\n", summary.SuiteName)
			fmt.Printf("Scenario:    %s\n", summary.Scenario)
			fmt.Printf("Duration:    %v\n", summary.Duration)
			fmt.Printf("SLA Verdict: Passed=%v\n", summary.Passed)

			// Step 1: Inspect aggregated metrics
			totalTasks := summary.Counter("tasks_completed_total")
			successRate := summary.Rate("task_success_rate")
			latencyMetric := summary.Metric("task_latency")

			fmt.Printf("Total Tasks: %d | Success Rate: %.2f%%\n", totalTasks, successRate*100)
			if latencyMetric != nil {
				fmt.Printf("Latency p95: %v | Max: %v\n", latencyMetric.P95, latencyMetric.Max)
			}

			// Step 2: Programmatically inspect SLA threshold results
			for _, th := range summary.Thresholds {
				status := "PASS"
				if !th.Passed {
					status = "FAIL"
				}
				fmt.Printf("Threshold [%s]: %s %s %s (actual: %s)\n",
					status, th.Metric, th.Stat, th.Target, th.Actual)
			}

			// Step 3: Post structured results payload to a webhook alert endpoint
			notification := map[string]any{
				"suite":            summary.SuiteName,
				"scenario":         summary.Scenario,
				"passed":           summary.Passed,
				"duration_seconds": summary.Duration.Seconds(),
				"total_tasks":      totalTasks,
				"success_rate":     successRate,
			}

			bodyBytes, err := json.Marshal(notification)
			if err != nil {
				return fmt.Errorf("failed to marshal notification: %w", err)
			}

			webhookReq, err := http.NewRequestWithContext(ctx, http.MethodPost, ts.URL+"/webhook/alerts", bytes.NewReader(bodyBytes))
			if err != nil {
				return fmt.Errorf("failed to prepare webhook request: %w", err)
			}
			webhookReq.Header.Set("Content-Type", "application/json")

			webhookClient := &http.Client{Timeout: 3 * time.Second}
			resp, err := webhookClient.Do(webhookReq)
			if err != nil {
				return fmt.Errorf("failed to dispatch webhook notification: %w", err)
			}
			_ = resp.Body.Close()

			fmt.Println("Successfully delivered notification payload to webhook.")
			fmt.Println("------------------------------------")
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
