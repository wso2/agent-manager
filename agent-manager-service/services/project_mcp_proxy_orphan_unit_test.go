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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// staleProjUUID is the UUID of a project that was deleted in OpenChoreo; testProjA is the
// UUID of the project re-created with the same name (testProjName).
const staleProjUUID = "deleted-project-uuid"

// orphanFixture holds "orders", published by testProjName before it was deleted.
func orphanFixture(t *testing.T) *projectProxyFixture {
	t.Helper()
	f := newProjectProxyFixture(t)
	_, _, err := f.svc.Upsert(context.Background(), testOrg, testProjName, staleProjUUID, "orders", orderReq())
	require.NoError(t, err)
	return f
}

func TestUpsertAdoptsProxyLeftBySameNamedDeletedProject(t *testing.T) {
	f := orphanFixture(t)
	ctx := context.Background()

	resp, created, err := f.svc.Upsert(ctx, testOrg, testProjName, testProjA, "orders", orderReq())
	require.NoError(t, err)
	assert.False(t, created, "adoption is an update, not a create")
	assert.Equal(t, "orders", resp.ID)
	assert.Equal(t, testProjName, resp.ProjectName)
	assert.Equal(t, []string{"orders:" + staleProjUUID + "->" + testProjA}, f.reassigned)
	assert.Zero(t, f.updateCalls, "an identical body changes nothing but the project")

	got, err := f.svc.Get(ctx, testOrg, testProjA, "orders")
	require.NoError(t, err, "the re-created project now owns the proxy")
	assert.Equal(t, "orders", got.ID)
	list, err := f.svc.List(ctx, testOrg, testProjA, "", "", 50, 0)
	require.NoError(t, err)
	assert.Len(t, list.List, 1)

	_, created, err = f.svc.Upsert(ctx, testOrg, testProjName, testProjA, "orders", orderReq())
	require.NoError(t, err)
	assert.False(t, created)
	assert.Len(t, f.reassigned, 1, "an adopted proxy is not adopted again")
}

func TestUpsertAdoptionAppliesTheBody(t *testing.T) {
	f := orphanFixture(t)
	req := orderReq()
	req.Name = "Orders MCP v2"

	resp, created, err := f.svc.UpsertWithCreateCheck(context.Background(), testOrg, testProjName, testProjA, "orders", req, false)
	require.NoError(t, err, "adopting needs no create permission: the proxy exists")
	assert.False(t, created)
	assert.Equal(t, "Orders MCP v2", resp.Name)
	assert.Len(t, f.reassigned, 1)
	assert.Equal(t, 1, f.updateCalls)
}

func TestUpsertAdoptionKeepsSourceImmutable(t *testing.T) {
	f := orphanFixture(t)
	req := orderReq()
	req.Source.EndpointName = "other"

	_, _, err := f.svc.Upsert(context.Background(), testOrg, testProjName, testProjA, "orders", req)
	require.ErrorIs(t, err, utils.ErrImmutableFieldChange)
	assert.Empty(t, f.reassigned, "a refused upsert adopts nothing")
}

func TestUpsertDoesNotAdoptOtherProjectsProxy(t *testing.T) {
	f := newProjectProxyFixture(t)
	ctx := context.Background()
	_, _, err := f.svc.Upsert(ctx, testOrg, "proj-b", staleProjUUID, "orders", orderReq())
	require.NoError(t, err)

	_, _, err = f.svc.Upsert(ctx, testOrg, testProjName, testProjA, "orders", orderReq())
	require.ErrorIs(t, err, utils.ErrMCPProxyHandleInUse)
	assert.Empty(t, f.reassigned)
}

func TestUpsertDoesNotAdoptOrgProxy(t *testing.T) {
	f := newProjectProxyFixture(t)
	name := testProjName
	// An org proxy has no project UUID; a project name on it must not make it adoptable.
	f.proxies["orders"] = &models.MCPProxy{ProjectName: &name, Artifact: &models.Artifact{Handle: "orders"}}

	_, _, err := f.svc.Upsert(context.Background(), testOrg, testProjName, testProjA, "orders", orderReq())
	require.ErrorIs(t, err, utils.ErrMCPProxyHandleInUse)
	assert.Empty(t, f.reassigned)
	assert.Zero(t, f.createCalls)
}

func TestDeletedProjectRoutesReachOnlyItsStoredProxies(t *testing.T) {
	f := orphanFixture(t)
	ctx := context.Background()
	_, _, err := f.svc.Upsert(ctx, testOrg, "proj-b", testProjB, "b-one", orderReq())
	require.NoError(t, err)
	f.proxies["org-one"] = &models.MCPProxy{Artifact: &models.Artifact{Handle: "org-one"}}

	got, err := f.svc.GetInDeletedProject(ctx, testOrg, testProjName, "orders")
	require.NoError(t, err)
	assert.Equal(t, "orders", got.ID)
	assert.Equal(t, testProjName, got.ProjectName)

	for _, handle := range []string{"b-one", "org-one", "missing"} {
		_, err := f.svc.GetInDeletedProject(ctx, testOrg, testProjName, handle)
		assert.ErrorIs(t, err, utils.ErrMCPProxyNotFound, handle)
		assert.ErrorIs(t, f.svc.DeleteInDeletedProject(ctx, testOrg, testProjName, handle), utils.ErrMCPProxyNotFound, handle)
		_, err = f.svc.GetEnvironmentInDeletedProject(ctx, testOrg, testProjName, handle, "development")
		assert.ErrorIs(t, err, utils.ErrMCPProxyNotFound, handle)
		assert.ErrorIs(t, f.svc.DeleteEnvironmentInDeletedProject(ctx, testOrg, testProjName, handle, "development"),
			utils.ErrMCPProxyNotFound, handle)
	}

	list, err := f.svc.ListInDeletedProject(ctx, testOrg, testProjName, "", "", 50, 0)
	require.NoError(t, err)
	require.Len(t, list.List, 1)
	assert.Equal(t, "orders", list.List[0].ID)

	_, err = f.svc.GetEnvironmentInDeletedProject(ctx, testOrg, testProjName, "orders", "development")
	assert.ErrorIs(t, err, utils.ErrMCPProxyEnvironmentNotBound)
	assert.Empty(t, f.deletedProxy)
}

func TestDeleteInDeletedProject(t *testing.T) {
	t.Run("refuses while agents use the proxy", func(t *testing.T) {
		f := orphanFixture(t)
		f.mappings = []models.EnvAgentMCPMapping{{}}
		err := f.svc.DeleteInDeletedProject(context.Background(), testOrg, testProjName, "orders")
		require.ErrorIs(t, err, utils.ErrMCPProxyHasMappings)
		assert.Empty(t, f.deletedProxy)
	})
	t.Run("tears the proxy down", func(t *testing.T) {
		f := orphanFixture(t)
		require.NoError(t, f.svc.DeleteInDeletedProject(context.Background(), testOrg, testProjName, "orders"))
		assert.Equal(t, []string{"orders"}, f.deletedProxy)
	})
}
