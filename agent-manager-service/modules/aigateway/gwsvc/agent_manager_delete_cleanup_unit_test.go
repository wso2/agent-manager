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
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
)

// stubAgentConfigurationService embeds the interface so only the two methods the
// cleanup path uses need stubbing; any other call panics on the nil interface,
// which is what makes an unexpected code path fail loudly.
type stubAgentConfigurationService struct {
	AgentConfigurationService

	ListFunc                   func(ctx context.Context, ouID, projectName, agentName string, limit, offset int) (*models.AgentModelConfigListResponse, error)
	DeleteForAgentDeletionFunc func(ctx context.Context, configUUID uuid.UUID, ouID, projectName, agentName string, isExternalAgent bool) error
}

func (s *stubAgentConfigurationService) List(
	ctx context.Context, ouID, projectName, agentName string, limit, offset int,
) (*models.AgentModelConfigListResponse, error) {
	return s.ListFunc(ctx, ouID, projectName, agentName, limit, offset)
}

func (s *stubAgentConfigurationService) DeleteForAgentDeletion(
	ctx context.Context, configUUID uuid.UUID, ouID, projectName, agentName string, isExternalAgent bool,
) error {
	return s.DeleteForAgentDeletionFunc(ctx, configUUID, ouID, projectName, agentName, isExternalAgent)
}

// newAIAppServiceForCleanup builds a real AIApplicationService over mocked repos that
// report no applications, so the cleanup's final step is a no-op.
func newAIAppServiceForCleanup() *AIApplicationService {
	appRepo := &gwrepomocks.AIApplicationRepositoryMock{
		ListByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.AIApplication, error) {
			return []models.AIApplication{}, nil
		},
		DeleteByAgentFunc: func(_ context.Context, _, _, _ string) error { return nil },
	}
	return NewAIApplicationService(appRepo, &gwrepomocks.GatewayRepositoryMock{}, nil, discardLogger())
}

// TestRevokeAgentConfigurations covers what keeps LLM proxy credentials from surviving an
// agent deletion: every configuration the agent owns is handed to DeleteForAgentDeletion once,
// a failure on one does not abandon the rest but is reported so the teardown is retried, and
// the per-agent AI application records go only once every configuration is gone.
func TestRevokeAgentConfigurations(t *testing.T) {
	const org, proj, agent = "acme", "proj1", "chat-agent"

	newSvc := func(cfgSvc AgentConfigurationService) *AgentGatewayService {
		return &AgentGatewayService{
			agentConfigurationService: cfgSvc,
			aiApplicationService:      newAIAppServiceForCleanup(),
			logger:                    discardLogger(),
		}
	}

	listOf := func(uuids ...string) func(context.Context, string, string, string, int, int) (*models.AgentModelConfigListResponse, error) {
		items := make([]models.AgentModelConfigListItem, len(uuids))
		for i, u := range uuids {
			items[i] = models.AgentModelConfigListItem{UUID: u, Name: "cfg-" + u, Type: "llm"}
		}
		return func(_ context.Context, _, _, _ string, _, _ int) (*models.AgentModelConfigListResponse, error) {
			return &models.AgentModelConfigListResponse{Configs: items, Pagination: models.PaginationInfo{Count: len(items)}}, nil
		}
	}

	t.Run("revokes every configuration the agent owns", func(t *testing.T) {
		first, second := uuid.NewString(), uuid.NewString()
		var revoked []string
		cfgSvc := &stubAgentConfigurationService{
			ListFunc: listOf(first, second),
			DeleteForAgentDeletionFunc: func(_ context.Context, got uuid.UUID, gotOU, gotProj, gotAgent string, external bool) error {
				assert.Equal(t, org, gotOU)
				assert.Equal(t, proj, gotProj)
				assert.Equal(t, agent, gotAgent)
				assert.True(t, external, "Lite registers external agents only")
				revoked = append(revoked, got.String())
				return nil
			},
		}
		require.NoError(t, newSvc(cfgSvc).revokeAgentConfigurations(context.Background(), org, proj, agent))
		assert.ElementsMatch(t, []string{first, second}, revoked)
	})

	t.Run("reports a failure after trying the rest", func(t *testing.T) {
		doomed, healthy := uuid.NewString(), uuid.NewString()
		attempts := map[string]int{}
		cfgSvc := &stubAgentConfigurationService{
			ListFunc: listOf(doomed, healthy),
			DeleteForAgentDeletionFunc: func(_ context.Context, got uuid.UUID, _, _, _ string, _ bool) error {
				attempts[got.String()]++
				if got.String() == doomed {
					return errors.New("external cleanup incomplete, DB record preserved for retry")
				}
				return nil
			},
		}
		err := newSvc(cfgSvc).revokeAgentConfigurations(context.Background(), org, proj, agent)
		require.Error(t, err, "the teardown is retried")
		assert.Equal(t, 1, attempts[doomed])
		assert.Equal(t, 1, attempts[healthy])
	})

	t.Run("reports a configuration whose UUID cannot be parsed", func(t *testing.T) {
		cfgSvc := &stubAgentConfigurationService{ListFunc: listOf("not-a-uuid")}
		require.Error(t, newSvc(cfgSvc).revokeAgentConfigurations(context.Background(), org, proj, agent))
	})

	t.Run("reports a list failure", func(t *testing.T) {
		cfgSvc := &stubAgentConfigurationService{
			ListFunc: func(_ context.Context, _, _, _ string, _, _ int) (*models.AgentModelConfigListResponse, error) {
				return nil, errors.New("db unavailable")
			},
		}
		require.Error(t, newSvc(cfgSvc).revokeAgentConfigurations(context.Background(), org, proj, agent))
	})
}
