// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

// Command mockllm is a fixed-answer LLM backend for E2E tests, so suites that send
// traffic through an LLM provider need no real key. It answers in each provider's
// shape, with fixed token usage (10,000 input of which 2,000 cached, 1,000 output):
//
//   - OpenAI and Azure OpenAI: POST .../chat/completions (streaming too), POST .../responses,
//     POST .../embeddings, GET .../models.
//   - Anthropic: POST .../messages.
//   - Gemini: POST ...:generateContent.
//   - Mistral and other OpenAI-compatible providers use the OpenAI paths.
//
// It accepts any credential. A model name containing "nousage" gets no usage.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	inputTokens  = 10000
	cachedTokens = 2000
	outputTokens = 1000
	reply        = "This is a reply from the mock LLM."
)

func main() {
	addr := ":" + envOr("PORT", "8000")
	log.Printf("mock LLM listening on %s", addr)
	srv := &http.Server{Addr: addr, Handler: http.HandlerFunc(handle), ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

func handle(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if r.Method == http.MethodGet {
		switch {
		case path == "/healthz":
			writeJSON(w, map[string]any{"status": "ok"})
		case strings.HasSuffix(path, "/models"):
			writeJSON(w, map[string]any{"object": "list", "data": []any{
				map[string]any{"id": "gpt-4o", "object": "model", "owned_by": "mock"},
				map[string]any{"id": "gpt-4o-mini", "object": "model", "owned_by": "mock"},
			}})
		default:
			http.NotFound(w, r)
		}
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	model, _ := body["model"].(string)
	if model == "" {
		model = modelFromPath(path)
	}
	usage := !strings.Contains(model, "nousage")
	stream, _ := body["stream"].(bool)
	log.Printf("%s model=%q stream=%v", path, model, stream)

	switch {
	case strings.HasSuffix(path, "/chat/completions"):
		if stream {
			streamChat(w, model, usage)
			return
		}
		writeJSON(w, chatCompletion(model, usage))
	case strings.HasSuffix(path, "/responses"):
		writeJSON(w, response(model, usage))
	case strings.HasSuffix(path, "/embeddings"):
		writeJSON(w, embeddings(model, usage))
	case strings.HasSuffix(path, "/messages"):
		writeJSON(w, anthropicMessage(model, usage))
	case strings.Contains(path, ":generateContent"):
		writeJSON(w, geminiContent(model, usage))
	default:
		http.NotFound(w, r)
	}
}

func chatCompletion(model string, usage bool) map[string]any {
	out := map[string]any{
		"id": "chatcmpl-mock", "object": "chat.completion", "created": time.Now().Unix(), "model": model,
		"choices": []any{map[string]any{
			"index": 0, "finish_reason": "stop",
			"message": map[string]any{"role": "assistant", "content": reply},
		}},
	}
	if usage {
		out["usage"] = openAIUsage()
	}
	return out
}

func openAIUsage() map[string]any {
	return map[string]any{
		"prompt_tokens": inputTokens, "completion_tokens": outputTokens, "total_tokens": inputTokens + outputTokens,
		"prompt_tokens_details": map[string]any{"cached_tokens": cachedTokens},
	}
}

func streamChat(w http.ResponseWriter, model string, usage bool) {
	w.Header().Set("Content-Type", "text/event-stream")
	flusher, _ := w.(http.Flusher)
	send := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if flusher != nil {
			flusher.Flush()
		}
	}
	chunk := func(delta map[string]any, finish any) map[string]any {
		return map[string]any{
			"id": "chatcmpl-mock", "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": model,
			"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}},
		}
	}
	send(chunk(map[string]any{"role": "assistant", "content": reply}, nil))
	send(chunk(map[string]any{}, "stop"))
	if usage {
		send(map[string]any{
			"id": "chatcmpl-mock", "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": model,
			"choices": []any{}, "usage": openAIUsage(),
		})
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
}

func response(model string, usage bool) map[string]any {
	out := map[string]any{
		"id": "resp-mock", "object": "response", "created_at": time.Now().Unix(), "model": model, "status": "completed",
		"output": []any{map[string]any{
			"type": "message", "role": "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": reply}},
		}},
	}
	if usage {
		out["usage"] = map[string]any{
			"input_tokens": inputTokens, "output_tokens": outputTokens, "total_tokens": inputTokens + outputTokens,
			"input_tokens_details": map[string]any{"cached_tokens": cachedTokens},
		}
	}
	return out
}

func embeddings(model string, usage bool) map[string]any {
	out := map[string]any{
		"object": "list", "model": model,
		"data": []any{map[string]any{"object": "embedding", "index": 0, "embedding": []float64{0.1, 0.2, 0.3}}},
	}
	if usage {
		out["usage"] = map[string]any{"prompt_tokens": inputTokens, "total_tokens": inputTokens}
	}
	return out
}

func anthropicMessage(model string, usage bool) map[string]any {
	out := map[string]any{
		"id": "msg_mock", "type": "message", "role": "assistant", "model": model,
		"content":     []any{map[string]any{"type": "text", "text": reply}},
		"stop_reason": "end_turn",
	}
	if usage {
		out["usage"] = map[string]any{
			"input_tokens": inputTokens - cachedTokens, "cache_read_input_tokens": cachedTokens,
			"output_tokens": outputTokens,
		}
	}
	return out
}

func geminiContent(model string, usage bool) map[string]any {
	out := map[string]any{
		"modelVersion": model,
		"candidates": []any{map[string]any{
			"content":      map[string]any{"role": "model", "parts": []any{map[string]any{"text": reply}}},
			"finishReason": "STOP",
		}},
	}
	if usage {
		out["usageMetadata"] = map[string]any{
			"promptTokenCount": inputTokens, "cachedContentTokenCount": cachedTokens,
			"candidatesTokenCount": outputTokens, "totalTokenCount": inputTokens + outputTokens,
		}
	}
	return out
}

// modelFromPath reads the model from paths that carry it, such as Gemini's
// /v1beta/models/<model>:generateContent and Azure's /openai/deployments/<model>/...
func modelFromPath(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if (p == "models" || p == "deployments") && i+1 < len(parts) {
			return strings.SplitN(parts[i+1], ":", 2)[0]
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
