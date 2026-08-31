//go:build vuhive_example

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/morphy76/vuhive/examples/conversation_flow/dsl"
	"github.com/morphy76/vuhive/pkg/vuhive"
)

// Setup initializes global scenario state, starting the mock server if BASE_URL is unset or "mock".
// If a messages_file param is provided, loads user prompts from CSV; otherwise uses built-in defaults.
func Setup(ctx vuhive.SetupContext) (map[string]any, error) {
	baseURL := ctx.Param("base_url")
	var mockServer *httptest.Server

	if baseURL == "" || baseURL == "mock" {
		mockServer = startMockServer()
		baseURL = mockServer.URL
	}

	// Load prompt dataset (from CSV file or fallback defaults)
	var messages []dsl.Message
	if messagesFile := ctx.Param("messages_file"); messagesFile != "" {
		loaded, err := dsl.LoadMessages(messagesFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load messages dataset: %w", err)
		}
		messages = loaded
	}
	if len(messages) == 0 {
		messages = defaultMessages()
	}

	// Initialize HTTP client with connection pooling
	httpClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        1000,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
			DisableKeepAlives:   false,
		},
		Timeout: 30 * time.Second,
	}
	client := dsl.NewConversationClient(baseURL, ctx.Param("token"), ctx.Param("tenant"), httpClient)

	return map[string]any{
		"server_url":  baseURL,
		"mock_server": mockServer,
		"messages":    messages,
		"client":      client,
	}, nil
}

// PreTest executes per-VU initialization before iterations start.
func PreTest(ctx vuhive.VUContext) error {
	ctx.Log().Debug().Int64("vu", ctx.VUID()).Msg("initiating conversation session")
	return nil
}

// RunVU executes the multi-turn conversational AI load iteration for a single virtual user.
// It delegates to the event-driven ConversationFlow which mirrors an SSE callback architecture.
func RunVU(ctx vuhive.VUContext) error {
	client := vuhive.MustState[*dsl.ConversationClient](ctx, "client")
	messages := vuhive.MustState[[]dsl.Message](ctx, "messages")

	dialogModel := ctx.Param("dialog_model")
	if dialogModel == "" {
		dialogModel = "gpt-4o"
	}

	flow := dsl.NewConversationFlow(client, dsl.FlowConfig{
		DialogModel:      dialogModel,
		Turns:            ctx.ParamInt("turns", 2),
		InteractionDelay: ctx.ParamDuration("interaction_delay", 0),
		SSEEventTimeout:  ctx.ParamDuration("sse_event_timeout", 5*time.Second),
		Messages:         messages,
	})

	return flow.Run(ctx)
}

// AfterTest executes per-VU cleanup after iterations complete.
func AfterTest(ctx vuhive.VUContext) error {
	ctx.Log().Debug().Int64("vu", ctx.VUID()).Msg("completed conversation session")
	return nil
}

// Teardown cleans up global resources created in Setup.
func Teardown(ctx vuhive.TeardownContext, state map[string]any) error {
	if mockServer, ok := state["mock_server"].(*httptest.Server); ok && mockServer != nil {
		mockServer.Close()
	}
	return nil
}

// defaultMessages returns the built-in prompt set used when no external CSV is configured.
func defaultMessages() []dsl.Message {
	return []dsl.Message{
		{ID: "1", Text: "Hello, what services do you offer?", Category: "general", ExpectedTokens: 15},
		{ID: "2", Text: "Can you help me with pricing details?", Category: "pricing", ExpectedTokens: 20},
		{ID: "3", Text: "Thank you, goodbye!", Category: "closing", ExpectedTokens: 10},
	}
}

// --- In-Process Mock Server Harness ---

type sseChannel struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// startMockServer creates an in-process mock server replicating the conversational AI SSE protocol.
func startMockServer() *httptest.Server {
	var mu sync.Mutex
	channels := make(map[string]*sseChannel)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if strings.HasPrefix(path, "/api/v1/conversation/") {
			// GET /api/v1/conversation/:external_id -> Open SSE stream & send created event
			parts := strings.Split(path, "/")
			externalID := parts[len(parts)-1]
			dialogID := "dlg-" + externalID

			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
				return
			}

			mu.Lock()
			ch := &sseChannel{w: w, flusher: flusher}
			channels[dialogID] = ch
			mu.Unlock()

			// Send lifecycle created event
			createdEvent := map[string]any{
				"lifecycle": map[string]string{
					"event":     "created",
					"dialog_id": dialogID,
				},
			}
			data, _ := json.Marshal(createdEvent)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

			// Keep connection open until context cancelled
			<-r.Context().Done()

			mu.Lock()
			delete(channels, dialogID)
			mu.Unlock()
			return
		}

		if strings.HasPrefix(path, "/api/v1/message/") {
			// POST /api/v1/message/:dialog_id -> Add customer message & emit bot response via SSE
			parts := strings.Split(path, "/")
			dialogID := parts[len(parts)-1]

			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)

			mu.Lock()
			ch, exists := channels[dialogID]
			mu.Unlock()

			if exists {
				// Echo customer messageAdded event
				custEvent := map[string]any{
					"message": map[string]string{
						"event": "messageAdded",
						"role":  "CUSTOMER",
						"text":  body["text"],
					},
				}
				cData, _ := json.Marshal(custEvent)
				_, _ = fmt.Fprintf(ch.w, "data: %s\n\n", cData)
				ch.flusher.Flush()
			}

			w.WriteHeader(http.StatusOK)

			// Emit bot response event asynchronously over the open SSE connection
			go func() {
				time.Sleep(10 * time.Millisecond) // Simulated LLM inference delay

				mu.Lock()
				ch, exists := channels[dialogID]
				mu.Unlock()

				if exists {
					botEvent := map[string]any{
						"message": map[string]string{
							"event": "messageAdded",
							"role":  "BOT",
							"text":  "Response to: " + body["text"],
						},
					}
					bData, _ := json.Marshal(botEvent)
					_, _ = fmt.Fprintf(ch.w, "data: %s\n\n", bData)
					ch.flusher.Flush()
				}
			}()

			return
		}

		if strings.HasPrefix(path, "/api/v1/close/") {
			// DELETE /api/v1/close/:external_id/:dialog_id -> Close dialog
			w.WriteHeader(http.StatusOK)
			return
		}

		http.NotFound(w, r)
	}))
}
