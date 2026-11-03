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

package reconcile

import (
	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// RecordColumns names the SQL expressions of a kind's record query that UnplacedCondition reads.
type RecordColumns struct {
	OUID            string
	ArtifactUUID    string
	EnvironmentUUID string
}

// UnplacedCondition is the WHERE condition every UnplacedRecords shares: the record's artifact has no
// deployment_status row at all (an UNDEPLOYED row counts, so a user's undeploy sticks), and its environment has a
// gateway mapped that isn't deleted and whose role is one of roles, the roles the kind can be placed on. Gateway
// delete is a soft delete that leaves its mappings in place. A steady-state pass reads nothing, and a record in an
// environment with no such gateway isn't re-read on every pass.
// ouID "" means every org; env nil means every environment.
func UnplacedCondition(cols RecordColumns, roles []string, ouID string, env *uuid.UUID) (string, []any) {
	condition := `NOT EXISTS (SELECT 1 FROM deployment_status ds
			WHERE ds.artifact_uuid = ` + cols.ArtifactUUID + ` AND ds.ou_id = ` + cols.OUID + `)
		AND EXISTS (SELECT 1 FROM gateway_environment_mappings gem
			INNER JOIN gateways g ON g.uuid = gem.gateway_uuid AND g.deleted_at IS NULL AND g.gateway_functionality_type IN ?
			WHERE gem.environment_uuid = ` + cols.EnvironmentUUID + `)`
	args := []any{roles}
	if ouID != "" {
		condition += " AND " + cols.OUID + " = ?"
		args = append(args, ouID)
	}
	if env != nil {
		condition += " AND " + cols.EnvironmentUUID + " = ?"
		args = append(args, *env)
	}
	return condition, args
}

// PlacedCondition is the WHERE condition every PlacedOn shares: the record is in env, and its artifact has a row on
// gateway that is to be deployed or has a deploy owed, whatever its status. A row the user undeployed, with nothing
// owed, doesn't count.
func PlacedCondition(cols RecordColumns, env, gateway uuid.UUID) (string, []any) {
	condition := cols.EnvironmentUUID + ` = ? AND EXISTS (SELECT 1 FROM deployment_status ds
			WHERE ds.artifact_uuid = ` + cols.ArtifactUUID + ` AND ds.ou_id = ` + cols.OUID + `
			AND ds.gateway_uuid = ? AND (ds.status_desired = ? OR ds.next_attempt_at IS NOT NULL))`
	return condition, []any{env, gateway, models.DeploymentStatusDeployed}
}
