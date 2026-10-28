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
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
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
	appRepo := &repomocks.AIApplicationRepositoryMock{
		ListByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.AIApplication, error) {
			return []models.AIApplication{}, nil
		},
		DeleteByAgentFunc: func(_ context.Context, _, _, _ string) error { return nil },
	}
	return NewAIApplicationService(appRepo, &repomocks.GatewayRepositoryMock{}, nil, discardLogger())
}

// shrinkCleanupRetryDelay drops the backoff for the duration of a test so a case that
// exhausts the attempt budget costs microseconds instead of the production ~14s.
func shrinkCleanupRetryDelay(t *testing.T) {
	t.Helper()
	original := agentConfigCleanupRetryDelay
	agentConfigCleanupRetryDelay = time.Microsecond
	t.Cleanup(func() { agentConfigCleanupRetryDelay = original })
}

// TestDeleteAgentLLMConfigurations covers what keeps LLM proxy credentials from surviving an
// agent deletion: every configuration the agent owns is handed to DeleteForAgentDeletion, a
// failure on one does not abandon the rest, and the per-agent AI application records are
// removed once the loop has run.
func TestDeleteAgentLLMConfigurations(t *testing.T) {
	const org, proj, agent = "acme", "proj1", "chat-agent"

	newSvc := func(cfgSvc AgentConfigurationService) *agentManagerService {
		return &agentManagerService{
			agentConfigurationService: cfgSvc,
			aiApplicationService:      newAIAppServiceForCleanup(),
			logger:                    discardLogger(),
		}
	}

	listOf := func(uuids ...string) func(context.Context, string, string, string, int, int) (*models.AgentModelConfigListResponse, error) {
		items := make([]models.AgentModelConfigListItem, len(uuids))
		for i, u := range uuids {
			items[i] = models.AgentModelConfigListItem{UUID: u, Type: "llm"}
		}
		return func(_ context.Context, _, _, _ string, _, _ int) (*models.AgentModelConfigListResponse, error) {
			return &models.AgentModelConfigListResponse{
				Configs:    items,
				Pagination: models.PaginationInfo{Count: len(items)},
			}, nil
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
				assert.False(t, external)
				revoked = append(revoked, got.String())
				return nil
			},
		}

		newSvc(cfgSvc).deleteAgentLLMConfigurations(context.Background(), org, proj, agent, false)

		assert.ElementsMatch(t, []string{first, second}, revoked)
	})

	t.Run("keeps revoking the remaining configurations after one exhausts its retries", func(t *testing.T) {
		shrinkCleanupRetryDelay(t)
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

		newSvc(cfgSvc).deleteAgentLLMConfigurations(context.Background(), org, proj, agent, false)

		// A proxy whose credential cannot be revoked must not cost the other configs their
		// cleanup — each one names a separate live credential.
		assert.Equal(t, agentConfigCleanupAttempts, attempts[doomed])
		assert.Equal(t, 1, attempts[healthy])
	})

	t.Run("skips a configuration whose UUID cannot be parsed", func(t *testing.T) {
		cfgSvc := &stubAgentConfigurationService{
			ListFunc: listOf("not-a-uuid"),
			// DeleteForAgentDeletionFunc left nil: calling it would panic, which is the
			// assertion — an unparseable UUID must not reach the teardown.
		}

		newSvc(cfgSvc).deleteAgentLLMConfigurations(context.Background(), org, proj, agent, false)
	})

	t.Run("does nothing when the configuration list cannot be read", func(t *testing.T) {
		cfgSvc := &stubAgentConfigurationService{
			ListFunc: func(_ context.Context, _, _, _ string, _, _ int) (*models.AgentModelConfigListResponse, error) {
				return nil, errors.New("db unavailable")
			},
		}

		newSvc(cfgSvc).deleteAgentLLMConfigurations(context.Background(), org, proj, agent, false)
	})
}

