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

package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
)

// TestBuildTraitEnvConfigsOmitsAPIManagementForA2A pins the rule that an agent
// whose api-configuration trait is not attached must not carry a
// traitEnvironmentConfigs key naming it: OpenChoreo resolves those keys against
// attached trait instances, so a dangling key is config for a trait that is not
// there.
func TestBuildTraitEnvConfigsOmitsAPIManagementForA2A(t *testing.T) {
	apiKey := "my-agent-" + string(client.TraitAPIManagement)

	withTrait := buildTraitEnvConfigs("my-agent", nil, "", 0, false, false, true, "", true)
	require.Contains(t, withTrait, apiKey, "a REST agent still gets its api-configuration key")

	withoutTrait := buildTraitEnvConfigs("my-agent", nil, "", 0, false, false, true, "", false)
	assert.NotContains(t, withoutTrait, apiKey, "an a2a agent must not carry a key for a detached trait")
}
