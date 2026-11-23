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
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
)

const resourceSyncOrg = "org-1"

var resourceSyncGateway = uuid.MustParse("0193f1c2-0000-7000-8000-0000000000a1")

func gatewayRepoFor(gw *gwmodels.Gateway, err error) *gwrepomocks.GatewayRepositoryMock {
	return &gwrepomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(string) (*gwmodels.Gateway, error) { return gw, err },
	}
}

func budgetItem(id, kind, name string) gwmodels.GatewayResourceItem {
	doc, _ := json.Marshal(map[string]any{
		"apiVersion": "gateway.api-platform.wso2.com/v1", "kind": kind,
		"metadata": map[string]any{"name": name}, "spec": map[string]any{},
	})
	now := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	return gwmodels.GatewayResourceItem{DPID: id, Configuration: doc, CreatedAt: now, UpdatedAt: now}
}

func TestGatewayResourceSync_RefusesAnotherOrgsGatewayAndKeepsRealErrors(t *testing.T) {
	other := &gwmodels.Gateway{UUID: resourceSyncGateway, OUID: "org-2"}
	svc := NewGatewayResourceSyncService(gatewayRepoFor(other, nil), &gwrepomocks.BudgetRepositoryMock{}, nil)
	_, err := svc.Sync(context.Background(), resourceSyncGateway.String(), resourceSyncOrg, gwmodels.GatewayResourcesRequest{})
	assert.ErrorIs(t, err, gwerr.ErrGatewayNotFound)

	dbDown := errors.New("db down")
	svc = NewGatewayResourceSyncService(gatewayRepoFor(nil, dbDown), &gwrepomocks.BudgetRepositoryMock{}, nil)
	_, err = svc.Sync(context.Background(), resourceSyncGateway.String(), resourceSyncOrg, gwmodels.GatewayResourcesRequest{})
	assert.ErrorIs(t, err, dbDown)
	assert.NotErrorIs(t, err, gwerr.ErrGatewayNotFound, "a real error is not masked as not found")
}

func TestGatewayResourceSync_SavesDeletesAndReportsEachOne(t *testing.T) {
	gw := &gwmodels.Gateway{UUID: resourceSyncGateway, OUID: resourceSyncOrg}
	cpID := uuid.MustParse("0193f1c2-0000-7000-8000-0000000000c1")
	var saved []gwmodels.Budget
	var deleted []string
	repo := &gwrepomocks.BudgetRepositoryMock{
		SaveForGatewayFunc: func(_ context.Context, b *gwmodels.Budget, name string) (bool, error) {
			if name == "org-wide" {
				return false, nil // the control plane holds the name
			}
			b.ID = cpID
			b.Name = name
			saved = append(saved, *b)
			return true, nil
		},
		DeleteForGatewayFunc: func(_ context.Context, ouID string, gatewayID uuid.UUID, names []string) error {
			assert.Equal(t, resourceSyncOrg, ouID)
			assert.Equal(t, resourceSyncGateway, gatewayID)
			deleted = append(deleted, names...)
			return nil
		},
	}
	svc := NewGatewayResourceSyncService(gatewayRepoFor(gw, nil), repo, nil)

	resp, err := svc.Sync(context.Background(), resourceSyncGateway.String(), resourceSyncOrg, gwmodels.GatewayResourcesRequest{
		Resources: []gwmodels.GatewayResourceItem{
			budgetItem("dp-a", "Budget", "team-a"),
			budgetItem("dp-clash", "Budget", "org-wide"),
			budgetItem("dp-dup", "Budget", "team-a"),
			budgetItem("dp-kind", "RestApi", "team-x"),
			budgetItem("dp-noname", "Budget", " "),
			// Valid on a gateway, not as a control-plane budget name.
			budgetItem("dp-dotted", "Budget", "team.a"),
			budgetItem("dp-long", "Budget", strings.Repeat("a", 64)),
			{DPID: "dp-bad", Configuration: json.RawMessage(`not json`)},
		},
		Deleted: []gwmodels.GatewayResourceItem{budgetItem("dp-gone", "Budget", "team-old")},
	})
	require.NoError(t, err)
	assert.Equal(t, 9, resp.Total)
	assert.Equal(t, 2, resp.Success)
	assert.Equal(t, 7, resp.Failed)
	assert.Equal(t, gwmodels.GatewayResourceResult{ID: cpID.String(), Status: gwmodels.GatewayResourceStatusSynced}, resp.Resources["dp-a"])
	assert.Equal(t, gwmodels.GatewayResourceStatusSynced, resp.Resources["dp-gone"].Status)
	assert.Equal(t, BudgetErrAlreadyExists, resp.Resources["dp-clash"].Error)
	assert.Equal(t, GatewayResourceErrDuplicateName, resp.Resources["dp-dup"].Error)
	assert.Equal(t, GatewayResourceErrUnsupported, resp.Resources["dp-kind"].Error)
	assert.Equal(t, GatewayResourceErrInvalid, resp.Resources["dp-noname"].Error)
	assert.Equal(t, GatewayResourceErrInvalid, resp.Resources["dp-bad"].Error)
	assert.Equal(t, GatewayResourceErrInvalid, resp.Resources["dp-dotted"].Error)
	assert.Equal(t, GatewayResourceErrInvalid, resp.Resources["dp-long"].Error)
	assert.Equal(t, []string{"team-old"}, deleted)
	assert.Empty(t, repo.DeleteForGatewayExceptCalls(), "only a complete push removes unlisted budgets")

	require.Len(t, saved, 1)
	b := saved[0]
	assert.Equal(t, "team-a", b.Name)
	assert.Equal(t, gwmodels.BudgetOriginGateway, b.Origin)
	assert.Equal(t, resourceSyncGateway, *b.GatewayID)
}

