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

package gwsvc

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwaudit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

// TestConfigUpdateRecordNamesTheConfigWithoutARename is the regression for a
// record that could not identify what it described.
//
// req.Name is empty on any update that is not a rename, and the record was
// built from the request — so a description-only or mappings-only update
// produced a record whose resource name and configName were both "". "Someone
// changed a configuration" is not an answer; the record has to say which.
func TestConfigUpdateRecordNamesTheConfigWithoutARename(t *testing.T) {
	ctx, sink, flush := capturingAuditCtx(t)

	svc := &agentConfigurationService{logger: discardLogger()}
	configUUID := uuid.New()
	existing := &gwmodels.AgentConfiguration{UUID: configUUID, Name: "production-llm"}

	svc.recordConfigUpdate(ctx, configUUID, "ou-1", "proj", "agent",
		existing, gwmodels.UpdateAgentModelConfigRequest{Description: "new description"})

	flush()

	e, ok := findEvent(sink.captured(), gwaudit.AgentConfigUpdate)
	require.True(t, ok, "a successful configuration update must be recorded")
	require.Equal(t, "production-llm", e.ResourceName,
		"resource name must fall back to the stored name when the request is not a rename")
	require.Equal(t, "production-llm", e.Details["configName"],
		"configName must fall back to the stored name when the request is not a rename")
	require.Equal(t, []string{"description"}, e.Details["updatedFields"])
}

// TestConfigUpdateRecordPrefersTheNewNameOnRename keeps the fallback from
// hiding a rename: when the request does carry a name, that is the name the
// configuration now has, and it is what the record must show.
func TestConfigUpdateRecordPrefersTheNewNameOnRename(t *testing.T) {
	ctx, sink, flush := capturingAuditCtx(t)

	svc := &agentConfigurationService{logger: discardLogger()}
	configUUID := uuid.New()
	existing := &gwmodels.AgentConfiguration{UUID: configUUID, Name: "old-name"}

	svc.recordConfigUpdate(ctx, configUUID, "ou-1", "proj", "agent",
		existing, gwmodels.UpdateAgentModelConfigRequest{Name: "new-name"})

	flush()

	e, ok := findEvent(sink.captured(), gwaudit.AgentConfigUpdate)
	require.True(t, ok)
	require.Equal(t, "new-name", e.ResourceName)
	require.Equal(t, []string{"name"}, e.Details["updatedFields"])
}

// TestConfigUpdateRecordListsEveryChangedField pins updatedFields to what the
// request actually changed. environmentVariables was absent from the list, so a
// record could report an update that named none of what it altered.
func TestConfigUpdateRecordListsEveryChangedField(t *testing.T) {
	ctx, sink, flush := capturingAuditCtx(t)

	svc := &agentConfigurationService{logger: discardLogger()}
	configUUID := uuid.New()
	existing := &gwmodels.AgentConfiguration{UUID: configUUID, Name: "cfg"}

	svc.recordConfigUpdate(ctx, configUUID, "ou-1", "proj", "agent", existing,
		gwmodels.UpdateAgentModelConfigRequest{
			Name:                 "cfg2",
			Description:          "d",
			EnvMappings:          map[string]gwmodels.EnvModelConfigRequest{},
			EnvironmentVariables: []gwmodels.EnvironmentVariableConfig{{}},
		})

	flush()

	e, ok := findEvent(sink.captured(), gwaudit.AgentConfigUpdate)
	require.True(t, ok)
	require.ElementsMatch(t,
		[]string{"name", "description", "envMappings", "environmentVariables"},
		e.Details["updatedFields"],
		"every changed field must appear; environmentVariables was previously omitted")
}

// TestConfigUpdateRecordSurvivesAMissingExistingConfig guards the fallback
// itself: recording must never panic on the path it exists to serve.
func TestConfigUpdateRecordSurvivesAMissingExistingConfig(t *testing.T) {
	ctx, sink, flush := capturingAuditCtx(t)

	svc := &agentConfigurationService{logger: discardLogger()}
	require.NotPanics(t, func() {
		svc.recordConfigUpdate(ctx, uuid.New(), "ou-1", "proj", "agent",
			nil, gwmodels.UpdateAgentModelConfigRequest{Description: "d"})
	})

	flush()

	_, ok := findEvent(sink.captured(), gwaudit.AgentConfigUpdate)
	require.True(t, ok, "a record must still be emitted, even unnamed")
}
