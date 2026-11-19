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
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwaudit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
)

// The module's tables reference core rows by id, without foreign keys. The
// agent hooks validate and store the extensions.ai-gateway block and record a
// deleted agent's teardown; the event handlers provision and revoke what that
// needs on the gateways (see gwsvc/agent_bindings.go). The project and
// environment finalize hooks refuse to delete what the module still uses.
func registerHooks(rt modkit.Runtime, agents *gwsvc.AgentGatewayService) {
	rt.Hook(modkit.ResourceAgent, modkit.Validate, func(ctx context.Context, req modkit.HookRequest) error {
		if req.Action == modkit.ActionDelete {
			return nil
		}
		ext, err := gwsvc.DecodeAgentExtension(req.Extension)
		if err != nil {
			return err
		}
		return agents.ValidateAgentExtension(ctx, req.Tx, req, ext)
	})
	rt.Hook(modkit.ResourceAgent, modkit.Apply, func(ctx context.Context, req modkit.HookRequest) error {
		ext, err := gwsvc.DecodeAgentExtension(req.Extension)
		if err != nil {
			return err
		}
		changed, err := agents.StoreAgentExtension(ctx, req.Tx, req, ext)
		if err != nil || !changed || req.Action != modkit.ActionUpdate {
			return err
		}
		// A create is reconciled on agent.created.
		return rt.PublishTx(ctx, req.Tx, gwaudit.AgentBindingsChanged, map[string]string{"agentId": req.ID, "orgId": req.OrgID})
	})
	rt.Hook(modkit.ResourceAgent, modkit.Finalize, func(_ context.Context, req modkit.HookRequest) error {
		return gwsvc.RecordAgentTeardown(req.Tx, req)
	})
	rt.Hook(modkit.ResourceProject, modkit.Finalize, finalizeProject)
	rt.Hook(modkit.ResourceEnvironment, modkit.Finalize, finalizeEnvironment(rt.Secrets()))

	reconcile := func(ctx context.Context, e modkit.Event) error {
		var d struct {
			ID      string `json:"id"`
			AgentID string `json:"agentId"`
		}
		if err := json.Unmarshal(e.Data, &d); err != nil {
			return err
		}
		if d.AgentID == "" {
			d.AgentID = d.ID
		}
		return agents.ReconcileAgentBindings(ctx, e.OrgID, d.AgentID)
	}
	rt.Subscribe(modkit.EventAgentCreated, reconcile)
	rt.Subscribe(gwaudit.AgentBindingsChanged, reconcile)
	rt.Subscribe(modkit.EventAgentDeleted, func(ctx context.Context, e modkit.Event) error {
		var d struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(e.Data, &d); err != nil {
			return err
		}
		return agents.TeardownAgent(ctx, d.ID)
	})
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
// serves artifacts, and otherwise removes the module's rows of it and the
// credentials they own.
func finalizeEnvironment(secrets modkit.Secrets) modkit.HookFunc {
	return func(ctx context.Context, req modkit.HookRequest) error {
		return finalizeEnvironmentRows(ctx, secrets, req)
	}
}

func finalizeEnvironmentRows(ctx context.Context, secrets modkit.Secrets, req modkit.HookRequest) error {
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
	if err := gwrepo.DeleteA2AProxySecrets(ctx, req.Tx, secrets, "org_id = ? AND environment_id = ?", req.OrgID, req.ID); err != nil {
		return err
	}
	for _, table := range environmentTables {
		if err := req.Tx.Exec("DELETE FROM "+table+" WHERE org_id = ? AND environment_id = ?", req.OrgID, req.ID).Error; err != nil {
			return fmt.Errorf("delete %s of environment %s: %w", table, req.ID, err)
		}
	}
	return nil
}
