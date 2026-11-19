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

package gwrepo

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

// The due-scan and the API both filter on these exact strings.
func TestA2AAgentCardTableAndEnums(t *testing.T) {
	assert.Equal(t, "mod_ai_gateway_agent_cards", gwmodels.A2AAgentCard{}.TableName())
	assert.Equal(t, gwmodels.A2AAgentCardStatus("pending"), gwmodels.A2AAgentCardStatusPending)
	assert.Equal(t, gwmodels.A2AAgentCardStatus("fetched"), gwmodels.A2AAgentCardStatusFetched)
	assert.Equal(t, gwmodels.A2AAgentCardStatus("failed"), gwmodels.A2AAgentCardStatusFailed)
	assert.Equal(t, gwmodels.A2AAgentCardSource("platform"), gwmodels.A2AAgentCardSourcePlatform)
	assert.Equal(t, gwmodels.A2AAgentCardSource("external"), gwmodels.A2AAgentCardSourceExternal)
}