// TestCheckNoOrphanedConfigs covers the create-time guard for the credential-reuse path:
// agent_configurations are keyed by agent *name*, and DeleteForAgentDeletion deliberately keeps
// a row whose revocation failed, so a new agent taking a deleted agent's name would otherwise
// adopt that row's live LLM proxy credential.
func TestCheckNoOrphanedConfigs(t *testing.T) {
	const org, proj, agent = "acme", "proj1", "chat-agent"

	newSvc := func(cfgSvc AgentConfigurationService) *agentManagerService {
		return &agentManagerService{agentConfigurationService: cfgSvc, logger: discardLogger()}
	}

	t.Run("allows a name with no leftover configurations", func(t *testing.T) {
		cfgSvc := &stubAgentConfigurationService{
			ListFunc: func(_ context.Context, gotOU, gotProj, gotAgent string, _, _ int) (*models.AgentModelConfigListResponse, error) {
				assert.Equal(t, org, gotOU)
				assert.Equal(t, proj, gotProj)
				assert.Equal(t, agent, gotAgent)
				return &models.AgentModelConfigListResponse{Pagination: models.PaginationInfo{Count: 0}}, nil
			},
		}

		require.NoError(t, newSvc(cfgSvc).checkNoOrphanedConfigs(context.Background(), org, proj, agent))
	})

	t.Run("refuses a name still holding a deleted agent's configurations", func(t *testing.T) {
		cfgSvc := &stubAgentConfigurationService{
			ListFunc: func(_ context.Context, _, _, _ string, _, _ int) (*models.AgentModelConfigListResponse, error) {
				return &models.AgentModelConfigListResponse{
					Configs:    []models.AgentModelConfigListItem{{UUID: uuid.NewString(), Type: "llm"}},
					Pagination: models.PaginationInfo{Count: 2},
				}, nil
			},
		}

		err := newSvc(cfgSvc).checkNoOrphanedConfigs(context.Background(), org, proj, agent)

		require.ErrorIs(t, err, utils.ErrOrphanedAgentConfigsExist)
		assert.Contains(t, err.Error(), agent)
	})

	t.Run("refuses the create when the check itself cannot run", func(t *testing.T) {
		cfgSvc := &stubAgentConfigurationService{
			ListFunc: func(_ context.Context, _, _, _ string, _, _ int) (*models.AgentModelConfigListResponse, error) {
				return nil, errors.New("db unavailable")
			},
		}

		// Failing open here would reintroduce the very reuse this guard exists to stop.
		err := newSvc(cfgSvc).checkNoOrphanedConfigs(context.Background(), org, proj, agent)

		require.Error(t, err)
		assert.NotErrorIs(t, err, utils.ErrOrphanedAgentConfigsExist)
	})
}

// TestRetryCleanup covers the bounded retry that fronts each configuration
// teardown. Revocation fails under cluster load — a transient condition — and every step of
// DeleteForAgentDeletion tolerates an already-gone resource, so re-running it converges.
func TestRetryCleanup(t *testing.T) {
	t.Run("returns immediately on first success without sleeping", func(t *testing.T) {
		calls := 0
		start := time.Now()

		err := retryCleanup(context.Background(), discardLogger(), agentConfigCleanupRetry(), "Configuration teardown", func() error {
			calls++
			return nil
		})

		require.NoError(t, err)
		assert.Equal(t, 1, calls)
		assert.Less(t, time.Since(start), agentConfigCleanupRetryDelay, "a first-attempt success must not back off")
	})

	t.Run("retries a transient failure and reports success", func(t *testing.T) {
		shrinkCleanupRetryDelay(t)
		calls := 0

		err := retryCleanup(context.Background(), discardLogger(), agentConfigCleanupRetry(), "Configuration teardown", func() error {
			calls++
			if calls < 2 {
				return errors.New("gateway unavailable")
			}
			return nil
		})

		require.NoError(t, err)
		assert.Equal(t, 2, calls, "should have succeeded on the second attempt")
	})

	t.Run("gives up after the attempt budget and surfaces the last error", func(t *testing.T) {
		shrinkCleanupRetryDelay(t)
		calls := 0
		wantErr := errors.New("proxy API key revocation failed")

		err := retryCleanup(context.Background(), discardLogger(), agentConfigCleanupRetry(), "Configuration teardown", func() error {
			calls++
			return wantErr
		})

		require.ErrorIs(t, err, wantErr)
		assert.Equal(t, agentConfigCleanupAttempts, calls, "should spend exactly the attempt budget")
	})

	t.Run("stops backing off when the context is cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		start := time.Now()

		err := retryCleanup(ctx, discardLogger(), agentConfigCleanupRetry(), "Configuration teardown", func() error {
			calls++
			cancel()
			return errors.New("gateway unavailable")
		})

		// Cancellation must cut the wait short rather than sleeping out the full budget,
		// so a shutting-down process is not held open by a doomed retry loop.
		require.Error(t, err)
		assert.Equal(t, 1, calls)
		assert.Less(t, time.Since(start), agentConfigCleanupRetryDelay)
	})

	t.Run("does not sleep after the final attempt", func(t *testing.T) {
		done := make(chan error, 1)
		go func() {
			done <- retryCleanup(context.Background(), discardLogger(), fixedCleanupRetry(1, time.Hour), "Teardown",
				func() error { return errors.New("boom") })
		}()

		select {
		case err := <-done:
			require.Error(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("retryCleanup slept after its final attempt")
		}
	})
}
