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

package app

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// The configured organization has its default environment after startup,
// however often startup runs; with no OU id configured nothing is created.
func TestEnsureDefaultOrg(t *testing.T) {
	ctx := context.Background()
	g := db.GetDB()
	ouID := uuid.Must(uuid.NewV7()).String()
	t.Cleanup(func() {
		g.Where("org_id = ?", ouID).Delete(&models.Environment{})
	})

	require.NoError(t, ensureDefaultOrg(ctx, g, config.AuthModeFile, config.DefaultOrgConfig{}))

	cfg := config.DefaultOrgConfig{OUID: ouID, Handle: "seeded-" + ouID[len(ouID)-8:]}
	for range 2 {
		require.NoError(t, ensureDefaultOrg(ctx, g, config.AuthModeFile, cfg))
	}
	var envs []models.Environment
	require.NoError(t, g.Where("org_id = ?", ouID).Find(&envs).Error)
	require.Len(t, envs, 1)
	require.Equal(t, models.DefaultEnvironmentName, envs[0].Name)

	require.Error(t, ensureDefaultOrg(ctx, g, config.AuthModeFile, config.DefaultOrgConfig{OUID: "not-a-uuid", Handle: "x"}))
}

// With an identity provider the trial organization is not set up at startup: the provider's own organization gets
// its default environment on its first request.
func TestEnsureDefaultOrgOnlyWithFileSignIn(t *testing.T) {
	ctx := context.Background()
	g := db.GetDB()
	ouID := uuid.Must(uuid.NewV7()).String()
	t.Cleanup(func() { g.Where("org_id = ?", ouID).Delete(&models.Environment{}) })

	require.NoError(t, ensureDefaultOrg(ctx, g, config.AuthModeIDP, config.DefaultOrgConfig{OUID: ouID, Handle: "trial"}))
	var envs int64
	require.NoError(t, g.Model(&models.Environment{}).Where("org_id = ?", ouID).Count(&envs).Error)
	require.Zero(t, envs)
}
