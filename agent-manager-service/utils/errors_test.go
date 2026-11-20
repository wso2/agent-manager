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

package utils

import (
	"errors"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

// A ValidationError carrying a short UI message is only usable in place of a
// fmt.Errorf("%w: ...", ErrInvalidInput) if callers that classify errors by
// sentinel still recognise it — otherwise swapping one for the other silently
// reroutes a 400 into the controller's generic 500 fallback.
func TestNewInvalidInputError_MatchesItsSentinelAndKeepsBothHalves(t *testing.T) {
	err := httpx.NewInvalidInputError("Promotion blocked: identity is not ready", "retry once provisioning completes")

	if !errors.Is(err, httpx.ErrInvalidInput) {
		t.Errorf("errors.Is(err, httpx.ErrInvalidInput) = false, want true")
	}
	ve := httpx.IsValidationError(err)
	if ve == nil {
		t.Fatalf("IsValidationError(err) = nil, want the httpx.ValidationError")
	}
	if ve.Message != "Promotion blocked: identity is not ready" {
		t.Errorf("Message = %q", ve.Message)
	}
	if ve.Reason != "retry once provisioning completes" {
		t.Errorf("Reason = %q", ve.Reason)
	}
	if err.Error() != "retry once provisioning completes" {
		t.Errorf("Error() = %q, want the technical reason", err.Error())
	}
}

// A plain NewValidationError has no sentinel to match, and asking whether it
// matches one must not panic on the nil wrapped error.
func TestNewValidationError_MatchesNoSentinel(t *testing.T) {
	err := httpx.NewValidationError("Agent type is required", "agentType is required")

	if errors.Is(err, httpx.ErrInvalidInput) {
		t.Errorf("errors.Is(err, httpx.ErrInvalidInput) = true, want false for a sentinel-less validation error")
	}
}
