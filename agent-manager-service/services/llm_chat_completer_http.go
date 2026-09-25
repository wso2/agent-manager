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

package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// chatCompletionTimeout bounds the upstream call. The console blocks a user on this
// request, so it fails fast rather than holding a spinner open indefinitely.
const chatCompletionTimeout = 90 * time.Second

// maxChatResponseBytes caps how much of an upstream response is read. A completion of
// this size is already far past the token budget, so anything larger is a malfunctioning
// or hostile upstream, not an answer.
const maxChatResponseBytes = 4 << 20 // 4 MiB

// httpChatCompleter calls an LLM upstream directly over HTTP. It is used only by the
// evaluator authoring aid; production inference goes through the gateway proxy, which
// applies guardrails and rate limits this path does not.
type httpChatCompleter struct {
	client *http.Client
}

// NewHTTPChatCompleter creates an LLMChatCompleter backed by a plain HTTP client.
func NewHTTPChatCompleter() LLMChatCompleter {
	return &httpChatCompleter{
		client: &http.Client{
			Timeout: chatCompletionTimeout,
			// Go replays the Authorization/api-key header on a same-host redirect, so
			// an upstream that redirects https -> http would hand the provider's
			// credential to the network in cleartext. Refuse that downgrade; ordinary
			// redirects still follow.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) > 0 && via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
					return fmt.Errorf("%w: refusing redirect from https to %s",
						utils.ErrLLMUpstreamFailed, req.URL.Scheme)
				}
				return nil
			},
		},
	}
}

// Complete sends one completion request in the dialect the caller resolved and returns
// the assistant's text.
func (c *httpChatCompleter) Complete(ctx context.Context, req ChatCompletionRequest) (string, error) {
	url, body, err := buildChatPayload(req)
	if err != nil {
		return "", err
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("failed to encode upstream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		return "", fmt.Errorf("failed to build upstream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(req.AuthHeader, req.AuthValue)
	if req.Dialect == DialectAnthropic {
		// Anthropic rejects /v1/messages without an explicit API version.
		httpReq.Header.Set("anthropic-version", anthropicAPIVersion)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("%w: %w", utils.ErrLLMUpstreamFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxChatResponseBytes))
	if err != nil {
		return "", fmt.Errorf("%w: failed to read upstream response: %w", utils.ErrLLMUpstreamFailed, err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		// The upstream body can echo the prompt, so only the status is surfaced. An
		// operator who needs the body has the upstream's own logs.
		return "", fmt.Errorf("%w: upstream returned status %d", utils.ErrLLMUpstreamFailed, resp.StatusCode)
	}

	return extractCompletionText(req.Dialect, raw)
}

// anthropicAPIVersion is the Messages API version this client speaks.
const anthropicAPIVersion = "2023-06-01"

// buildChatPayload returns the URL and request body for the given dialect.
func buildChatPayload(req ChatCompletionRequest) (string, any, error) {
	switch req.Dialect {
	case DialectOpenAI:
		endpoint, err := joinUpstreamPath(req.BaseURL, "chat", "completions")
		if err != nil {
			return "", nil, err
		}
		return endpoint, map[string]any{
			"model": req.Model,
			"messages": []map[string]string{
				{"role": "system", "content": req.SystemPrompt},
				{"role": "user", "content": req.UserPrompt},
			},
			"max_tokens": req.MaxTokens,
		}, nil
	case DialectAnthropic:
		endpoint, err := joinUpstreamPath(req.BaseURL, "v1", "messages")
		if err != nil {
			return "", nil, err
		}
		return endpoint, map[string]any{
			"model":  req.Model,
			"system": req.SystemPrompt,
			"messages": []map[string]string{
				{"role": "user", "content": req.UserPrompt},
			},
			"max_tokens": req.MaxTokens,
		}, nil
	default:
		return "", nil, fmt.Errorf("%w: unsupported dialect %q",
			utils.ErrLLMProviderNotGenerationCapable, req.Dialect)
	}
}

// joinUpstreamPath appends path segments to the provider's base URL.
//
// Parsed rather than concatenated: a base URL carrying a query string would
// otherwise produce ".../v1?foo=bar/chat/completions", putting the path inside the
// query. JoinPath also collapses duplicate separators.
func joinUpstreamPath(base string, segments ...string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("%w: provider upstream URL is malformed",
			utils.ErrLLMProviderNotGenerationCapable)
	}
	return parsed.JoinPath(segments...).String(), nil
}

// openAIChatResponse is the subset of an OpenAI chat completion this client reads.
type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// anthropicMessageResponse is the subset of an Anthropic message this client reads.
type anthropicMessageResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// extractCompletionText pulls the assistant's text out of a successful response.
func extractCompletionText(dialect ChatDialect, raw []byte) (string, error) {
	switch dialect {
	case DialectOpenAI:
		var parsed openAIChatResponse
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return "", fmt.Errorf("%w: failed to parse upstream response: %w", utils.ErrLLMUpstreamFailed, err)
		}
		if len(parsed.Choices) == 0 {
			return "", fmt.Errorf("%w: upstream response contained no choices", utils.ErrLLMUpstreamFailed)
		}
		return parsed.Choices[0].Message.Content, nil
	case DialectAnthropic:
		var parsed anthropicMessageResponse
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return "", fmt.Errorf("%w: failed to parse upstream response: %w", utils.ErrLLMUpstreamFailed, err)
		}
		// A message can carry several blocks; only text blocks hold the completion.
		var out bytes.Buffer
		for _, block := range parsed.Content {
			if block.Type == "text" {
				out.WriteString(block.Text)
			}
		}
		if out.Len() == 0 {
			return "", fmt.Errorf("%w: upstream response contained no text content", utils.ErrLLMUpstreamFailed)
		}
		return out.String(), nil
	default:
		return "", fmt.Errorf("%w: unsupported dialect %q",
			utils.ErrLLMProviderNotGenerationCapable, dialect)
	}
}
