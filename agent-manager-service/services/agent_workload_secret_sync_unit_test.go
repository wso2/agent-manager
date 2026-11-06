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
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// TestUpdateAgentConfigurations_WaitsForEditedWorkloadSecrets checks save, sync, and rollout ordering.
func TestUpdateAgentConfigurations_WaitsForEditedWorkloadSecrets(t *testing.T) {
	const (
		envKey  = "OPENAI_API_KEY"
		fileKey = "credentials.json"
		refName = "my-agent-dev-secrets"
	)
	syncErr := errors.New("ESO sync unconfirmed")
	for _, tc := range []struct {
		name         string
		key          string
		existingKeys []string
		file         bool
		syncErr      error
		wantEvents   []string
	}{
		{name: "existing env key", key: envKey, existingKeys: []string{envKey}, wantEvents: []string{"save", "sync", "rollout"}},
		{name: "new env key", key: envKey, existingKeys: []string{"removed-key"}, wantEvents: []string{"save", "rollout"}},
		{name: "existing file key", key: fileKey, existingKeys: []string{fileKey}, file: true, wantEvents: []string{"save", "sync", "rollout"}},
		{name: "failed sync blocks rollout", key: envKey, existingKeys: []string{envKey}, syncErr: syncErr, wantEvents: []string{"save", "sync"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var events []string
			secrets := &clientmocks.SecretManagementClientMock{
				GetSecretFunc: func(context.Context, string) (*secretmanagersvc.SecretInfo, error) {
					return &secretmanagersvc.SecretInfo{Keys: tc.existingKeys}, nil
				},
				PatchSecretFunc: func(_ context.Context, _ secretmanagersvc.SecretLocation, values map[string]string, _ []string) (string, error) {
					require.Equal(t, map[string]string{tc.key: "dummy"}, values)
					events = append(events, "save")
					return refName, nil
				},
			}
			oc := &clientmocks.OpenChoreoClientMock{
				GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
					return &models.OrganizationResponse{Name: name}, nil
				},
				GetComponentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
					return &models.AgentResponse{Provisioning: models.Provisioning{Type: string(utils.InternalAgent)}}, nil
				},
				GetEnvironmentFunc: nonProductionEnvStub(),
				GetComponentConfigurationsFunc: func(context.Context, string, string, string, string) ([]models.EnvVars, error) {
					return []models.EnvVars{}, nil
				},
				SyncEditedWorkloadSecretsFunc: func(_ context.Context, org, agent, env, ref string, keys []string) error {
					require.Equal(t, []string{"acme", "my-agent", "dev", refName}, []string{org, agent, env, ref})
					require.Equal(t, []string{tc.key}, keys)
					events = append(events, "sync")
					return tc.syncErr
				},
				ReplaceReleaseBindingWorkloadOverridesFunc: func(context.Context, string, string, string, []client.EnvVar, []client.FileVar) error {
					events = append(events, "rollout")
					return nil
				},
			}
			injector := &agentIdentityInjectorStub{
				EnvVarsForEnvironmentFunc: func(context.Context, string, string, string, string) ([]client.EnvVar, error) {
					return []client.EnvVar{}, nil
				},
			}
			svc := NewAgentManagerService(nil, oc, secrets, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
				injector, nil, nil, nil, nil, nil, nil, nil, nil, discardLogger())
			req := &spec.UpdateAgentConfigurationsRequest{EnvironmentName: "dev", Env: []spec.EnvironmentVariable{}}
			if tc.file {
				file := spec.FileMount{Key: tc.key, MountPath: "/etc/credentials.json"}
				file.SetIsSensitive(true)
				file.SetValue("dummy")
				req.Files = []spec.FileMount{file}
			} else {
				env := spec.EnvironmentVariable{Key: tc.key}
				env.SetIsSensitive(true)
				env.SetValue("dummy")
				req.Env = append(req.Env, env)
			}

			err := svc.UpdateAgentConfigurations(tierGrantedCtx(t), "acme", "proj1", "my-agent", req)
			if tc.syncErr != nil {
				require.ErrorIs(t, err, utils.ErrWorkloadSecretSyncUnconfirmed)
				require.ErrorIs(t, err, tc.syncErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantEvents, events)
		})
	}
}
