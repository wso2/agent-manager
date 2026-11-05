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

// staleProjUUID is the UUID of a project that was deleted in OpenChoreo.
const staleProjUUID = "deleted-project-uuid"

// orphanFixture holds "orders", published by testProjName before it was deleted.
func orphanFixture(t *testing.T) *projectProxyFixture {
	t.Helper()
	f := newProjectProxyFixture(t)
	_, _, err := f.svc.Upsert(context.Background(), testOrg, testProjName, staleProjUUID, "orders", orderReq())
	require.NoError(t, err)
	return f
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
