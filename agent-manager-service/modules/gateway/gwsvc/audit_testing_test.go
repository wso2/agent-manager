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

package gwsvc

import (
	"context"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
)

// auditableCtx returns a context carrying a recorder that discards events.
//
// Operations that must not happen unrecorded — minting a token, rotating a key,
// deploying, deleting — refuse to proceed when no recorder is installed, so a
// bare context makes them fail by design. Tests exercising those paths have to
// say which recorder they mean; a discarding one keeps the assertions about the
// behaviour under test rather than about audit records.
//
// Tests that mean to assert the refusal itself should pass a bare
// context.Background() and expect audit.ErrRecorderUnavailable.
func auditableCtx(t *testing.T) context.Context {
	t.Helper()
	return audit.WithRecorder(context.Background(), audit.NewNoopRecorder())
}
