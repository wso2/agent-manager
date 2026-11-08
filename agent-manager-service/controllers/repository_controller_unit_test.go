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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// A git secret the user picked but that no longer exists, or a name no git
// secret can have, tells the user to pick another secret instead of failing
// as a generic 500.
func TestHandleGitProviderError_GitSecretErrors(t *testing.T) {
	cases := []struct {
		name        string
		err         error
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{
			name:        "deleted git secret is 404",
			err:         fmt.Errorf("%w: %q is not in the namespace's git secrets", utils.ErrGitSecretNotFound, "booking-pat"),
			wantStatus:  http.StatusNotFound,
			wantCode:    utils.ErrCodeGitSecretNotFound,
			wantMessage: "Git secret not found",
		},
		{
			name:        "invalid git secret name is 400",
			err:         utils.NewInvalidInputError("Invalid git secret name", `invalid secretRef "../x": not a valid git secret name`),
			wantStatus:  http.StatusBadRequest,
			wantCode:    utils.ErrCodeValidation,
			wantMessage: "Invalid git secret name",
		},
		{
			name:        "anything else stays 500",
			err:         errors.New("openbao unreachable"),
			wantStatus:  http.StatusInternalServerError,
			wantMessage: "Failed to fetch repository data",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()

			handleGitProviderError(rr, tc.err)

			require.Equal(t, tc.wantStatus, rr.Code)
			var body spec.ErrorResponse
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
			assert.Equal(t, tc.wantMessage, body.Message)
			if tc.wantCode != "" {
				assert.Equal(t, tc.wantCode, body.Code)
			}
		})
	}
}
