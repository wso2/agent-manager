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

package render

import (
	"errors"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

func TestJSONSuccess_WritesEnvelope(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	ios.JSON = true

	scope := Scope{Instance: "prod", Org: "acme"}
	err := JSONSuccess(ios, scope, map[string]string{"key": "val"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("expected output")
	}
}

func TestJSONError_WritesEnvelope(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	ios.JSON = true

	scope := Scope{Instance: "prod"}
	err := JSONError(ios, scope, clierr.New(clierr.NoOrg, "missing org"))
	if err == nil {
		t.Fatal("expected renderedError")
	}
	if !IsRendered(err) {
		t.Fatal("expected IsRendered to be true")
	}
	if out.Len() == 0 {
		t.Fatal("expected JSON output on stdout")
	}
}

func TestError_DispatchesJSON(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	ios.JSON = true

	scope := Scope{Instance: "prod"}
	err := Error(ios, scope, clierr.New(clierr.NoOrg, "missing org"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsRendered(err) {
		t.Fatal("expected IsRendered")
	}
	if out.Len() == 0 {
		t.Fatal("expected JSON on stdout")
	}
}

func TestError_DispatchesText(t *testing.T) {
	ios, _, out, errOut := iostreams.Test()
	ios.JSON = false

	scope := Scope{Instance: "prod"}
	err := Error(ios, scope, clierr.New(clierr.NoOrg, "missing org"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsRendered(err) {
		t.Fatal("expected IsRendered")
	}
	if out.Len() != 0 {
		t.Errorf("stdout should be empty in text mode, got %q", out.String())
	}
	if errOut.Len() == 0 {
		t.Fatal("expected text error on stderr")
	}
	if got := errOut.String(); got == "" {
		t.Fatal("expected non-empty stderr")
	}
}

func TestError_TextUsesMessageNotCode(t *testing.T) {
	ios, _, _, errOut := iostreams.Test()
	ios.JSON = false

	scope := Scope{Instance: "prod"}
	_ = Error(ios, scope, clierr.New(clierr.NoOrg, "no organization set"))
	got := errOut.String()
	if !strings.Contains(got, "no organization set") {
		t.Errorf("stderr = %q, want it to contain the message", got)
	}
}

func TestError_TextFallsBackForPlainError(t *testing.T) {
	ios, _, _, errOut := iostreams.Test()
	ios.JSON = false

	scope := Scope{}
	_ = Error(ios, scope, errors.New("something broke"))
	got := errOut.String()
	if !strings.Contains(got, "something broke") {
		t.Errorf("stderr = %q, want it to contain the message", got)
	}
}

func TestIsRendered_Unwrap(t *testing.T) {
	inner := clierr.New(clierr.NoOrg, "test")
	rendered := &renderedError{err: inner}

	var cli clierr.CLIError
	if !errors.As(rendered, &cli) {
		t.Fatal("expected errors.As to find clierr.CLIError through renderedError")
	}
	if cli.Code != clierr.NoOrg {
		t.Errorf("code = %q, want %q", cli.Code, clierr.NoOrg)
	}
}

