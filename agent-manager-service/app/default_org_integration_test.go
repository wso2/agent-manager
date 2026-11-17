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

// The configured organization exists after startup, under its handle and
// with its default environment, however often startup runs; with no OU id
// configured nothing is created.
func TestEnsureDefaultOrg(t *testing.T) {
	ctx := context.Background()
	g := db.GetDB()
	ouID := uuid.Must(uuid.NewV7()).String()
	handle := "seeded-" + ouID[len(ouID)-8:]
	t.Cleanup(func() {
		g.Where("org_id = ?", ouID).Delete(&models.Environment{})
		g.Where("id = ?", ouID).Delete(&models.Organization{})
	})

	require.NoError(t, ensureDefaultOrg(ctx, g, config.DefaultOrgConfig{}))

	cfg := config.DefaultOrgConfig{OUID: ouID, Handle: handle}
	for range 2 {
		require.NoError(t, ensureDefaultOrg(ctx, g, cfg))
	}
	var orgs []models.Organization
	require.NoError(t, g.Where("id = ?", ouID).Find(&orgs).Error)
	require.Len(t, orgs, 1)
	require.Equal(t, handle, orgs[0].Handle)
	var envs []models.Environment
	require.NoError(t, g.Where("org_id = ?", ouID).Find(&envs).Error)
	require.Len(t, envs, 1)
	require.Equal(t, models.DefaultEnvironmentName, envs[0].Name)

	require.Error(t, ensureDefaultOrg(ctx, g, config.DefaultOrgConfig{OUID: "not-a-uuid", Handle: "x"}))
}
