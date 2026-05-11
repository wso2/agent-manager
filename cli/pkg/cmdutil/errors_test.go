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
	"testing"

	"github.com/wso2/agent-manager/cli/pkg/clierr"
)

func TestFlagErrors(t *testing.T) {
	violations := []string{
		"--name is required",
		"--build-type=buildpack requires --language",
	}
	err := FlagErrors(violations)
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	// Must be a FlagError (exit code 2).
	var fe *FlagError
	if !errors.As(err, &fe) {
		t.Fatalf("expected *FlagError, got %T", err)
	}

	// Must unwrap to CLIError with InvalidFlag code.
	var ce clierr.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("expected clierr.CLIError, got %T", err)
	}
	if ce.Code != clierr.InvalidFlag {
		t.Errorf("code = %q, want %q", ce.Code, clierr.InvalidFlag)
	}

	// Message must contain each violation on its own line.
	wantMsg := "invalid flags\n    --name is required\n    --build-type=buildpack requires --language"
	if ce.Message != wantMsg {
		t.Errorf("message =\n%q\nwant\n%q", ce.Message, wantMsg)
	}

	// AdditionalData["details"] carries the structured list.
	details, ok := ce.AdditionalData["details"].([]string)
	if !ok {
		t.Fatalf("details type = %T, want []string", ce.AdditionalData["details"])
	}
	if len(details) != 2 {
		t.Fatalf("details len = %d, want 2", len(details))
	}
	if details[0] != violations[0] || details[1] != violations[1] {
		t.Errorf("details = %v, want %v", details, violations)
	}
}
