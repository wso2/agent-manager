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
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/logger"
)

// BeginAuditOrFail starts a fail-closed audit attempt and, when the trail
// cannot accept it, writes the refusal response itself.
//
// Controllers repeated this block verbatim at every fail-closed site: begin,
// log, 503, return. Centralising it makes the refusal impossible to forget and
// keeps the status and message consistent — a site that logged but returned 500,
// or logged and continued, would be a silent hole in the guarantee.
//
// Returns false when the caller must stop; the response is already written.
func BeginAuditOrFail(
	w http.ResponseWriter,
	r *http.Request,
	operation, failureMessage string,
	action audit.Action,
	opts ...audit.Option,
) (*audit.Attempt, bool) {
	ctx := r.Context()

	attempt, err := audit.Begin(ctx, action, opts...)
	if err != nil {
		logger.GetLogger(ctx).Error(operation+": refusing, audit record could not be written",
			"action", string(action), "error", err)
		httpx.WriteErrorResponse(w, http.StatusServiceUnavailable, failureMessage)
		return nil, false
	}
	return attempt, true
}
