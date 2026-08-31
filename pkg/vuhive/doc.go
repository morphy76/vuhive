// Package vuhive provides the public developer API and load test execution framework.
//
// vuhive is an enterprise-grade Go load testing framework featuring:
//   - Streamlined single-scenario execution via vuhive.Run with functional options.
//   - Declarative YAML configuration (VU count, pacing, ramp-up, ramp-down, SLA thresholds).
//   - Scenario lifecycle hooks (Setup, PreTest, RunVU, AfterTest, Teardown, HandleSummary).
//   - High-performance, lock-free telemetry (Counters, Gauges, Rates, and HDR Histograms).
//   - Non-blocking think time generators (fixed, uniform range, exponential, gaussian).
//   - Clean-architecture execution model with structured summary reports and JSON export.
//
// Quick Start (Single Scenario):
//
//	package main
//
//	import (
//		"github.com/morphy76/vuhive/pkg/vuhive"
//		vuhivehttp "github.com/morphy76/vuhive/pkg/vuhive/http"
//	)
//
//	func main() {
//		vuhive.Run("checkout_flow", func(ctx vuhive.VUContext) error {
//			_, err := vuhivehttp.Default(ctx).Get(ctx, "/checkout")
//			return err
//		})
//	}
//
// Suite Usage (Multi-Scenario):
//
//	package main
//
//	import (
//		"net/http"
//		"time"
//
//		"github.com/morphy76/vuhive/pkg/vuhive"
//	)
//
//	func main() {
//		suite := vuhive.NewSuite("E-Commerce Load Tests")
//
//		suite.RegisterScenario("checkout_flow", vuhive.Scenario{
//			RunVU: func(ctx vuhive.VUContext) error {
//				start := time.Now()
//				resp, err := http.Get("https://api.example.com/checkout")
//				if err != nil {
//					return err
//				}
//				defer resp.Body.Close()
//
//				ctx.Metrics().Duration("checkout_latency", nil).Observe(time.Since(start))
//				ctx.Check("status_200", func() string {
//					if resp.StatusCode != http.StatusOK {
//						return "non-200 status code"
//					}
//					return ""
//				})
//				return nil
//			},
//		})
//
//		res := suite.Execute()
//		if res.ExitCode() != 0 {
//			// Handle failure or let main return non-zero exit code
//		}
//	}
package vuhive
