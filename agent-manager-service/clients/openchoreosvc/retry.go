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

package openchoreosvc

import (
	"context"
	"log/slog"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// BackoffFunc defines the function signature for backoff calculation
type BackoffFunc func(attemptCount int) time.Duration

// RetryConfig holds configuration for retry behavior
type RetryConfig struct {
	MaxRetries  int
	BackoffFunc BackoffFunc
}

// defaultRetryConfig returns a sensible default retry configuration
func defaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		BackoffFunc: func(attemptCount int) time.Duration {
			// Exponential backoff: wait 2^(attemptCount-1) before retrying
			return time.Duration(1<<uint(attemptCount-1)) * time.Second
		},
	}
}

// retryK8sOperation executes a K8s operation with retry logic
func (k *openChoreoSvcClient) retryK8sOperation(ctx context.Context, operationName string, operation func() error) error {
	config := defaultRetryConfig()
	var lastErr error

	for attempt := 1; attempt <= config.MaxRetries+1; attempt++ {

		// Check context before attempting
		select {
		case <-ctx.Done():
			slog.Warn("K8s operation canceled",
				"operation", operationName,
				"context_error", ctx.Err())
			return ctx.Err()
		default:
		}

		// Apply backoff delay before retry (skip on first attempt)
		if attempt > 1 {
			backoffDuration := config.BackoffFunc(attempt - 1)
			slog.Info("retrying K8s operation after backoff",
				"operation", operationName,
				"attempt", attempt,
				"backoff", backoffDuration.String())
			select {
			case <-time.After(backoffDuration):
				// Wait completed, continue to next retry
			case <-ctx.Done():
				slog.Warn("K8s operation canceled during backoff",
					"operation", operationName,
					"context_error", ctx.Err())
				return ctx.Err()
			}
		}

		// Execute the operation
		err := operation()
		if err == nil {
			// Operation succeeded
			if attempt > 1 {
				slog.Info("K8s operation succeeded after retry",
					"operation", operationName,
					"attempt", attempt)
			}
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryableK8sError(err) {
			slog.Debug("K8s operation failed with non-retryable error",
				"operation", operationName,
				"error", err,
				"attempt", attempt)
			return err
		}

		slog.Warn("K8s operation failed with retryable error",
			"operation", operationName,
			"error", err,
			"attempt", attempt,
			"max_retries", config.MaxRetries)
	}

	slog.Error("K8s operation failed after all retries",
		"operation", operationName,
		"attempts", config.MaxRetries+1,
		"error", lastErr)
	return lastErr
}

// isRetryableK8sError determines if a K8s error is retryable
func isRetryableK8sError(err error) bool {
	if err == nil {
		return false
	}

	// K8s API errors that are typically retryable
	if apierrors.IsTimeout(err) {
		return true
	}
	if apierrors.IsServerTimeout(err) {
		return true
	}
	if apierrors.IsServiceUnavailable(err) {
		return true
	}
	if apierrors.IsInternalError(err) {
		return true
	}
	if apierrors.IsTooManyRequests(err) {
		return true
	}
	// Conflict errors - resource was modified, should refetch and retry
	if apierrors.IsConflict(err) {
		return true
	}
	// Temporary network issues
	if apierrors.IsUnexpectedServerError(err) {
		return true
	}

	// Check error message for common transient issues
	errMsg := err.Error()
	transientErrors := []string{
		"connection refused",
		"connection reset",
		"i/o timeout",
		"timed out",
		"deadline exceeded",
		"context deadline exceeded",
		"temporary failure",
		"network is unreachable",
		"no such host",
		"eof",
		"broken pipe",
		"client rate limiter",
	}

	for _, transientErr := range transientErrors {
		if strings.Contains(strings.ToLower(errMsg), transientErr) {
			return true
		}
	}

	return false
}
