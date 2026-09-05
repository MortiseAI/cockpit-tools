package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	providerexecutor "github.com/router-for-me/CLIProxyAPI/v7/internal/runtime/executor"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestCodexFastServiceTierReachesUpstream(t *testing.T) {
	for _, format := range []sdktranslator.Format{sdktranslator.FormatOpenAI, sdktranslator.FormatOpenAIResponse} {
		for _, stream := range []bool{false, true} {
			for _, tc := range []struct {
				name, tier  string
				defaultFast bool
			}{
				{"gateway_default", "", true},
				{"client_priority", "priority", false},
				{"client_fast_alias", "fast", false},
			} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", format, tc.name, stream), func(t *testing.T) {
					captured := make(chan []byte, 1)
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						body, _ := io.ReadAll(r.Body)
						captured <- body
						w.Header().Set("Content-Type", "text/event-stream")
						fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_test\",\"model\":\"gpt-6-astra\",\"status\":\"completed\",\"service_tier\":\"priority\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n")
					}))
					defer upstream.Close()
					configJSON := `{}`
					if tc.defaultFast {
						configJSON = `{"payload":{"default":[{"models":[{"name":"*","protocol":"codex"},{"name":"*","protocol":"openai"},{"name":"*","protocol":"openai-response"}],"params":{"service_tier":"priority"}}]}}`
					}
					path := filepath.Join(t.TempDir(), "config.json")
					if err := os.WriteFile(path, []byte(configJSON), 0600); err != nil {
						t.Fatal(err)
					}
					cfg, err := config.LoadConfig(path)
					if err != nil {
						t.Fatal(err)
					}
					executor := providerexecutor.NewCodexExecutor(cfg)
					auth := &coreauth.Auth{Attributes: map[string]string{"base_url": upstream.URL}, Metadata: map[string]any{"access_token": "test-token"}}
					payload := fmt.Sprintf(`{"model":"gpt-6-astra","stream":%t`, stream)
					if format == sdktranslator.FormatOpenAI {
						payload += `,"messages":[{"role":"user","content":"hello"}]`
					} else {
						payload += `,"input":"hello"`
					}
					if tc.tier != "" {
						payload += fmt.Sprintf(`,"service_tier":%q`, tc.tier)
					}
					payload += `}`
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					request, options := buildExecutorRequest(nil, []byte(payload), "gpt-6-astra", format, "", stream)
					if stream {
						result, err := executor.ExecuteStream(ctx, auth, request, options)
						if err != nil {
							t.Fatal(err)
						}
						for chunk := range result.Chunks {
							if chunk.Err != nil {
								t.Fatal(chunk.Err)
							}
						}
					} else {
						if _, err := executor.Execute(ctx, auth, request, options); err != nil {
							t.Fatal(err)
						}
					}
					body := <-captured
					if got := gjson.GetBytes(body, "service_tier").String(); got != "priority" {
						t.Fatalf("upstream service_tier = %q, want priority; body=%s", got, body)
					}
				})
			}
		}
	}
}

func TestRelayRecordsRequestedServiceTier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct{ body, want string }{
		{``, "auto"}, {`,"service_tier":"priority"`, "priority"},
		{`,"service_tier":"fast"`, "fast"}, {`,"service_tier":"default"`, "default"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			runtime := &fakeRuntime{response: cliproxyexecutor.Response{Payload: []byte(`{"ok":true}`)}}
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-6-astra","messages":[{"role":"user","content":"hello"}]`+tc.body+`}`))
			request.Header.Set("Authorization", "Bearer client-key")
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			apiKey := apiKeySpec{ID: "key_1", Key: "client-key", Enabled: true}
			m := &manifest{
				APIKeys: []apiKeySpec{apiKey}, ModelIDs: []string{"gpt-6-astra"},
				apiKeyByValue: map[string]*apiKeySpec{"client-key": &apiKey},
			}
			router := (&relayServer{runtime: runtime, cfg: &config.Config{}, manifest: m, policy: &requestPolicy{manifest: m}}).router()
			router.ServeHTTP(response, request)
			if response.Code != 200 {
				t.Fatalf("status=%d body=%s", response.Code, response.Body)
			}
			if got := runtime.lastOpts.Metadata[cliproxyexecutor.ServiceTierMetadataKey]; got != tc.want {
				t.Fatalf("service tier metadata=%v, want %s", got, tc.want)
			}
		})
	}
}
