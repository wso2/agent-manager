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
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const inventoryCSV = "id,name,status,Owner Email,team\n" +
	"bot-1,Support bot,active,jane@example.com,support\n" +
	",No id,,,\n" +
	"bot-1,Repeated,,,\n" +
	"bot-2,HR bot,,,\n"

// inventoryRepo is a repository with one connection that records the saved
// scan.
func inventoryRepo(c *models.ShadowConnection) *repomocks.ShadowAgentRepositoryMock {
	return &repomocks.ShadowAgentRepositoryMock{
		GetConnectionFunc: func(_ context.Context, ou string, id uuid.UUID) (*models.ShadowConnection, error) {
			if ou != c.OUID || id != c.ID {
				return nil, gorm.ErrRecordNotFound
			}
			return c, nil
		},
		SaveUploadedScanFunc: func(_ context.Context, scan *models.ShadowScan, in repositories.ShadowConnectionApply) (int, error) {
			scan.FindingsCount = len(in.Findings)
			scan.Results[0].FindingsCount = len(in.Findings)
			return len(in.Findings), nil
		},
		ListOrgFindingsFunc: func(context.Context, string) ([]models.ShadowFinding, error) { return nil, nil },
		ApplyCorrelationFunc: func(context.Context, string, map[uuid.UUID][]models.ShadowFindingRelation,
			map[uuid.UUID][]models.ShadowFindingSignal,
		) error {
			return nil
		},
	}
}

func inventoryConnection() *models.ShadowConnection {
	return &models.ShadowConnection{ID: uuid.New(), OUID: shadowTestOU, Name: "Inventory", Provider: models.ShadowProviderCustom}
}

// An uploaded file is saved as a finished, complete scan of its connection:
// the valid agents, one per id, with the upload as their evidence.
func TestShadowUploadInventory_SavesACompleteScan(t *testing.T) {
	c := inventoryConnection()
	repo := inventoryRepo(c)
	svc := newShadowSvc(repo)
	now := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	res, err := svc.UploadInventory(auditableCtx(t), shadowTestOU, c.ID, "my-agents.csv", []byte(inventoryCSV))
	require.NoError(t, err)
	assert.Equal(t, 4, res.AgentsRead)
	assert.Equal(t, 2, res.AgentsSaved)
	assert.Equal(t, 2, res.LeftOut)
	assert.Equal(t, []string{"team"}, res.IgnoredColumns)
	require.Len(t, res.Problems, 2)
	assert.Equal(t, 2, res.ProblemCount)
	assert.Equal(t, 3, res.Problems[0].Row)
	assert.Equal(t, 4, res.Problems[1].Row)
	assert.True(t, res.Problems[0].LeftOut)

	require.Len(t, repo.SaveUploadedScanCalls(), 1)
	call := repo.SaveUploadedScanCalls()[0]
	scan, in := call.Scan, call.In
	assert.Equal(t, models.ShadowScanSucceeded, scan.Status)
	assert.True(t, scan.Options.InventoryUpload, "marked so it is never the latest cloud scan")
	assert.Equal(t, []string{c.ID.String()}, scan.ConnectionIDs)
	assert.Equal(t, now, scan.CreatedAt)
	require.NotNil(t, scan.FinishedAt)
	require.Len(t, scan.Results, 1)
	r := scan.Results[0]
	assert.Equal(t, shadowConnectionOK, r.Outcome)
	require.Len(t, r.Calls, 1)
	assert.Equal(t, "Uploaded CSV file", r.Calls[0].Call, "the call never names the user's file")
	assert.Equal(t, 2, r.Calls[0].Items)
	assert.Contains(t, r.Calls[0].Message, "2 agents left out")
	assert.NotContains(t, r.Calls[0].Message, "my-agents")

	assert.True(t, in.Complete, "the file is the complete inventory: agents not in it are marked gone")
	assert.Equal(t, c.ID, in.ConnectionID)
	assert.Equal(t, scan.ID, in.ScanID)
	assert.Equal(t, now, in.ScanStartedAt)
	require.Len(t, in.Findings, 2)
	f := in.Findings[0]
	assert.Equal(t, models.ShadowProviderCustom, f.Finding.Provider)
	assert.Equal(t, models.ShadowKindPlatformAgent, f.Finding.Kind)
	assert.Equal(t, "bot-1", f.Finding.ExternalID)
	assert.Equal(t, "Support bot", f.Finding.DisplayName)
	assert.Equal(t, "active", f.Finding.State)
	assert.Equal(t, "jane@example.com", f.Finding.OwnerHint.Email)
	require.Len(t, f.Signals, 1)
	assert.Equal(t, sa.SigCustomAgent, f.Signals[0].Code)
	assert.Equal(t, "Uploaded CSV file", f.Signals[0].Source)
	assert.NotContains(t, f.Finding.Attributes, "team", "unknown columns are never stored")
	assert.Len(t, repo.ApplyCorrelationCalls(), 1, "duplicates across connections are recomputed")
}

