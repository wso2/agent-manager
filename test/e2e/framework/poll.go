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

package framework

import (
	"context"
	"testing"
	"time"
)

// PollConfig controls the polling behavior.
type PollConfig struct {
	// Timeout is the maximum duration to keep polling.
	Timeout time.Duration
	// InitialInterval is the first sleep between attempts. Default: 5s.
	InitialInterval time.Duration
	// MaxInterval caps the backoff. Default: 30s.
	MaxInterval time.Duration
}

// PollFunc is called on each polling iteration. It should return:
//   - result: the value if the condition is met
//   - done: true if polling should stop (success)
//   - err: non-nil to abort polling immediately with an error
type PollFunc[T any] func() (result T, done bool, err error)

// Poll repeatedly calls fn until it returns done=true, an error, or the timeout expires.
// It uses exponential backoff: interval = interval * 3/2, capped at MaxInterval.
func Poll[T any](t *testing.T, description string, cfg PollConfig, fn PollFunc[T]) T {
	t.Helper()

	if cfg.InitialInterval == 0 {
		cfg.InitialInterval = 5 * time.Second
	}
	if cfg.MaxInterval == 0 {
		cfg.MaxInterval = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	start := time.Now()
	backoff := cfg.InitialInterval
	for {
		result, done, err := fn()
		if err != nil {
			Fatalf(t, "poll %s: aborted with error: %v", description, err)
		}
		if done {
			return result
		}

		elapsed := time.Since(start).Round(time.Second)
		Log(t, "  Waiting... (%s elapsed, next check in %v)", elapsed, backoff)

		select {
		case <-ctx.Done():
			Fatalf(t, "poll %s: timed out after %v", description, cfg.Timeout)
		case <-time.After(backoff):
		}

		if backoff < cfg.MaxInterval {
			backoff = backoff * 3 / 2
			if backoff > cfg.MaxInterval {
				backoff = cfg.MaxInterval
			}
		}
	}
}
