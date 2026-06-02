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

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/wso2/agent-manager/cli/pkg/auth"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/config"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

func TestRunLogin(t *testing.T) {
	cases := []struct {
		name        string
		url         string
		authErr     error
		wantErrCode string
	}{
		{
			name:        "typed CLIError passes through unchanged",
			url:         "https://example.com",
			authErr:     clierr.New(clierr.Unauthorized, "client credentials rejected (401)"),
			wantErrCode: clierr.Unauthorized,
		},
		{
			name:        "plain error becomes Transport",
			url:         "https://example.com",
			authErr:     errors.New("dial tcp: connection refused"),
			wantErrCode: clierr.Transport,
		},
		{
			name:        "AuthLoginCancelled passes through unchanged",
			url:         "https://example.com",
			authErr:     clierr.New(clierr.AuthLoginCancelled, "browser login cancelled"),
			wantErrCode: clierr.AuthLoginCancelled,
		},
		{
			name:        "missing --url returns InvalidFlag without calling Authenticate",
			url:         "",
			authErr:     nil,
			wantErrCode: clierr.InvalidFlag,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			io, _, out, _ := iostreams.Test()
			io.JSON = true

			opts := &LoginOptions{
				IO:  io,
				URL: tc.url,
				Authenticate: func(_ context.Context, _ auth.LoginOptions) (*config.Instance, error) {
					if tc.url == "" {
						t.Fatal("Authenticate should not be called when --url is missing")
					}
					return nil, tc.authErr
				},
			}

			err := runLogin(context.Background(), opts)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			var env map[string]any
			if jerr := json.Unmarshal(out.Bytes(), &env); jerr != nil {
				t.Fatalf("decode envelope: %v\nbody=%q", jerr, out.String())
			}
			errBody, ok := env["error"].(map[string]any)
			if !ok {
				t.Fatalf("envelope missing 'error' field: %v", env)
			}
			if got := errBody["code"]; got != tc.wantErrCode {
				t.Errorf("code = %q, want %q", got, tc.wantErrCode)
			}
		})
	}
}