func TestShadowUploadInventory_Refusals(t *testing.T) {
	c := inventoryConnection()
	repo := inventoryRepo(c)
	svc := newShadowSvc(repo)
	ctx := auditableCtx(t)

	_, err := svc.UploadInventory(ctx, shadowTestOU, uuid.New(), "a.csv", []byte(inventoryCSV))
	assert.ErrorIs(t, err, utils.ErrShadowConnectionNotFound)

	openai := &models.ShadowConnection{ID: uuid.New(), OUID: shadowTestOU, Provider: models.ShadowProviderOpenAI}
	_, err = newShadowSvc(inventoryRepo(openai)).UploadInventory(ctx, shadowTestOU, openai.ID, "a.csv", []byte(inventoryCSV))
	assert.ErrorIs(t, err, utils.ErrShadowConnectionNotFound, "only a Bring your own inventory connection takes files")

	_, err = svc.UploadInventory(ctx, shadowTestOU, c.ID, "a.csv", []byte("name,owner\nbot,jane\n"))
	require.ErrorIs(t, err, utils.ErrInvalidInput)
	var bad *sa.InventoryFileError
	require.ErrorAs(t, err, &bad)
	assert.Contains(t, bad.Message, "no id column")

	_, err = svc.UploadInventory(context.Background(), shadowTestOU, c.ID, "a.csv", []byte(inventoryCSV))
	assert.ErrorIs(t, err, audit.ErrRecorderUnavailable, "nothing is saved unrecorded")
	assert.Empty(t, repo.SaveUploadedScanCalls())

	repo.SaveUploadedScanFunc = func(context.Context, *models.ShadowScan, repositories.ShadowConnectionApply) (int, error) {
		return 0, utils.ErrShadowScanSuperseded
	}
	_, err = svc.UploadInventory(ctx, shadowTestOU, c.ID, "a.csv", []byte(inventoryCSV))
	assert.ErrorIs(t, err, utils.ErrShadowScanSuperseded)

	repo.SaveUploadedScanFunc = func(context.Context, *models.ShadowScan, repositories.ShadowConnectionApply) (int, error) {
		return 0, gorm.ErrRecordNotFound
	}
	_, err = svc.UploadInventory(ctx, shadowTestOU, c.ID, "a.csv", []byte(inventoryCSV))
	assert.ErrorIs(t, err, utils.ErrShadowConnectionNotFound, "the connection was deleted meanwhile")

	repo.SaveUploadedScanFunc = func(context.Context, *models.ShadowScan, repositories.ShadowConnectionApply) (int, error) {
		return 0, errors.New("db down")
	}
	_, err = svc.UploadInventory(ctx, shadowTestOU, c.ID, "a.csv", []byte(inventoryCSV))
	require.Error(t, err)
	assert.NotErrorIs(t, err, utils.ErrInvalidInput)
}

// A Bring your own inventory connection has nothing to test.
func TestShadowTestConnection_InventoryIsRefused(t *testing.T) {
	c := inventoryConnection()
	_, err := newShadowSvc(inventoryRepo(c)).TestConnection(context.Background(), shadowTestOU, c.ID)
	require.ErrorIs(t, err, utils.ErrInvalidInput)
	assert.Contains(t, err.Error(), "inventory files")
}
