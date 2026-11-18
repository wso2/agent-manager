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

package aigateway

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

// The module's tables reference core rows by id, without foreign keys. These
// finalize hooks run in the core's delete transaction and do what the keys
// did: remove the module's rows of a deleted agent, and refuse to delete a
// project or an environment the module still uses.
func registerHooks(rt modkit.Runtime) {
	rt.Hook(modkit.ResourceAgent, modkit.Finalize, finalizeAgent)
	rt.Hook(modkit.ResourceProject, modkit.Finalize, finalizeProject)
	rt.Hook(modkit.ResourceEnvironment, modkit.Finalize, finalizeEnvironment)
}

// agentTables hold rows of one agent. Rows of their own child tables go with
// them through the module's foreign keys.
var agentTables = []string{
	"mod_ai_gateway_agent_gateway_settings",
	"mod_ai_gateway_agent_model_configs",
	"mod_ai_gateway_ai_applications",
	"mod_ai_gateway_agent_exposures",
	"mod_ai_gateway_agent_cards",
	"mod_ai_gateway_a2a_proxies",
}

func finalizeAgent(_ context.Context, req modkit.HookRequest) error {
	for _, table := range agentTables {
		if err := req.Tx.Exec("DELETE FROM "+table+" WHERE org_id = ? AND agent_id = ?", req.OrgID, req.ID).Error; err != nil {
			return fmt.Errorf("delete %s of agent %s: %w", table, req.ID, err)
		}
	}
	return nil
}

// projectTables hold rows that belong to a project.
var projectTables = []struct{ table, what string }{
	{"mod_ai_gateway_llm_proxies", "LLM proxy(ies)"},
	{"mod_ai_gateway_mcp_proxies", "MCP server(s)"},
}

func finalizeProject(_ context.Context, req modkit.HookRequest) error {
	var left []string
	for _, t := range projectTables {
		var n int64
		if err := req.Tx.Raw("SELECT COUNT(*) FROM "+t.table+" WHERE org_id = ? AND project_id = ?", req.OrgID, req.ID).Scan(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			left = append(left, fmt.Sprintf("%d %s", n, t.what))
		}
	}
	if len(left) == 0 {
		return nil
	}
	name, _ := req.Object["name"].(string)
	return &modkit.Error{
		Status:  http.StatusConflict,
		Message: fmt.Sprintf("project %q still has %s", name, strings.Join(left, ", ")),
	}
}

// environmentTables hold rows of one environment, removed with it.
var environmentTables = []string{
	"mod_ai_gateway_environment_default_gateways",
	"mod_ai_gateway_gateway_environments",
	"mod_ai_gateway_mcp_proxy_endpoint_environments",
	"mod_ai_gateway_agent_model_config_llm",
	"mod_ai_gateway_agent_model_config_mcp_envs",
	"mod_ai_gateway_agent_model_config_vars",
	"mod_ai_gateway_agent_gateway_settings",
	"mod_ai_gateway_ai_applications",
	"mod_ai_gateway_agent_exposures",
	"mod_ai_gateway_agent_cards",
	"mod_ai_gateway_a2a_proxies",
}

// finalizeEnvironment refuses while a gateway mapped to the environment
// serves artifacts, and otherwise removes the module's rows of it.
func finalizeEnvironment(_ context.Context, req modkit.HookRequest) error {
	var serving int64
	err := req.Tx.Raw(`SELECT COUNT(*) FROM mod_ai_gateway_artifact_gateway_status s
		JOIN mod_ai_gateway_gateway_environments ge ON ge.gateway_id = s.gateway_id
		WHERE ge.org_id = ? AND ge.environment_id = ?`, req.OrgID, req.ID).Scan(&serving).Error
	if err != nil {
		return err
	}
	if serving > 0 {
		name, _ := req.Object["name"].(string)
		return &modkit.Error{
			Status:  http.StatusConflict,
			Message: fmt.Sprintf("environment %q has gateways serving %d artifact(s); undeploy them or unmap the gateways first", name, serving),
		}
	}
	for _, table := range environmentTables {
		if err := req.Tx.Exec("DELETE FROM "+table+" WHERE org_id = ? AND environment_id = ?", req.OrgID, req.ID).Error; err != nil {
			return fmt.Errorf("delete %s of environment %s: %w", table, req.ID, err)
		}
	}
	return nil
}
