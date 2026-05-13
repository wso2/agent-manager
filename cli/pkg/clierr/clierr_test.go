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

import "testing"

func TestSkillErrorCodes(t *testing.T) {
	tests := []struct {
		code string
		want string
	}{
		{SkillInstallFailed, "SKILL_INSTALL_FAILED"},
		{SkillRemoveFailed, "SKILL_REMOVE_FAILED"},
		{SkillListFailed, "SKILL_LIST_FAILED"},
	}
	for _, tt := range tests {
		if tt.code != tt.want {
			t.Errorf("code = %q, want %q", tt.code, tt.want)
		}
	}
}
