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

// Package gwtest seeds the AI Gateway's rows for integration tests, on top of the core rows dbtest seeds.
package gwtest

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

func now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

// Gateway creates an active gateway with the given role, maps it to the organization's default environment, and
// returns it.
func Gateway(t *testing.T, db *gorm.DB, orgID, role string) *gwmodels.Gateway {
	t.Helper()
	return GatewayIn(t, db, orgID, role, dbtest.Environment(t, db, orgID))
}

// GatewayIn creates an active gateway with the given role mapped to exactly envs, creating any of them the
// organization does not have yet, and returns it.
func GatewayIn(t *testing.T, db *gorm.DB, orgID, role string, envs ...uuid.UUID) *gwmodels.Gateway {
	t.Helper()
	ts := now()
	id := uuid.Must(uuid.NewV7())
	gw := &gwmodels.Gateway{
		UUID: id, OUID: orgID, Name: "gw-" + id.String()[24:], DisplayName: "Test gateway", Properties: map[string]any{},
		Vhost: "gw.example.com", RuntimeURL: "http://gw.example.com:22893", GatewayFunctionalityType: role,
		IsActive: true, Hosting: gwmodels.GatewayHostingManaged, CreatedAt: ts, UpdatedAt: ts,
	}
	require.NoError(t, db.Create(gw).Error)
	for _, env := range envs {
		dbtest.EnvironmentWithID(t, db, orgID, env)
		require.NoError(t, db.Create(&gwmodels.GatewayEnvironmentMapping{
			GatewayUUID: id, EnvironmentUUID: env, OrgID: uuid.MustParse(orgID), CreatedAt: ts,
		}).Error)
	}
	return gw
}

// MapGateway maps the gateway to env, creating env if the organization does not have it.
func MapGateway(t *testing.T, db *gorm.DB, orgID string, gateway, env uuid.UUID) {
	t.Helper()
	dbtest.EnvironmentWithID(t, db, orgID, env)
	require.NoError(t, db.Create(&gwmodels.GatewayEnvironmentMapping{
		GatewayUUID: gateway, EnvironmentUUID: env, OrgID: uuid.MustParse(orgID), CreatedAt: now(),
	}).Error)
}

// LLMConfig gives the agent an LLM configuration that uses proxy in env, and returns the configuration's id.
func LLMConfig(t *testing.T, db *gorm.DB, orgID string, agentID, env, proxy uuid.UUID) uuid.UUID {
	t.Helper()
	dbtest.EnvironmentWithID(t, db, orgID, env)
	ts := now()
	cfg := gwmodels.AgentConfiguration{
		UUID: uuid.Must(uuid.NewV7()), Name: "c-" + uuid.NewString()[:8], OUID: orgID, AgentUUID: agentID, Type: "llm",
		CreatedAt: ts, UpdatedAt: ts,
	}
	require.NoError(t, db.Omit(clause.Associations).Create(&cfg).Error)
	require.NoError(t, db.Omit(clause.Associations).Create(&gwmodels.EnvAgentModelMapping{
		ConfigUUID: cfg.UUID, EnvironmentUUID: env, OrgID: orgID, LLMProxyUUID: proxy, CreatedAt: ts,
	}).Error)
	return cfg.UUID
}
