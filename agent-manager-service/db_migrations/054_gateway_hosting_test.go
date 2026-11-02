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

package dbmigrations

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration054(t *testing.T) {
	g := scratchDatabase(t)
	require.NoError(t, migrateTo(g, 53))
	require.NoError(t, migrateTo(g, 54))

	insertGateway := func(hosting string) error {
		id := uuid.New()
		if hosting == "" {
			return g.Exec(`INSERT INTO gateways (uuid, ou_id, name, display_name, vhost, gateway_functionality_type) VALUES (?, ?, ?, ?, 'v.example.com', 'both')`,
				id, seedOU, id.String(), id.String()).Error
		}
		return g.Exec(`INSERT INTO gateways (uuid, ou_id, name, display_name, vhost, gateway_functionality_type, hosting) VALUES (?, ?, ?, ?, 'v.example.com', 'both', ?)`,
			id, seedOU, id.String(), id.String(), hosting).Error
	}

	require.NoError(t, insertGateway(""))
	var hosting string
	require.NoError(t, g.Raw(`SELECT hosting FROM gateways LIMIT 1`).Scan(&hosting).Error)
	assert.Equal(t, "managed", hosting)

	assert.NoError(t, insertGateway("self_hosted"))
	err := insertGateway("x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chk_gateways_hosting")
}
