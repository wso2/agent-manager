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

package clierr

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestNew_AdditionalDataMarshalsAsEmptyObject(t *testing.T) {
	raw, err := json.Marshal(New(NotFound, "no such agent"))
	if err != nil {
		t.Fatal(err)
	}
	// Downstream tools depend on exactly this shape.
	want := `{"status":0,"code":"NOT_FOUND","message":"no such agent","reason":null,"additionalData":{}}`
	if string(raw) != want {
		t.Errorf("wire form = %s\nwant        %s", raw, want)
	}
}

func TestNewf_FormatsMessage(t *testing.T) {
	e := Newf(NoOrg, "org %q missing (%d)", "acme", 3)
	if e.Code != NoOrg || e.Message != `org "acme" missing (3)` {
		t.Errorf("got %+v", e)
	}
}

func TestCLIError_ErrorAndExtraction(t *testing.T) {
	var err error = New(Forbidden, "denied")
	if err.Error() != "FORBIDDEN: denied" {
		t.Errorf("Error() = %q", err.Error())
	}
	wrapped := fmt.Errorf("while deleting: %w", err)
	var ce CLIError
	if !errors.As(wrapped, &ce) || ce.Code != Forbidden {
		t.Errorf("errors.As through a wrap failed: %v", wrapped)
	}
}

// Codes are a wire contract: renaming one breaks callers that match on it.
func TestCodes_AreStable(t *testing.T) {
	for got, want := range map[string]string{
		Transport: "CLI_TRANSPORT", AuthTokenExpired: "AUTH_TOKEN_EXPIRED", AuthRefreshFailed: "AUTH_REFRESH_FAILED",
		ConfigNotLoaded: "CONFIG_NOT_LOADED", ConfigSaveFailed: "CONFIG_SAVE_FAILED", NoInstance: "NO_INSTANCE",
		NoOrg: "NO_ORG", NoProject: "NO_PROJECT", NoAgent: "NO_AGENT", NoEnvironment: "NO_ENVIRONMENT",
		NotLinked: "NOT_LINKED", Internal: "INTERNAL", ConfirmationRequired: "CONFIRMATION_REQUIRED",
		InvalidFlag: "INVALID_FLAG", Unauthorized: "UNAUTHORIZED", Forbidden: "FORBIDDEN", ServerError: "SERVER_ERROR",
		ServerInvalid: "SERVER_RESPONSE_INVALID", NotFound: "NOT_FOUND", Validation: "VALIDATION",
		BuildNotDeployable: "BUILD_NOT_DEPLOYABLE", SkillInstallFailed: "SKILL_INSTALL_FAILED",
		SkillRemoveFailed: "SKILL_REMOVE_FAILED", SkillListFailed: "SKILL_LIST_FAILED", FeatureUnavailable: "FEATURE_UNAVAILABLE",
	} {
		if got != want {
			t.Errorf("code %q changed from %q", got, want)
		}
	}
}
