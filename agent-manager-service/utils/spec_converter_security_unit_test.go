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
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

func TestSecurityConfigConversionPreservesIdentity(t *testing.T) {
	enabled, disabled := true, false

	for _, tc := range []struct {
		name     string
		identity *spec.IdentitySecurity
	}{
		{name: "enabled", identity: &spec.IdentitySecurity{Enabled: &enabled}},
		{name: "disabled", identity: &spec.IdentitySecurity{Enabled: &disabled}},
		{name: "enabled omitted", identity: &spec.IdentitySecurity{}},
		{name: "identity omitted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := spec.SecurityConfig{Enabled: &enabled, Identity: tc.identity}
			assert.Equal(t, input, ConvertModelToSpecSecurityConfig(ConvertSpecToModelSecurityConfig(input)))
		})
	}
}
