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

func TestMigration052(t *testing.T) {
	g := scratchDatabase(t)
	require.NoError(t, migrateTo(g, 51))
	require.NoError(t, migrateTo(g, 52))

	insertExposure := func(protocol string) error {
		artifactID := uuid.New()
		exec(t, g, `INSERT INTO artifacts (uuid, handle, name, version, kind, ou_id) VALUES (?, ?, ?, 'v1', 'RestApi', ?)`,
			artifactID, artifactID.String(), artifactID.String(), seedOU)
		return g.Exec(`INSERT INTO platform_agent_exposures
			(ou_id, artifact_uuid, project_name, agent_name, environment_uuid, environment_name, protocol)
			VALUES (?, ?, 'proj', ?, ?, 'dev', ?)`, seedOU, artifactID, artifactID.String(), uuid.New(), protocol).Error
	}

	assert.NoError(t, insertExposure("a2a"))
	assert.NoError(t, insertExposure("rest"))
	err := insertExposure("x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chk_platform_agent_exposures_protocol")
}
