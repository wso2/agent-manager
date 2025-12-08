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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"slices"
	"time"

	"github.com/hashicorp/go-retryablehttp"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
)

type HttpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

var _ HttpClient = (*http.Client)(nil)

func SendRequest(ctx context.Context, client HttpClient, req *HttpRequest, opts ...RequestRetryConfig) *Result {
	var retryConfig RequestRetryConfig
	if len(opts) > 0 {
		retryConfig = opts[0]
	}
	retryConfig = retryConfig.withDefaults(req)
	checkRetry := retryConfig.makeCheckRetry()
	log := logger.GetLogger(ctx).With(slog.String("request", req.Name))
	// RetryAttemptsMax=0: attempt 1
	// RetryAttemptsMax=2: attempt 1, 2, 3
	for attempt := 1; attempt < retryConfig.RetryAttemptsMax+2; attempt++ {
		isLastAttempt := attempt == retryConfig.RetryAttemptsMax+1

		// Execute attempt in a function to ensure proper cleanup via defer
		result := func() *Result {
			attemptCtx, cancel := context.WithTimeout(ctx, retryConfig.AttemptTimeout)
			defer cancel() // Cleanup happens at end of each iteration

			httpReq, err := req.buildHttpRequest(attemptCtx)
			if err != nil {
				return &Result{err: fmt.Errorf("failed to build http request: %w", err)}
			}
			start := time.Now()
			resp, err := client.Do(httpReq)
			if err != nil {
				// Check if parent context was cancelled
				if ctx.Err() != nil {
					return &Result{err: fmt.Errorf("parent context cancelled or timed out: %w", ctx.Err())}
				}
				// Check if attempt context timed out
				if attemptCtx.Err() != nil {
					log.Info("HTTP request attempt timed out, will retry",
						slog.Int("attempt", attempt),
						slog.Int("maxAttempts", retryConfig.RetryAttemptsMax+1),
						slog.Duration("timeout", retryConfig.AttemptTimeout))
					if isLastAttempt {
						return &Result{response: resp, err: fmt.Errorf("request failed with: %w", err)}
					}
					return nil // Signal to retry
				}
				if isLastAttempt {
					return &Result{response: resp, err: fmt.Errorf("request failed with: %w", err)}
				}
				log.Info("HTTP request failed, will retry",
					slog.Int("attempt", attempt),
					slog.Int("maxAttempts", retryConfig.RetryAttemptsMax+1),
					slog.String("error", err.Error()))
				return nil // Signal to retry
			}
			elapsedTime := time.Since(start)

			// Read response body and close immediately to avoid resource leaks
			respBody, err := io.ReadAll(resp.Body)
			closeErr := resp.Body.Close()
			if closeErr != nil {
				log.Warn("failed to close response body", slog.String("error", closeErr.Error()))
			}
			if err != nil {
				return &Result{err: fmt.Errorf("failed to read response body: %w", err)}
			}
			// using parent context to check if the request should be retried
			// as client timeout shouldn't affect the retry logic.
			shouldRetry, shouldRetryErr := checkRetry(ctx, resp, err)
			if !shouldRetry && shouldRetryErr == nil {
				// Request completed successfully (non-retryable response received)
				return &Result{response: resp, err: nil, responseBody: respBody}
			}
			if isLastAttempt {
				return &Result{response: resp, err: shouldRetryErr, responseBody: respBody}
			}
			log.Info("HTTP request returned retryable status, will retry",
				slog.Int("attempt", attempt),
				slog.Int("maxAttempts", retryConfig.RetryAttemptsMax+1),
				slog.Duration("duration", elapsedTime),
				slog.Int("status", resp.StatusCode))

			// Wait before retry
			waitDuration := retryablehttp.DefaultBackoff(retryConfig.RetryWaitMin, retryConfig.RetryWaitMax, attempt, resp)
			select {
			case <-time.After(waitDuration):
				return nil // Signal to retry
			case <-ctx.Done():
				return &Result{err: fmt.Errorf("parent context cancelled during retry wait: %w", ctx.Err())}
			}
		}()

		// If result is not nil, we have a final result to return
		if result != nil {
			return result
		}
		// Otherwise, continue to next retry attempt
	}
	return &Result{err: fmt.Errorf("unexpected error: reached max retry attempts: %d", retryConfig.RetryAttemptsMax)}
}

type Result struct {
	responseBody []byte
	response     *http.Response
	err          error
}

func (r *Result) ScanResponse(body any, successStatus int) error {
	if r.err != nil {
		return r.err
	}
	if r.response == nil {
		return fmt.Errorf("unexpected nil response")
	}
	if body == nil || reflect.ValueOf(body).Kind() != reflect.Ptr {
		return fmt.Errorf("non-nil pointer expected for decoding response body")
	}
	if r.response.StatusCode != successStatus {
		return &HttpError{
			StatusCode: r.response.StatusCode,
			Body:       string(r.responseBody),
		}
	}
	if err := json.Unmarshal(r.responseBody, body); err != nil {
		// if decoding fails, we should not lose the status code.
		// As the request was successful & may contain sensitive information,
		// response body should not be logged or returned as error.
		return fmt.Errorf("failed to decode response body for status %d: %w", r.response.StatusCode, err)
	}
	return nil
}

func (r *Result) CheckStatus(successStatuses ...int) (int, error) {
	if r.err != nil {
		return 0, r.err
	}
	status := r.response.StatusCode
	if !slices.Contains(successStatuses, status) {
		return status, &HttpError{
			StatusCode: r.response.StatusCode,
			Body:       string(r.responseBody),
		}
	}
	return status, nil
}
