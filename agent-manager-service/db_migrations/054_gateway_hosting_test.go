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

	insertGateway := func(hosting, role string) error {
		id := uuid.New()
		if hosting == "" {
			return g.Exec(`INSERT INTO gateways (uuid, ou_id, name, display_name, vhost, gateway_functionality_type) VALUES (?, ?, ?, ?, 'v.example.com', ?)`,
				id, seedOU, id.String(), id.String(), role).Error
		}
		return g.Exec(`INSERT INTO gateways (uuid, ou_id, name, display_name, vhost, gateway_functionality_type, hosting) VALUES (?, ?, ?, ?, 'v.example.com', ?, ?)`,
			id, seedOU, id.String(), id.String(), role, hosting).Error
	}

	// Existing gateways of every role become managed, which the egress-only rule allows.
	for _, role := range []string{"egress", "ingress", "both"} {
		require.NoError(t, insertGateway("", role))
	}
	require.NoError(t, migrateTo(g, 54))

	var hostings []string
	require.NoError(t, g.Raw(`SELECT DISTINCT hosting FROM gateways`).Scan(&hostings).Error)
	assert.Equal(t, []string{"managed"}, hostings)

	require.NoError(t, insertGateway("", "both"))
	assert.NoError(t, insertGateway("self_hosted", "egress"))

	err := insertGateway("self_hosted", "both")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chk_gateways_self_hosted_egress")

	err = insertGateway("x", "egress")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chk_gateways_hosting")
}
