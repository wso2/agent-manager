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

package gwrepo

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

// newID is the id of a new row: a UUIDv7, so ids sort by creation time.
func newID() uuid.UUID { return uuid.Must(uuid.NewV7()) }

// agentCond matches the rows of the named agent.
func agentCond(db *gorm.DB, ouID, projectName, agentName string) (string, []any) {
	return "org_id = ? AND agent_id IN (?)", []any{ouID, gwmodels.AgentIDQuery(db, ouID, projectName, agentName)}
}

// agentEnvCond matches the named agent's rows in the named environment.
func agentEnvCond(db *gorm.DB, ouID, projectName, agentName, environmentName string) (string, []any) {
	cond, args := agentCond(db, ouID, projectName, agentName)
	return cond + " AND environment_id IN (?)", append(args, gwmodels.EnvironmentIDQuery(db, ouID, environmentName))
}

// lockAgent locks the named agent's row for the rest of tx through the core, serialising work on that agent.
func lockAgent(ctx context.Context, core modkit.CoreAPI, tx *gorm.DB, ouID, projectName, agentName string) error {
	agent, err := core.GetAgentByName(ctx, ouID, projectName, agentName)
	if err != nil {
		return err
	}
	_, err = core.LockAgent(ctx, tx, ouID, agent.ID)
	return err
}
