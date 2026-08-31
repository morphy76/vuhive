//go:build vuhive_example

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/morphy76/vuhive/pkg/vuhive"
	vuhivehttp "github.com/morphy76/vuhive/pkg/vuhive/http"
)

// --- In-Process Mock LLM Streaming Server ---

type chatRequest struct {
	Prompt string `json:"prompt"`
}

func startMockLLMServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/event-stream" {
			http.Error(w, "expected text/event-stream Accept header", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		tokens := []string{"Hello", " world", "!", " This", " is", " a", " live", " token", " stream", " from", " vuhive."}
		for i, tok := range tokens {
			data, _ := json.Marshal(map[string]any{
				"index": i,
				"token": tok,
			})
			_, _ = fmt.Fprintf(w, "id: %d\nevent: token\ndata: %s\n\n", i+1, data)
			flusher.Flush()
			time.Sleep(5 * time.Millisecond) // Simulated LLM inter-token generation time
		}

		// Send completion event
		_, _ = fmt.Fprintf(w, "event: done\ndata: [DONE]\n\n")
		flusher.Flush()
	}))
}

// --- Load Test Scenario ---

func main() {
	// 1. Start mock LLM streaming server
	ts := startMockLLMServer()
	defer ts.Close()

	// 2. Initialize suite
	suite := vuhive.NewSuite("SSE Streaming Demo Suite")

	// 3. Register SSE streaming scenario
	suite.RegisterScenario("sse_streaming_demo", vuhive.Scenario{
		Setup: func(ctx vuhive.SetupContext) (map[string]any, error) {
			return map[string]any{
				"server_url": ts.URL,
			}, nil
		},

		RunVU: func(ctx vuhive.VUContext) error {
			client := vuhivehttp.Default(ctx)
			serverURL := vuhive.MustState[string](ctx, "server_url")

			streamPath := ctx.Param("stream_path")
			if streamPath == "" {
				streamPath = "/v1/chat/completions/stream"
			}

			// Execute streaming request via POST (or GET via client.StreamSSE)
			reqBody := `{"prompt":"Tell me a story"}`
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+streamPath, strings.NewReader(reqBody))
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}
			req.Header.Set("Content-Type", "application/json")

			stream, err := client.DoStream(ctx, req)
			if err != nil {
				return fmt.Errorf("failed to open SSE stream: %w", err)
			}
			defer func() { _ = stream.Close() }()

			ctx.Check("stream_status_200", func() string {
				if stream.StatusCode != http.StatusOK {
					return fmt.Sprintf("expected 200, got %d", stream.StatusCode)
				}
				return ""
			})

			var tokenCount int
			for stream.Next() {
				event := stream.Event()

				if event.Event == "token" {
					tokenCount++
				}

				if event.Data == "[DONE]" {
					break
				}
			}

			if err := stream.Err(); err != nil {
				return fmt.Errorf("stream read error: %w", err)
			}

			ctx.Check("received_tokens", func() string {
				if tokenCount == 0 {
					return "no tokens received from SSE stream"
				}
				return ""
			})

			return nil
		},

		Teardown: func(ctx vuhive.TeardownContext, _ map[string]any) error {
			ctx.Log().Info().Msg("SSE streaming demo completed")
			return nil
		},
	})

	// 4. Execute and exit
	res := suite.Execute()
	if res.Error != nil {
		fmt.Fprintf(os.Stderr, "Execution error: %v\n", res.Error)
	}
	os.Exit(res.ExitCode())
}
