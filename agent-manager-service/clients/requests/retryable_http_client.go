// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package requests

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// errRetry is a sentinel error used internally to signal retry attempts.
var errRetry = errors.New("retry")

// RetryableHTTPClient wraps an HttpClient with retry logic.
// It implements HttpClient interface and can be used with oapi-codegen generated clients.
type RetryableHTTPClient struct {
	client HttpClient
	config RequestRetryConfig
}

// NewRetryableHTTPClient creates a new RetryableHTTPClient.
// Config is optional - defaults will be used if not provided.
func NewRetryableHTTPClient(client HttpClient, config ...RequestRetryConfig) *RetryableHTTPClient {
	if client == nil {
		client = &http.Client{}
	}
	var cfg RequestRetryConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	return &RetryableHTTPClient{
		client: client,
		config: cfg,
	}
}

// Do executes the HTTP request with retry logic.
func (c *RetryableHTTPClient) Do(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	cfg := c.config.getRetryConfig(&HttpRequest{Method: req.Method})
	// The context logger carries the correlation ID of the request that caused
	// this call, so an upstream failure can be read next to the API request it
	// belongs to instead of being an orphan line.
	log := logger.GetLogger(ctx).With(
		slog.String("log_type", "upstream"),
		// Prefixed because the context logger already carries `method` for the
		// inbound request. Unprefixed, one record would hold two different
		// methods under one key and a reader could not tell which is which.
		slog.String("upstream_method", req.Method),
		slog.String("upstream_host", req.URL.Host),
		// The path can carry caller-supplied segments.
		slog.String("upstream_url", utils.TruncateForLog(utils.SanitizeForLog(req.URL.String()), 512)),
	)

	// Capture body bytes for replay on retries
	var bodyBytes []byte
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		if closeErr := req.Body.Close(); closeErr != nil {
			log.Warn("failed to close request body", slog.String("error", closeErr.Error()))
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
	}

	start := time.Now()
	for attempt := 1; attempt <= cfg.RetryAttemptsMax+1; attempt++ {
		isLastAttempt := attempt == cfg.RetryAttemptsMax+1

		// Reset body for each attempt
		if bodyBytes != nil {
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		resp, err := c.doAttempt(ctx, req, cfg, attempt, isLastAttempt, log)
		if !errors.Is(err, errRetry) {
			// One record per outbound call, whatever the outcome. Without it a
			// slow or failing dependency is invisible: the per-attempt lines
			// below only fire on retryable failures.
			logUpstreamResult(log, resp, err, attempt, start)
			return resp, err
		}

		// errRetry means retry - wait before next attempt
		if !isLastAttempt {
			waitDuration := calculateBackoff(cfg.RetryWaitMin, cfg.RetryWaitMax, attempt)
			select {
			case <-time.After(waitDuration):
				// Continue to next attempt
			case <-ctx.Done():
				return nil, fmt.Errorf("context cancelled during retry wait: %w", ctx.Err())
			}
		}
	}
	return nil, fmt.Errorf("unreachable: retry loop exited without returning a response or error")
}

func (c *RetryableHTTPClient) doAttempt(ctx context.Context, req *http.Request, cfg RequestRetryConfig, attempt int, isLastAttempt bool, log *slog.Logger) (*http.Response, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, cfg.AttemptTimeout)
	defer cancel()

	reqWithTimeout := req.Clone(attemptCtx)
	start := time.Now()
	resp, err := c.client.Do(reqWithTimeout)
	elapsed := time.Since(start)

	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("context cancelled or timed out: %w", ctx.Err())
		}
		if attemptCtx.Err() != nil {
			logAttrs := []any{
				slog.Int("attempt", attempt),
				slog.Int("max_attempts", cfg.RetryAttemptsMax+1),
				slog.Duration("timeout", cfg.AttemptTimeout),
			}
			if isLastAttempt {
				log.Warn("HTTP request timed out after all attempts", logAttrs...)
				return nil, fmt.Errorf("request timed out after %d attempts: %w", attempt, err)
			}
			log.Debug("HTTP request attempt timed out, retrying", logAttrs...)
			return nil, errRetry
		}
		logAttrs := []any{
			slog.Int("attempt", attempt),
			slog.Int("max_attempts", cfg.RetryAttemptsMax+1),
			slog.String("error", err.Error()),
		}
		if isLastAttempt {
			log.Warn("HTTP request failed after all attempts", logAttrs...)
			return nil, fmt.Errorf("request failed after %d attempts: %w", attempt, err)
		}
		log.Debug("HTTP request failed, retrying", logAttrs...)
		return nil, errRetry
	}

	// Check if status code is retryable
	if cfg.RetryOnStatus != nil && cfg.RetryOnStatus(resp.StatusCode) {
		logAttrs := []any{
			slog.Int("attempt", attempt),
			slog.Int("max_attempts", cfg.RetryAttemptsMax+1),
			slog.Int64("duration_ms", elapsed.Milliseconds()),
			slog.Int("status", resp.StatusCode),
		}
		if isLastAttempt {
			log.Warn("HTTP request returned retryable status after all attempts", logAttrs...)
			// Read body before attemptCtx is canceled to prevent "context canceled" errors
			bodyBytes, err := io.ReadAll(resp.Body)
			if closeErr := resp.Body.Close(); closeErr != nil {
				log.Warn("failed to close response body", slog.String("error", closeErr.Error()))
			}
			if err != nil {
				return nil, fmt.Errorf("failed to read response body: %w", err)
			}
			// Replace body with buffered version
			resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			return resp, nil
		}
		log.Debug("HTTP request returned retryable status, retrying", logAttrs...)
		// Drain and close body to allow connection reuse
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			log.Warn("failed to drain response body", slog.String("error", err.Error()))
		}
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Warn("failed to close response body", slog.String("error", closeErr.Error()))
		}
		return nil, errRetry
	}

	// Read body before attemptCtx is canceled to prevent "context canceled" errors
	bodyBytes, err := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); closeErr != nil {
		log.Warn("failed to close response body", slog.String("error", closeErr.Error()))
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	// Replace body with buffered version so caller can still read it
	resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	return resp, nil
}

