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
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// renameVarRows builds one environment's url + apikey rows under the pre-rename names.
func renameVarRows(configUUID, envUUID uuid.UUID, secretRef string) []models.AgentEnvConfigVariable {
	return []models.AgentEnvConfigVariable{
		{ConfigUUID: configUUID, EnvironmentUUID: envUUID, VariableKey: "url", VariableName: "GITHUB_MCP_URL"},
		{ConfigUUID: configUUID, EnvironmentUUID: envUUID, VariableKey: "apikey", VariableName: "GITHUB_MCP_API_KEY", SecretReference: secretRef},
	}
}

// Inject a transaction completion failure without a live DB. Repository behavior
// remains mocked; the service still executes GORM's real transaction callback and commit.
type mcpRenameCommitPool struct {
	gorm.ConnPool
	tx     *mcpRenameCommitTx
	begins int
}

func (p *mcpRenameCommitPool) BeginTx(context.Context, *sql.TxOptions) (gorm.ConnPool, error) {
	p.begins++
	if p.begins > 1 {
		return &mcpRenameCommitTx{onCommit: func() {}}, nil
	}
	return p.tx, nil
}

type mcpRenameCommitTx struct {
	gorm.ConnPool
	commitErr error
	onCommit  func()
	commits   int
	rollbacks int
}

func (tx *mcpRenameCommitTx) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return driver.RowsAffected(1), nil
}

func (tx *mcpRenameCommitTx) Commit() error {
	tx.commits++
	tx.onCommit()
	return tx.commitErr
}

func (tx *mcpRenameCommitTx) Rollback() error {
	tx.rollbacks++
	return nil
}

func TestRenamedMCPProxyURL_ConfirmedMissingEndpointIsEmpty(t *testing.T) {
	f := newMCPURLTestFixture()
	f.mapping.MCPProxy.Endpoints = nil
	svc := f.newService()
	svc.mcpProxyRepo = &repomocks.MCPProxyRepositoryMock{GetByUUIDFunc: func(context.Context, string, string) (*models.MCPProxy, error) { return f.mapping.MCPProxy, nil }}
	url, err := svc.renamedMCPProxyURL(context.Background(), &models.AgentConfiguration{Name: "github"}, &f.mapping, "default", f.ouID)
	require.NoError(t, err)
	require.Empty(t, url)
}

func TestUpdateMCPConfigEnvironmentVariableNames_RejectsOtherConfigOwnership(t *testing.T) {
	for _, scenario := range []string{"collision", "ownership read fails", "own names allowed"} {
		t.Run(scenario, func(t *testing.T) {
			f := newMCPURLTestFixture()
			configID := uuid.New()
			tx := &mcpRenameCommitTx{onCommit: func() {}}
			database, err := gorm.Open(postgres.New(postgres.Config{Conn: &mcpRenameCommitPool{tx: tx}}), &gorm.Config{DisableAutomaticPing: true})
			require.NoError(t, err)
			svc := f.newService()
			svc.db = database
			readErr := errors.New("ownership read failed")
			locked := false
			repo := &repomocks.AgentEnvConfigVariableRepositoryMock{
				LockAndListByAgentFunc: func(_ context.Context, _ *gorm.DB, org, project, agent string) ([]models.AgentEnvConfigVariable, error) {
					require.Equal(t, []string{f.ouID, "project", "agent"}, []string{org, project, agent})
					locked = true
					if scenario == "ownership read fails" {
						return nil, readErr
					}
					owner := uuid.New()
					if scenario == "own names allowed" {
						owner = configID
					}
					return []models.AgentEnvConfigVariable{{ConfigUUID: owner, VariableName: "GITHUB_URL"}}, nil
				},
				ListByConfigForUpdateFunc: func(context.Context, *gorm.DB, uuid.UUID) ([]models.AgentEnvConfigVariable, error) {
					require.True(t, locked, "agent ownership lock must precede row locks")
					return renameVarRows(configID, f.envUUID, "secret"), nil
				},
			}
			svc.envVariableRepo = repo
			if scenario == "own names allowed" {
				repo.UpdateVariableNamesFunc = func(context.Context, *gorm.DB, uuid.UUID, map[string]string) error { return nil }
			}
			// No client methods are configured: rejected requests must never write live values.
			svc.ocClient = &repomocks.PlatformRepositoryMock{}
			policy := platformAgentPolicy
			if scenario == "own names allowed" {
				policy = externalAgentPolicy
			}
			err = svc.updateMCPConfigEnvironmentVariableNames(context.Background(), &models.AgentConfiguration{UUID: configID, Name: "github"}, f.ouID, "project", "agent", nil, policy, "default", []models.EnvironmentVariableConfig{{Key: "url", Name: "GITHUB_URL"}})
			switch scenario {
			case "collision":
				require.ErrorIs(t, err, utils.ErrInvalidInput)
			case "ownership read fails":
				require.ErrorIs(t, err, readErr)
				require.NotErrorIs(t, err, utils.ErrInvalidInput)
			default:
				require.NoError(t, err)
			}
			if scenario != "own names allowed" {
				require.Empty(t, repo.UpdateVariableNamesCalls())
				require.Zero(t, tx.commits)
			}
		})
	}
}
