//
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

package client

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
)

func TestComponentImageOwnershipComesFromWorkflow(t *testing.T) {
	for _, tc := range []struct {
		name     string
		workflow *gen.ComponentWorkflowConfig
		want     bool
	}{
		{name: "kind or direct image"},
		{name: "empty workflow", workflow: &gen.ComponentWorkflowConfig{}},
		{name: "source build", workflow: &gen.ComponentWorkflowConfig{Name: "amp-docker"}, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			comp := &gen.Component{Metadata: gen.ObjectMeta{Name: "agent"}, Spec: &gen.ComponentSpec{Workflow: tc.workflow}}
			agent, err := convertComponentFromTyped(comp)
			require.NoError(t, err)
			require.Equal(t, tc.want, agent.HasBuildWorkflow)
			body, err := json.Marshal(agent)
			require.NoError(t, err)
			require.False(t, strings.Contains(string(body), "HasBuildWorkflow"), "ownership evidence must stay internal")
		})
	}
}
