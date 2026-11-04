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

package reconcile

import (
	"errors"
	"fmt"

	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// ErrInputNotReady matches a Render error made by InputNotReady.
var ErrInputNotReady = errors.New("input not ready")

// ErrRecordGone is Render's re-check finding no per-environment record: a user undeploy raced the placement pass.
// The core undeploys the row instead of deploying it.
var ErrRecordGone = Permanent(errors.New("the artifact's per-environment record is gone"))

// ErrNotPlaced is a status asked for an artifact with no row on the gateway.
var ErrNotPlaced = errors.New("artifact is not placed on the gateway")

// ErrQueueFailed is MarkStale failing to queue the redeploys: nothing is owed, so no gateway will be re-rendered.
var ErrQueueFailed = errors.New("failed to queue renders")

type inputNotReady struct{ reason string }

func (e *inputNotReady) Error() string        { return e.reason }
func (e *inputNotReady) Is(target error) bool { return target == ErrInputNotReady }

// InputNotReady is a Render error for a dependency that isn't there yet. The row keeps its status and the render is
// retried with backoff, never giving up; reason is what the user is told it is waiting for.
func InputNotReady(reason string) error {
	return &inputNotReady{reason: reason}
}

type permanent struct{ err error }

func (e *permanent) Error() string { return e.err.Error() }
func (e *permanent) Unwrap() error { return e.err }

// Permanent marks a Render error that retrying won't fix: the owed deploy is given up.
func Permanent(err error) error {
	return &permanent{err: err}
}

// RenderFailedError is a claimed render that failed, with its outcome already recorded on the row: the row is
// queued, waiting or given up, and the gateway keeps what it was last sent. An inline caller is told; the worker
// isn't, since the outcome is its whole answer.
type RenderFailedError struct {
	Outcome repositories.RenderOutcomeKind
	Reason  string
	Err     error
}

func (e *RenderFailedError) Error() string { return fmt.Sprintf("render failed: %s", e.Reason) }
func (e *RenderFailedError) Unwrap() error { return e.Err }

// ignoreRenderFailed drops a RenderFailedError, which the worker has nothing more to do with.
func ignoreRenderFailed(err error) error {
	var failed *RenderFailedError
	if errors.As(err, &failed) {
		return nil
	}
	return err
}

// ignoreWaiting drops a render waiting for an input: the worker retries it, and it never fails a request.
func ignoreWaiting(err error) error {
	var failed *RenderFailedError
	if errors.As(err, &failed) && failed.Outcome == repositories.RenderWaiting {
		return nil
	}
	return err
}

// IsPermanent reports whether err, or an error it wraps, was marked Permanent.
func IsPermanent(err error) bool {
	var p *permanent
	return errors.As(err, &p)
}
