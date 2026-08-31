//go:build vuhive_example

package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/morphy76/vuhive/pkg/vuhive"
)

// mockUserStore simulates a backend user database/service state.
type mockUserStore struct {
	users map[int]string
}

func newMockUserStore() *mockUserStore {
	return &mockUserStore{
		users: map[int]string{
			1: "Alice",
			2: "Bob",
			3: "Charlie",
		},
	}
}

func main() {
	// 1. Initialize vuhive suite
	suite := vuhive.NewSuite("gRPC User Service Load Test")

	// 2. Register scenario configured with open-system arrival_rate pacing
	suite.RegisterScenario("grpc_user_service_flow", vuhive.Scenario{
		// Setup runs ONCE before any workers or token dispatches start.
		// Pedagogical Note:
		// When load testing a real gRPC service, initialize your *grpc.ClientConn or connection
		// pool (e.g. grpc.Dial / grpc.NewClient) and generated protobuf client stubs (pb.NewUserServiceClient)
		// here, storing them in the returned global state map.
		Setup: func(ctx vuhive.SetupContext) (map[string]any, error) {
			store := newMockUserStore()
			return map[string]any{
				"user_store": store,
			}, nil
		},

		PreTest: func(ctx vuhive.VUContext) error {
			ctx.Log().Debug().Msg("preparing RPC invocation")
			return nil
		},

		// RunVU is invoked by worker goroutines upon arrival of each rate-limited token.
		RunVU: func(ctx vuhive.VUContext) error {
			store := vuhive.MustState[*mockUserStore](ctx, "user_store")

			// Step 1: Read RPC service and method parameters from YAML configuration
			serviceName := ctx.Param("service_name")
			method := ctx.Param("method")

			// Step 2: Simulate RPC execution and network latency
			start := time.Now()
			userID := rand.Intn(3) + 1
			userName, found := store.users[userID]

			// Simulated RPC wire latency (2ms - 7ms)
			latency := time.Duration(2+rand.Intn(5)) * time.Millisecond
			time.Sleep(latency)
			elapsed := time.Since(start)

			// Step 3: Record gRPC latency duration histogram tagged with service and method dimensions
			ctx.Metrics().Duration("grpc_latency", vuhive.Tags{
				"service": serviceName,
				"method":  method,
			}).Observe(elapsed)

			// Step 4: Validate RPC outcome and record success rate and call counter
			if !found {
				ctx.Metrics().Rate("rpc_success_rate", vuhive.Tags{}).Add(0, 1)
				return fmt.Errorf("user %d not found", userID)
			}

			_ = userName
			ctx.Metrics().Rate("rpc_success_rate", vuhive.Tags{}).Add(1, 1)
			ctx.Metrics().Counter("grpc_calls_total", vuhive.Tags{"status": "OK"}).Inc()
			return nil
		},

		AfterTest: func(ctx vuhive.VUContext) error {
			ctx.Log().Debug().Msg("completed RPC invocation")
			return nil
		},

		Teardown: func(ctx vuhive.TeardownContext, state map[string]any) error {
			// In real gRPC load tests, close the gRPC connection pool here (e.g. conn.Close())
			return nil
		},
	})

	// 3. Execute the suite
	res := suite.Execute()
	if res.Error != nil {
		fmt.Fprintf(os.Stderr, "Execution error: %v\n", res.Error)
	}
	os.Exit(res.ExitCode())
}
