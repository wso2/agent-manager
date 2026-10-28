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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// The workflow kind recorded on a Component comes from
// OPEN_CHOREO_BUILD_WORKFLOW_KIND. A ClusterWorkflow pins workflowPlaneRef to the
// single cluster-wide ClusterWorkflowPlane, so its builds always run on that one
// plane; a namespaced Workflow resolves the WorkflowPlane inside the org's own
// namespace, which is what lets the org build on the data centre it belongs to.
//
// The kind has to be recorded here, at creation, because builds.go reads it from
// the Component and only falls back to ClusterWorkflow when it is absent. Left
// unset (the on-premises default) nothing is recorded, exactly as before the
// setting existed.
func TestBuildComponentRequest_RecordsConfiguredWorkflowKind(t *testing.T) {
	builds := []struct {
		name         string
		build        *BuildConfig
		wantWorkflow string
	}{
		{
			name:         "docker build",
			build:        &BuildConfig{Type: BuildTypeDocker, Docker: &DockerConfig{DockerfilePath: "Dockerfile"}},
			wantWorkflow: WorkflowNameDocker,
		},
		{
			name:         "google buildpack",
			build:        &BuildConfig{Type: BuildTypeBuildpack, Buildpack: &BuildpackConfig{Language: "python"}},
			wantWorkflow: WorkflowNameGoogleCloudBuildpacks,
		},
		{
			name:         "ballerina buildpack",
			build:        &BuildConfig{Type: BuildTypeBuildpack, Buildpack: &BuildpackConfig{Language: "ballerina"}},
			wantWorkflow: WorkflowNameBallerinaBuilpack,
		},
	}
	kinds := []struct {
		setting  string
		wantKind *gen.ComponentWorkflowConfigKind
	}{
		{setting: "", wantKind: nil},
		{setting: "Workflow", wantKind: ptrKind(gen.ComponentWorkflowConfigKindWorkflow)},
		{setting: "ClusterWorkflow", wantKind: ptrKind(gen.ComponentWorkflowConfigKindClusterWorkflow)},
	}

	cfg := config.GetConfig()
	orig := cfg.OpenChoreo.BuildWorkflowKind
	t.Cleanup(func() { cfg.OpenChoreo.BuildWorkflowKind = orig })

	for _, k := range kinds {
		for _, tc := range builds {
			t.Run(fmt.Sprintf("kind=%q/%s", k.setting, tc.name), func(t *testing.T) {
				cfg.OpenChoreo.BuildWorkflowKind = k.setting
				body, err := buildInternalAgentFromSourceComponentRequestBody("default", "proj1", CreateComponentRequest{
					Name:             "my-agent",
					DisplayName:      "My Agent",
					ProvisioningType: ProvisioningInternal,
					AgentType:        AgentTypeConfig{Type: string(utils.AgentTypeAPI), SubType: "custom-api"},
					Build:            tc.build,
					Repository:       &RepositoryConfig{},
				})
				require.NoError(t, err)
				require.NotNil(t, body.Spec)
				require.NotNil(t, body.Spec.Workflow)
				assert.Equal(t, k.wantKind, body.Spec.Workflow.Kind)
				assert.Equal(t, tc.wantWorkflow, body.Spec.Workflow.Name,
					"the workflow name is unchanged by the kind — the namespaced "+
						"workflows are provisioned under the same names")
			})
		}
	}
}

func ptrKind(k gen.ComponentWorkflowConfigKind) *gen.ComponentWorkflowConfigKind { return &k }
