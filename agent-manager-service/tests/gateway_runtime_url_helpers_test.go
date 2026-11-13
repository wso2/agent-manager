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
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
)

// runtimeURLTestTx opens a transaction rolled back at test end, so seeded rows never persist for another test.
func runtimeURLTestTx(t *testing.T) *gorm.DB {
	t.Helper()
	tx := db.DB(context.Background()).Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { require.NoError(t, tx.Rollback().Error) })
	return tx
}

// seedRuntimeURLGateway creates a gateway named name with no runtime URL and returns its id.
func seedRuntimeURLGateway(t *testing.T, tx *gorm.DB, name string) uuid.UUID {
	t.Helper()
	gw := dbtest.GatewayIn(t, tx, dbtest.Org(t, tx), "both")
	require.NoError(t, tx.Exec(`UPDATE gateways SET name = ?, display_name = ?, is_active = ?, runtime_url = NULL WHERE id = ?`,
		name, name, false, gw.UUID).Error)
	return gw.UUID
}

// runtimeURLOf returns the raw column value: the NULL vs populated distinction is the point of these assertions.
func runtimeURLOf(t *testing.T, tx *gorm.DB, id uuid.UUID) sql.NullString {
	t.Helper()
	var got sql.NullString
	require.NoError(t, tx.Raw(`SELECT runtime_url FROM gateways WHERE id = ?`, id).Row().Scan(&got))
	return got
}

// gatewayOrgOf is the organization a gateway belongs to.
func gatewayOrgOf(t *testing.T, tx *gorm.DB, id uuid.UUID) string {
	t.Helper()
	var org string
	require.NoError(t, tx.Raw(`SELECT org_id FROM gateways WHERE id = ?`, id).Row().Scan(&org))
	return org
}
