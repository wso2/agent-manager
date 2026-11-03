//go:build integration

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

package repositories

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// createMCPProxyForTest inserts a proxy through the repository and removes it afterwards.
func createMCPProxyForTest(t *testing.T, repo MCPProxyRepository, ou, handle string,
	projectUUID, projectName *string, src *models.MCPProxySource) *models.MCPProxy {
	t.Helper()
	ctx := context.Background()
	proxy := &models.MCPProxy{Configuration: models.MCPProxyConfig{Name: handle, Version: "v1", Source: src}}
	require.NoError(t, db.GetDB().Transaction(func(tx *gorm.DB) error {
		return repo.Create(ctx, tx, proxy, handle, handle, "v1", ou, projectUUID, projectName)
	}))
	t.Cleanup(func() { _ = repo.Delete(context.Background(), handle, ou) })
	return proxy
}

func handles(proxies []*models.MCPProxy) []string {
	out := make([]string, 0, len(proxies))
	for _, p := range proxies {
		out = append(out, p.Artifact.Handle)
	}
	return out
}

func TestCreateProjectProxyNotInCatalog(t *testing.T) {
	repo := NewMCPProxyRepo(db.GetDB())
	ou := "ou-" + uuid.NewString()[:8]
	project, projectName := uuid.NewString(), "checkout"

	orgProxy := createMCPProxyForTest(t, repo, ou, "org-proxy", nil, nil, nil)
	projProxy := createMCPProxyForTest(t, repo, ou, "proj-proxy", &project, &projectName, nil)

	got, err := repo.GetByUUID(context.Background(), orgProxy.UUID.String(), ou)
	require.NoError(t, err)
	assert.True(t, got.Artifact.InCatalog, "an organization proxy is listed in the catalog")
	assert.Nil(t, got.ProjectUUID)
	assert.Nil(t, got.ProjectName)

	got, err = repo.GetByUUID(context.Background(), projProxy.UUID.String(), ou)
	require.NoError(t, err)
	assert.False(t, got.Artifact.InCatalog, "a project proxy is not listed in the catalog")
	require.NotNil(t, got.ProjectUUID)
	assert.Equal(t, project, *got.ProjectUUID)
	require.NotNil(t, got.ProjectName)
	assert.Equal(t, projectName, *got.ProjectName)
}

func TestListFilters(t *testing.T) {
	repo := NewMCPProxyRepo(db.GetDB())
	ctx := context.Background()
	ou := "ou-" + uuid.NewString()[:8]
	p1, p2 := uuid.NewString(), uuid.NewString()
	n1, n2 := "one", "two"
	srcA := &models.MCPProxySource{ComponentName: "comp-a", EndpointName: "ep-1"}
	srcB := &models.MCPProxySource{ComponentName: "comp-a", EndpointName: "ep-2"}

	createMCPProxyForTest(t, repo, ou, "org-a", nil, nil, nil)
	createMCPProxyForTest(t, repo, ou, "p1-a", &p1, &n1, srcA)
	createMCPProxyForTest(t, repo, ou, "p1-b", &p1, &n1, srcB)
	createMCPProxyForTest(t, repo, ou, "p2-a", &p2, &n2, srcA)

	t.Run("zero filter lists organization proxies only", func(t *testing.T) {
		got, total, err := repo.List(ctx, ou, MCPProxyListFilter{})
		require.NoError(t, err)
		assert.Equal(t, []string{"org-a"}, handles(got))
		assert.EqualValues(t, 1, total)
	})
	t.Run("project only", func(t *testing.T) {
		got, total, err := repo.List(ctx, ou, MCPProxyListFilter{ProjectUUID: &p1})
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"p1-a", "p1-b"}, handles(got))
		assert.EqualValues(t, 2, total)
	})
	t.Run("project plus organization proxies, never another project", func(t *testing.T) {
		got, total, err := repo.List(ctx, ou, MCPProxyListFilter{ProjectUUID: &p1, IncludeOrgOwned: true})
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"org-a", "p1-a", "p1-b"}, handles(got))
		assert.EqualValues(t, 3, total)
	})
	t.Run("component and endpoint filter on configuration.source", func(t *testing.T) {
		got, _, err := repo.List(ctx, ou, MCPProxyListFilter{ProjectUUID: &p1, Component: "comp-a", Endpoint: "ep-2"})
		require.NoError(t, err)
		assert.Equal(t, []string{"p1-b"}, handles(got))

		got, _, err = repo.List(ctx, ou, MCPProxyListFilter{ProjectUUID: &p1, Component: "comp-a"})
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"p1-a", "p1-b"}, handles(got))
	})
	t.Run("limit and offset page the rows but not the total", func(t *testing.T) {
		got, total, err := repo.List(ctx, ou, MCPProxyListFilter{ProjectUUID: &p1, IncludeOrgOwned: true, Limit: 2})
		require.NoError(t, err)
		assert.Len(t, got, 2)
		assert.EqualValues(t, 3, total)

		got, total, err = repo.List(ctx, ou, MCPProxyListFilter{ProjectUUID: &p1, IncludeOrgOwned: true, Limit: 2, Offset: 2})
		require.NoError(t, err)
		assert.Len(t, got, 1)
		assert.EqualValues(t, 3, total)
	})
	t.Run("other organizations are invisible", func(t *testing.T) {
		got, total, err := repo.List(ctx, "other-"+ou, MCPProxyListFilter{ProjectUUID: &p1, IncludeOrgOwned: true})
		require.NoError(t, err)
		assert.Empty(t, got)
		assert.EqualValues(t, 0, total)
	})
	t.Run("Count stays organization-level", func(t *testing.T) {
		n, err := repo.Count(ctx, ou)
		require.NoError(t, err)
		assert.Equal(t, 1, n)
	})
}

func TestGetInProjectRejectsOtherProject(t *testing.T) {
	repo := NewMCPProxyRepo(db.GetDB())
	ctx := context.Background()
	ou := "ou-" + uuid.NewString()[:8]
	p1, p2, name := uuid.NewString(), uuid.NewString(), "n"
	createMCPProxyForTest(t, repo, ou, "org-a", nil, nil, nil)
	createMCPProxyForTest(t, repo, ou, "p1-a", &p1, &name, nil)

	got, err := repo.GetInProject(ctx, "p1-a", ou, p1)
	require.NoError(t, err)
	assert.Equal(t, "p1-a", got.Artifact.Handle)

	_, err = repo.GetInProject(ctx, "p1-a", ou, p2)
	assert.ErrorIs(t, err, utils.ErrMCPProxyNotFound, "the proxy lives in another project")
	_, err = repo.GetInProject(ctx, "org-a", ou, p1)
	assert.ErrorIs(t, err, utils.ErrMCPProxyNotFound, "an organization proxy is not in any project")
	_, err = repo.GetInProject(ctx, "p1-a", "other-"+ou, p1)
	assert.ErrorIs(t, err, utils.ErrMCPProxyNotFound, "other organizations are invisible")
}
