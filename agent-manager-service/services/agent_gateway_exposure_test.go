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

package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

func (f *a2aFixture) storedArtifact(t *testing.T, artifact models.ArtifactRef) *models.Artifact {
	t.Helper()
	stored, err := repositories.NewArtifactRepo(db.GetDB()).GetByUUID(artifact.ArtifactUUID.String(), artifact.OUID)
	require.NoError(t, err)
	return stored
}

// legacyRestAgent is a REST agent whose artifact predates RestApi: an Agent row, with one API key.
func (f *a2aFixture) legacyRestAgent(t *testing.T) (models.ArtifactRef, agentExposureKey) {
	t.Helper()
	artifact, key := f.platformAgent(t)
	f.seedAPIKey(t, artifact, "kept", "active", nil)
	return artifact, key
}

func TestEnsureArtifactSwitchesLegacyKind(t *testing.T) {
	f := newA2AFixture(t)
	legacy, key := f.legacyRestAgent(t)
	gdb := db.GetDB()
	artifacts := repositories.NewArtifactRepo(gdb)

	var switched *models.Artifact
	require.NoError(t, gdb.Transaction(func(tx *gorm.DB) error {
		var err error
		switched, err = ensureExposedAgentEnvAPIArtifact(tx, artifacts, key.OUID, key.ProjectName, key.AgentName,
			f.env.String(), restAgentShape)
		return err
	}))

	assert.Equal(t, legacy.ArtifactUUID, switched.UUID)
	assert.Equal(t, models.KindRestAPI, switched.Kind)
	stored := f.storedArtifact(t, legacy)
	assert.Equal(t, models.KindRestAPI, stored.Kind)
	assert.Equal(t, legacy.Handle, stored.Handle)
	keys, err := repositories.NewAPIKeyRepo(gdb).ListPermanentByArtifactKinds(key.OUID, []string{models.KindRestAPI})
	require.NoError(t, err)
	require.Len(t, keys, 1)
	assert.Equal(t, legacy.ArtifactUUID, keys[0].ArtifactUUID)

	t.Run("an A2A agent's Agent row stays", func(t *testing.T) {
		a2a, a2aKey := f.platformAgent(t)
		got, err := ensureExposedAgentEnvAPIArtifact(gdb, artifacts, a2aKey.OUID, a2aKey.ProjectName, a2aKey.AgentName,
			f.env.String(), a2aAgentShape)
		require.NoError(t, err)
		assert.Equal(t, a2a.ArtifactUUID, got.UUID)
		assert.Equal(t, models.KindAgent, f.storedArtifact(t, a2a).Kind)
	})
}
