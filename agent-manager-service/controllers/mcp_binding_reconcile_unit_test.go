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

package controllers

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The reconcile writes real resources after the triggering write committed, so cancelling
// the request (a client disconnect) must not cancel it — but it must still have a deadline.
func TestReconcileMCPBindingsInBackground_SurvivesRequestCancellation(t *testing.T) {
	reqCtx, cancelReq := context.WithCancel(context.Background())
	cancelReq()

	done := make(chan error, 1)
	reconcileMCPBindingsInBackground(reqCtx, slog.New(slog.NewTextHandler(io.Discard, nil)), "Test",
		func(ctx context.Context) error {
			_, hasDeadline := ctx.Deadline()
			require.True(t, hasDeadline, "detached reconcile must be bounded")
			done <- ctx.Err()
			return nil
		})

	select {
	case err := <-done:
		require.NoError(t, err, "a cancelled request must not cancel the reconcile")
	case <-time.After(5 * time.Second):
		t.Fatal("reconcile never ran")
	}
}

// A panic in the background reconcile is outside the request's panic-recovery middleware;
// it must be contained rather than crash the process (which would fail this test binary).
func TestReconcileMCPBindingsInBackground_RecoversPanic(t *testing.T) {
	ran := make(chan struct{})
	reconcileMCPBindingsInBackground(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), "Test",
		func(context.Context) error {
			defer close(ran)
			panic("boom")
		})

	select {
	case <-ran:
		time.Sleep(50 * time.Millisecond) // let the deferred recover finish
	case <-time.After(5 * time.Second):
		t.Fatal("reconcile never ran")
	}
}
