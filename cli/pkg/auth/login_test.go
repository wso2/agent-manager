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

package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/oauth2"

	"github.com/wso2/agent-manager/cli/pkg/clierr"
)

func TestClassifyTokenError(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode string // empty means expect nil return
		wantMsg  string // substring expected in message
	}{
		{
			name:     "401 from token endpoint",
			err:      &oauth2.RetrieveError{Response: &http.Response{StatusCode: http.StatusUnauthorized}},
			wantCode: clierr.Unauthorized,
			wantMsg:  "401",
		},
		{
			name: "invalid_grant with description",
			err: &oauth2.RetrieveError{
				Response:         &http.Response{StatusCode: http.StatusBadRequest},
				ErrorCode:        "invalid_grant",
				ErrorDescription: "token has expired",
			},
			wantCode: clierr.Unauthorized,
			wantMsg:  "invalid_grant",
		},
		{
			name: "invalid_grant on 400 response",
			err: &oauth2.RetrieveError{
				Response:  &http.Response{StatusCode: http.StatusBadRequest},
				ErrorCode: "invalid_grant",
			},
			wantCode: clierr.Unauthorized,
		},
		{
			name:     "403 forbidden — not classified",
			err:      &oauth2.RetrieveError{Response: &http.Response{StatusCode: http.StatusForbidden}},
			wantCode: "",
		},
		{
			name:     "other oauth error code — not classified",
			err:      &oauth2.RetrieveError{Response: &http.Response{StatusCode: http.StatusBadRequest}, ErrorCode: "unsupported_grant_type"},
			wantCode: "",
		},
		{
			name:     "plain error — not classified",
			err:      errors.New("connection refused"),
			wantCode: "",
		},
		{
			name:     "nil response on RetrieveError — not classified",
			err:      &oauth2.RetrieveError{Response: nil, ErrorCode: ""},
			wantCode: "",
		},
		{
			name:     "wrapped RetrieveError with 401 — still classified",
			err:      fmt.Errorf("outer wrapper: %w", &oauth2.RetrieveError{Response: &http.Response{StatusCode: http.StatusUnauthorized}}),
			wantCode: clierr.Unauthorized,
			wantMsg:  "401",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyTokenError(tc.err)

			if tc.wantCode == "" {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}

			if got == nil {
				t.Fatalf("expected classified error with code %q, got nil", tc.wantCode)
			}

			var ce clierr.CLIError
			if !errors.As(got, &ce) {
				t.Fatalf("returned error is not a clierr.CLIError: %T", got)
			}
			if ce.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", ce.Code, tc.wantCode)
			}
			if tc.wantMsg != "" && !strings.Contains(ce.Message, tc.wantMsg) {
				t.Errorf("message = %q, want to contain %q", ce.Message, tc.wantMsg)
			}
		})
	}
}
