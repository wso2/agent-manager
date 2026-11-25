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

package gwrepo

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// An A2A proxy is found by its artifact and listed per environment; deleting the artifact deletes the proxy and its
// stored credential, and refuses to when the repository has no secret store to delete it from.
func TestCovA2AProxyGoesWithItsArtifact(t *testing.T) {
	gdb := db.GetDB()
	secrets := gwtest.NewSecrets()
	repo := NewA2AProxyRepository(gdb, repositories.NewCoreReadAPI(gdb), secrets)
	ctx := context.Background()
	proxy := newTestProxyIn(t, "Staging", "cov-"+uuid.NewString()[:8])
	cleanupProxy(t, repo, proxy)
	require.NoError(t, repo.Upsert(ctx, proxy))
	require.Equal(t, 1, secrets.Len())

	byArtifact, err := repo.GetByArtifact(ctx, proxy.OUID, proxy.ArtifactUUID)
	require.NoError(t, err)
	assert.Equal(t, proxy.ID, byArtifact.ID)
	assert.NotEmpty(t, byArtifact.UpstreamAuthSecret)
	_, err = repo.GetByArtifact(ctx, newOrg(t), proxy.ArtifactUUID)
	require.ErrorIs(t, err, ErrA2AProxyNotFound)

	inEnv, err := repo.ListForEnvironment(ctx, proxy.OUID, proxy.EnvironmentUUID)
	require.NoError(t, err)
	require.Len(t, inEnv, 1)
	assert.Equal(t, proxy.ID, inEnv[0].ID)
	envs, err := repo.ListEnvironmentUUIDs(ctx, proxy.OUID, "Staging")
	require.NoError(t, err)
	assert.Contains(t, envs, proxy.EnvironmentUUID)

	require.ErrorContains(t, NewArtifactRepo(gdb, nil).Delete(gdb, proxy.ArtifactUUID.String()), "no secret store",
		"an artifact with an A2A proxy needs the secret store to be deleted")
	require.NoError(t, NewArtifactRepo(gdb, secrets).Delete(gdb, proxy.ArtifactUUID.String()))
	_, err = repo.GetByArtifact(ctx, proxy.OUID, proxy.ArtifactUUID)
	require.ErrorIs(t, err, ErrA2AProxyNotFound)
	assert.Zero(t, secrets.Len(), "the proxy's credential goes with it")
}
