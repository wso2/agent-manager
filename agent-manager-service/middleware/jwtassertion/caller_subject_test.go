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

package jwtassertion

import (
	"context"
	"testing"
)

// REST and MCP both attribute created_by through CallerSubject, so it must
// report no user, not a guess, when there is none.
func TestCallerSubject(t *testing.T) {
	cases := map[string]struct {
		ctx  context.Context
		want string
	}{
		"user token":         {ContextWithTokenClaims(context.Background(), &TokenClaims{Sub: "user-1"}), "user-1"},
		"token without user": {ContextWithTokenClaims(context.Background(), &TokenClaims{}), ""},
		"no token":           {context.Background(), ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := CallerSubject(tc.ctx); got != tc.want {
				t.Errorf("CallerSubject() = %q, want %q", got, tc.want)
			}
		})
	}
}