func TestGatewayResourceSync_ACompletePushRemovesWhatItDoesNotList(t *testing.T) {
	gw := &gwmodels.Gateway{UUID: resourceSyncGateway, OUID: resourceSyncOrg}
	var keep []string
	kept := false
	repo := &gwrepomocks.BudgetRepositoryMock{
		SaveForGatewayFunc: func(_ context.Context, _ *gwmodels.Budget, name string) (bool, error) { return name != "org-wide", nil },
		DeleteForGatewayExceptFunc: func(_ context.Context, _ string, _ uuid.UUID, names []string) error {
			keep, kept = names, true
			return nil
		},
	}
	svc := NewGatewayResourceSyncService(gatewayRepoFor(gw, nil), repo, nil)

	_, err := svc.Sync(context.Background(), resourceSyncGateway.String(), resourceSyncOrg, gwmodels.GatewayResourcesRequest{
		Complete:  true,
		Resources: []gwmodels.GatewayResourceItem{budgetItem("dp-a", "Budget", "team-a"), budgetItem("dp-c", "Budget", "org-wide")},
	})
	require.NoError(t, err)
	require.True(t, kept)
	assert.Equal(t, []string{"team-a", "org-wide"}, keep, "every listed name is kept, stored or not")

	// A complete push with no budgets removes all of the gateway's budgets.
	_, err = svc.Sync(context.Background(), resourceSyncGateway.String(), resourceSyncOrg, gwmodels.GatewayResourcesRequest{Complete: true})
	require.NoError(t, err)
	assert.Empty(t, keep)
}

func TestGatewayResourceSync_StoreFailureIsAnError(t *testing.T) {
	gw := &gwmodels.Gateway{UUID: resourceSyncGateway, OUID: resourceSyncOrg}
	dbDown := errors.New("db down")
	repo := &gwrepomocks.BudgetRepositoryMock{
		SaveForGatewayFunc: func(context.Context, *gwmodels.Budget, string) (bool, error) { return false, dbDown },
	}
	svc := NewGatewayResourceSyncService(gatewayRepoFor(gw, nil), repo, nil)
	_, err := svc.Sync(context.Background(), resourceSyncGateway.String(), resourceSyncOrg, gwmodels.GatewayResourcesRequest{
		Resources: []gwmodels.GatewayResourceItem{budgetItem("dp-a", "Budget", "team-a")},
	})
	assert.ErrorIs(t, err, dbDown)
}
