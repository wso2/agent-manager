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
	"net/http"
	"slices"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

var TransientHTTPErrorCodes = []int{
	http.StatusTooManyRequests,    // 429
	http.StatusBadGateway,         // 502
	http.StatusServiceUnavailable, // 503
	http.StatusGatewayTimeout,     // 504
}

var TransientHTTPGETErrorCodes = []int{
	http.StatusTooManyRequests,     // 429
	http.StatusInternalServerError, // 500
	http.StatusBadGateway,          // 502
	http.StatusServiceUnavailable,  // 503
	http.StatusGatewayTimeout,      // 504
}

type RequestRetryConfig struct {
	RetryWaitMin time.Duration
	RetryWaitMax time.Duration
	// RetryAttemptsMax is the maximum number of retries to attempt. 0 for no retries.
	RetryAttemptsMax int
	// AttemptTimeout is the maximum time allowed for a single request attempt.
	AttemptTimeout time.Duration
	// RetryOnStatus is a function that returns true if the request should be retried based on the status code.
	RetryOnStatus func(status int) bool
}

func (cfg RequestRetryConfig) withDefaults(req *HttpRequest) RequestRetryConfig {
	if cfg.RetryWaitMin == 0 {
		cfg.RetryWaitMin = 1 * time.Second
	}
	if cfg.RetryWaitMax == 0 {
		cfg.RetryWaitMax = 10 * time.Second
	}
	if cfg.RetryAttemptsMax == 0 {
		cfg.RetryAttemptsMax = 3
	}
	if cfg.AttemptTimeout == 0 {
		cfg.AttemptTimeout = 3 * time.Minute
	}
	if cfg.RetryOnStatus == nil {
		cfg.RetryOnStatus = func(status int) bool {
			if req.Method == http.MethodGet || req.Method == http.MethodDelete {
				return slices.Contains(TransientHTTPGETErrorCodes, status)
			}
			return slices.Contains(TransientHTTPErrorCodes, status)
		}
	}
	return cfg
}

func (cfg RequestRetryConfig) makeCheckRetry() retryablehttp.CheckRetry {
	return func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		if err != nil { // not nil for network errors, context.DeadlineExceeded etc.
			return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
		}
		if cfg.RetryOnStatus != nil {
			return cfg.RetryOnStatus(resp.StatusCode), nil
		}
		return false, nil
	}
}
