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

package cmdutil

import (
	"errors"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/cli/pkg/clierr"
)

func TestValidatePathParam(t *testing.T) {
	tests := []struct {
		name    string
		label   string
		value   string
		wantErr bool
		errMsg  string
	}{
		{name: "valid name", label: "agent name", value: "order-triage"},
		{name: "valid with dots", label: "agent name", value: "my.agent.v2"},
		{name: "valid with spaces", label: "build name", value: "my build"},
		{name: "empty string", label: "agent name", value: "", wantErr: true, errMsg: "must not be empty"},
		{name: "whitespace only", label: "agent name", value: "   ", wantErr: true, errMsg: "must not be empty"},
		{name: "tab only", label: "deploy ID", value: "\t", wantErr: true, errMsg: "must not be empty"},
		{name: "contains slash", label: "agent name", value: "foo/bar", wantErr: true, errMsg: "must not contain '/'"},
		{name: "single slash", label: "agent name", value: "/", wantErr: true, errMsg: "'/'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePathParam(tt.label, tt.value)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			var flagErr *FlagError
			if !errors.As(err, &flagErr) {
				t.Fatalf("error is %T, want *FlagError", err)
			}

			var cliErr clierr.CLIError
			if !errors.As(err, &cliErr) {
				t.Fatalf("error does not unwrap to clierr.CLIError")
			}
			if cliErr.Code != clierr.InvalidFlag {
				t.Errorf("code = %q, want %q", cliErr.Code, clierr.InvalidFlag)
			}

			if !strings.Contains(err.Error(), tt.label) {
				t.Errorf("error %q does not contain label %q", err.Error(), tt.label)
			}
			if !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.errMsg)
			}
		})
	}
}
