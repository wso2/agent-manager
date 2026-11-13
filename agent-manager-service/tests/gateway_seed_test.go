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

package tests

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
)

// seedIngressGatewayForDefaultEnvironment inserts an INGRESS gateway mapped to
// the default environment, because an API agent is published through its
// environment's ingress gateway and the test database carries none by default.
func seedIngressGatewayForDefaultEnvironment(t *testing.T, ouID string) uuid.UUID {
	t.Helper()
	gdb := db.GetDB()
	ensureOrgRow(t, ouID)
	env := uuid.MustParse(apitestutils.DefaultEnvironmentUUID)
	dbtest.EnvironmentNamed(t, gdb, ouID, env, "default-"+env.String()[:8])
	return dbtest.GatewayIn(t, gdb, ouID, models.GatewayRoleIngress, env).UUID
}

// ensureOrgRow makes sure the organization the mocked tokens name exists.
func ensureOrgRow(t *testing.T, ouID string) {
	t.Helper()
	gdb := db.GetDB()
	var n int64
	require.NoError(t, gdb.Model(&models.Organization{}).Where("id = ?", ouID).Count(&n).Error)
	if n > 0 {
		return
	}
	now := time.Now().UTC()
	require.NoError(t, gdb.Create(&models.Organization{
		ID: uuid.MustParse(ouID), Handle: "mock-org", DisplayName: "Mock org", ThunderOUID: &ouID, CreatedAt: now, UpdatedAt: now,
	}).Error)
}