// logUpstreamResult records how an outbound call finished. Levels follow the
// same rule as the inbound access log: a 5xx or transport failure is ours to
// investigate, a 4xx is the upstream telling us no.
func logUpstreamResult(log *slog.Logger, resp *http.Response, err error, attempts int, start time.Time) {
	attrs := []any{
		slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		slog.Int("attempts", attempts),
	}
	if err != nil {
		log.Error("upstream call failed", append(attrs, slog.String("error", err.Error()))...)
		return
	}
	attrs = append(attrs, slog.Int("status", resp.StatusCode))
	switch {
	case resp.StatusCode >= http.StatusInternalServerError:
		log.Error("upstream call completed", attrs...)
	case resp.StatusCode >= http.StatusBadRequest:
		log.Warn("upstream call completed", attrs...)
	default:
		log.Debug("upstream call completed", attrs...)
	}
}

// calculateBackoff returns an exponential backoff duration with jitter, capped by max.
// Uses "equal jitter" strategy: base/2 + random(0, base/2), giving a range of [base/2, base].
// This prevents thundering herd when many clients retry simultaneously.
func calculateBackoff(min, max time.Duration, attempt int) time.Duration {
	// Calculate base exponential backoff: 2^(attempt-1) * min
	base := min * time.Duration(1<<uint(attempt-1))
	if base > max {
		base = max
	}
	// Equal jitter: random value between base/2 and base
	halfBase := base / 2
	if halfBase <= 0 {
		return base
	}
	return halfBase + time.Duration(rand.Int64N(int64(halfBase)))
}
