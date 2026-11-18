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
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/coreext"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/agentgwv1"
)

// gatewayStateTables are the AI Gateway's tables whose rows mean agents have
// state there: their LLM and MCP bindings, which hold proxy credentials, and
// the registered gateways.
var gatewayStateTables = []string{models.AgentConfiguration{}.TableName(), models.Gateway{}.TableName()}

// warnGatewayStateWithoutProvider logs once when no active module provides
// agent.gateway/v1 but the database holds gateway state, as when amp-core
// runs on a database the full distribution wrote. The agent service then
// skips the gateway's part of create and delete, so that state is left
// alone. It returns the tables it found rows in.
func warnGatewayStateWithoutProvider(ctx context.Context, db *gorm.DB, log *slog.Logger) []string {
	if _, ok := coreext.Capability(agentgwv1.ID); ok {
		return nil
	}
	var held []string
	for _, table := range gatewayStateTables {
		var n int64
		if err := db.WithContext(ctx).Table(table).Count(&n).Error; err != nil {
			log.Debug("could not count gateway state", "table", table, "error", err)
			continue
		}
		if n > 0 {
			held = append(held, fmt.Sprintf("%s=%d", table, n))
		}
	}
	if len(held) > 0 {
		log.Warn("The database holds AI Gateway state, but no active module provides "+agentgwv1.ID+
			": deleting an agent will not revoke its LLM or MCP proxy credentials, and the name of an agent "+
			"with leftover configurations can be reused. Run the full distribution to manage or clean up that state.",
			"rows", held)
	}
	return held
}
