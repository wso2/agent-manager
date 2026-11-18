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

package gwsvc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// An agent's LLM and MCP bindings come in its extensions.ai-gateway block.
// The agent's hooks validate the block and store it with the agent; what a
// binding provisions (an LLM proxy deployed to a gateway, MCP mappings, their
// API keys) talks to gateways and runs in the background, in the module's
// agent.created and bindings-changed handlers, which converge on the stored
// rows. When the agent goes, its finalize hook records a teardown row and the
// agent.deleted handler revokes what the bindings and the agent's exposure
// hold, retrying until it is done.

// Binding states.
const (
	BindingPending  = "pending"
	BindingReady    = "ready"
	BindingFailed   = "failed"
	BindingRemoving = "removing"
)

const (
	bindingKindLLM = "llm"
	bindingKindMCP = "mcp"
)

// AgentExtension is the extensions.ai-gateway block of an agent create or
// update. A list that is absent leaves the agent's bindings of that kind alone.
type AgentExtension struct {
	ModelConfig *[]gwspec.ModelConfigRequest `json:"modelConfig,omitempty"`
	McpConfig   *[]gwspec.MCPConfigRequest   `json:"mcpConfig,omitempty"`
	Settings    *gwspec.AgentGatewaySettings `json:"settings,omitempty"`
}

// DecodeAgentExtension reads the block, refusing fields it does not know.
func DecodeAgentExtension(raw json.RawMessage) (AgentExtension, error) {
	var ext AgentExtension
	if len(raw) == 0 || string(raw) == "null" {
		return ext, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ext); err != nil {
		return ext, modkit.Reject("extensions.ai-gateway: %v", err)
	}
	return ext, nil
}

// AgentBinding is a row of mod_ai_gateway_agent_bindings.
type AgentBinding struct {
	ID        uuid.UUID       `gorm:"column:id;primaryKey"`
	OrgID     uuid.UUID       `gorm:"column:org_id"`
	AgentID   uuid.UUID       `gorm:"column:agent_id"`
	Kind      string          `gorm:"column:kind"`
	Position  int             `gorm:"column:position"`
	Target    string          `gorm:"column:target"`
	Request   json.RawMessage `gorm:"column:request;serializer:json"`
	State     string          `gorm:"column:state"`
	LastError *string         `gorm:"column:last_error"`
	ConfigID  *uuid.UUID      `gorm:"column:config_id"`
	CreatedAt time.Time       `gorm:"column:created_at"`
	UpdatedAt time.Time       `gorm:"column:updated_at"`
}

// TableName is the module's table.
func (AgentBinding) TableName() string { return "mod_ai_gateway_agent_bindings" }

// AgentTeardown is a row of mod_ai_gateway_agent_teardowns.
type AgentTeardown struct {
	AgentID     uuid.UUID `gorm:"column:agent_id;primaryKey"`
	OrgID       uuid.UUID `gorm:"column:org_id"`
	ProjectName string    `gorm:"column:project_name"`
	AgentName   string    `gorm:"column:agent_name"`
	LastError   *string   `gorm:"column:last_error"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

// TableName is the module's table.
func (AgentTeardown) TableName() string { return "mod_ai_gateway_agent_teardowns" }

// TeardownTables hold rows of an agent that its teardown revokes before
// deleting them: the configurations and the exposure, card and proxy rows.
var TeardownTables = []string{
	"mod_ai_gateway_agent_model_configs",
	"mod_ai_gateway_ai_applications",
	"mod_ai_gateway_agent_exposures",
	"mod_ai_gateway_agent_cards",
	"mod_ai_gateway_a2a_proxies",
}

// FinalizeTables hold rows of an agent that go in its delete transaction.
var FinalizeTables = []string{
	"mod_ai_gateway_agent_bindings",
	"mod_ai_gateway_agent_gateway_settings",
}

// bindingRequest is one requested binding, as stored.
type bindingRequest struct {
	target  string
	request json.RawMessage
}

func llmRequests(items []gwspec.ModelConfigRequest) ([]bindingRequest, error) {
	out := make([]bindingRequest, 0, len(items))
	for _, mc := range items {
		raw, err := json.Marshal(mc)
		if err != nil {
			return nil, err
		}
		out = append(out, bindingRequest{target: mc.ProviderName, request: raw})
	}
	return out, nil
}

func mcpRequests(items []gwspec.MCPConfigRequest) ([]bindingRequest, error) {
	out := make([]bindingRequest, 0, len(items))
	for _, mc := range items {
		raw, err := json.Marshal(mc)
		if err != nil {
			return nil, err
		}
		out = append(out, bindingRequest{target: mc.ProxyName, request: raw})
	}
	return out, nil
}

// ValidateAgentExtension is the agent validate hook's work. It reads the
// catalog outside tx: plain reads do not wait for the core's transaction.
func (s *AgentGatewayService) ValidateAgentExtension(ctx context.Context, tx *gorm.DB, req modkit.HookRequest, ext AgentExtension) error {
	project, _ := req.Object["projectName"].(string)
	name, _ := req.Object["name"].(string)
	if req.Action == modkit.ActionCreate {
		var pending int64
		if err := tx.Model(&AgentTeardown{}).Where("org_id = ? AND project_name = ? AND agent_name = ?", req.OrgID, project, name).
			Count(&pending).Error; err != nil {
			return err
		}
		if pending > 0 {
			return &modkit.Error{Status: http.StatusConflict, Message: fmt.Sprintf(
				"the AI Gateway is still revoking what a deleted agent named %q in project %q held; retry shortly or use another name", name, project)}
		}
	} else if ext.Settings != nil {
		return modkit.Reject("extensions.ai-gateway.settings is accepted on create only; change the settings with PUT /m/ai-gateway/orgs/{orgName}/projects/{projName}/agents/{agentName}/deploy-settings")
	}
	if ext.ModelConfig != nil {
		handles := make([]string, 0, len(*ext.ModelConfig))
		for _, mc := range *ext.ModelConfig {
			handles = append(handles, mc.ProviderName)
		}
		if err := s.ValidateProviders(ctx, req.OrgID, handles); err != nil {
			return rejectBinding("modelConfig", err)
		}
	}
	if ext.McpConfig != nil {
		handles := make([]string, 0, len(*ext.McpConfig))
		for _, mc := range *ext.McpConfig {
			handles = append(handles, mc.ProxyName)
		}
		if err := s.ValidateProxies(ctx, req.OrgID, project, handles); err != nil {
			return rejectBinding("mcpConfig", err)
		}
	}
	if st := ext.Settings; st != nil {
		if st.GetEnableApiKeySecurity() && st.GetEnableOAuthSecurity() {
			return modkit.Reject("extensions.ai-gateway.settings: enableApiKeySecurity and enableOAuthSecurity are mutually exclusive")
		}
		if _, err := resolveResilienceTimeoutSeconds(nil, st.ResilienceTimeoutSeconds, true); err != nil {
			return modkit.Reject("extensions.ai-gateway.settings: %v", err)
		}
	}
	return nil
}

// rejectBinding turns a refusal of a requested provider or proxy into a 400;
// any other error stays a server error.
func rejectBinding(field string, err error) error {
	for _, want := range []error{utils.ErrInvalidInput, utils.ErrLLMProviderNotFound, utils.ErrMCPProxyNotFound} {
		if errors.Is(err, want) {
			return modkit.Reject("extensions.ai-gateway.%s: %v", field, err)
		}
	}
	var verr *utils.ValidationError
	if errors.As(err, &verr) {
		return modkit.Reject("extensions.ai-gateway.%s: %s", field, verr.Message)
	}
	return err
}

// StoreAgentExtension is the agent apply hook's work: it writes the requested
// bindings, and on create the settings, in the agent's transaction. It reports
// whether the bindings changed, so the caller can have them reconciled.
func (s *AgentGatewayService) StoreAgentExtension(ctx context.Context, tx *gorm.DB, req modkit.HookRequest, ext AgentExtension) (bool, error) {
	orgID, err := uuid.Parse(req.OrgID)
	if err != nil {
		return false, err
	}
	agentID, err := uuid.Parse(req.ID)
	if err != nil {
		return false, err
	}
	changed := false
	if ext.ModelConfig != nil {
		reqs, err := llmRequests(*ext.ModelConfig)
		if err != nil {
			return false, err
		}
		c, err := replaceBindings(tx, orgID, agentID, bindingKindLLM, reqs)
		if err != nil {
			return false, err
		}
		changed = changed || c
	}
	if ext.McpConfig != nil {
		reqs, err := mcpRequests(*ext.McpConfig)
		if err != nil {
			return false, err
		}
		c, err := replaceBindings(tx, orgID, agentID, bindingKindMCP, reqs)
		if err != nil {
			return false, err
		}
		changed = changed || c
	}
	if st := ext.Settings; st != nil && req.Action == modkit.ActionCreate {
		project, _ := req.Object["projectName"].(string)
		name, _ := req.Object["name"].(string)
		timeout, err := resolveResilienceTimeoutSeconds(nil, st.ResilienceTimeoutSeconds, true)
		if err != nil {
			return false, modkit.Reject("extensions.ai-gateway.settings: %v", err)
		}
		api := resolveAPIConfig(nil, st.EnableApiKeySecurity, st.CorsConfig, st.EnableOAuthSecurity, st.OauthConfig, true)
		cfg := agentConfigFromAPIConfig(req.OrgID, project, name, models.DefaultEnvironmentName, api)
		cfg.EnableAutoInstrumentation = resolveTracingConfig(nil, nil, true).EnableAutoInstrumentation
		cfg.ResilienceTimeoutSeconds = &timeout
		if err := gwrepo.NewAgentConfigRepo(tx).Upsert(ctx, cfg); err != nil {
			return false, fmt.Errorf("store gateway settings: %w", err)
		}
	}
	return changed, nil
}

// replaceBindings makes the agent's bindings of one kind the requested ones.
// A binding requested again keeps its row (a failed one is retried); one no
// longer requested is marked for removal when it provisioned something, and
// deleted when it did not.
func replaceBindings(tx *gorm.DB, orgID, agentID uuid.UUID, kind string, reqs []bindingRequest) (bool, error) {
	var existing []AgentBinding
	if err := tx.Where("org_id = ? AND agent_id = ? AND kind = ? AND state <> ?", orgID, agentID, kind, BindingRemoving).
		Order("position").Find(&existing).Error; err != nil {
		return false, err
	}
	used := make([]bool, len(existing))
	changed := false
	now := time.Now().UTC()
	for pos, r := range reqs {
		match := -1
		for i, b := range existing {
			if !used[i] && b.Target == r.target && bytes.Equal(b.Request, r.request) {
				match = i
				break
			}
		}
		if match < 0 {
			changed = true
			row := AgentBinding{
				ID: uuid.New(), OrgID: orgID, AgentID: agentID, Kind: kind, Position: pos, Target: r.target,
				Request: r.request, State: BindingPending, CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.Create(&row).Error; err != nil {
				return false, err
			}
			continue
		}
		used[match] = true
		updates := map[string]any{"position": pos, "updated_at": now}
		if existing[match].State == BindingFailed {
			changed = true
			updates["state"] = BindingPending
		}
		if err := tx.Model(&AgentBinding{}).Where("id = ?", existing[match].ID).Updates(updates).Error; err != nil {
			return false, err
		}
	}
	for i, b := range existing {
		if used[i] {
			continue
		}
		changed = true
		q := tx.Model(&AgentBinding{}).Where("id = ?", b.ID)
		if b.ConfigID == nil {
			if err := tx.Delete(&AgentBinding{}, "id = ?", b.ID).Error; err != nil {
				return false, err
			}
			continue
		}
		if err := q.Updates(map[string]any{"state": BindingRemoving, "updated_at": now}).Error; err != nil {
			return false, err
		}
	}
	return changed, nil
}

// AgentBindings lists an agent's bindings, LLM first, in request order.
func (s *AgentGatewayService) AgentBindings(ctx context.Context, ouID, projectName, agentName string) ([]AgentBinding, error) {
	agentID, err := models.AgentIDByNames(s.db.WithContext(ctx), ouID, projectName, agentName)
	if err != nil {
		if errors.Is(err, models.ErrAgentRefNotFound) {
			return nil, utils.ErrAgentNotFound
		}
		return nil, err
	}
	var rows []AgentBinding
	err = s.db.WithContext(ctx).Where("org_id = ? AND agent_id = ?", ouID, agentID).Order("kind, position, created_at").Find(&rows).Error
	return rows, err
}

// ReconcileAgentBindings provisions the agent's pending and failed bindings
// and removes the ones marked for removal. It is idempotent: a binding that
// provisioned a configuration before its state could be recorded finds it
// again by name. It returns an error while any binding is not done, so the
// event that triggered it is retried.
func (s *AgentGatewayService) ReconcileAgentBindings(ctx context.Context, orgID, agentID string) error {
	db := s.db.WithContext(ctx)
	var gone int64
	if err := db.Model(&AgentTeardown{}).Where("agent_id = ?", agentID).Count(&gone).Error; err != nil {
		return err
	}
	if gone > 0 {
		return nil // the agent was deleted; its teardown revokes what was provisioned
	}
	aid, err := uuid.Parse(agentID)
	if err != nil {
		return err
	}
	projectName, agentName, err := gwmodels.AgentNamesByID(db, aid)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	var rows []AgentBinding
	if err := db.Where("org_id = ? AND agent_id = ?", orgID, agentID).Order("kind, position").Find(&rows).Error; err != nil {
		return err
	}
	var errs []error
	for _, b := range rows {
		switch {
		case b.State == BindingRemoving:
			if err := s.removeBinding(ctx, orgID, projectName, agentName, b); err != nil {
				errs = append(errs, err)
			}
		case b.State != BindingReady || b.ConfigID == nil:
			if err := s.provisionBinding(ctx, orgID, projectName, agentName, b); err != nil {
				errs = append(errs, fmt.Errorf("%s binding %s: %w", b.Kind, b.Target, err))
			}
		}
	}
	return errors.Join(errs...)
}

// bindingConfigName names the configuration a binding provisions, stable
// across retries.
func bindingConfigName(agentName string, b AgentBinding) string {
	return fmt.Sprintf("%s-%s-config-%s", agentName, b.Kind, b.ID.String()[:8])
}

func (s *AgentGatewayService) provisionBinding(ctx context.Context, orgID, projectName, agentName string, b AgentBinding) error {
	db := s.db.WithContext(ctx)
	configID, err := s.findConfig(ctx, orgID, projectName, agentName, bindingConfigName(agentName, b))
	if err == nil && configID == "" {
		configID, err = s.createBindingConfig(ctx, orgID, projectName, agentName, b)
	}
	if err != nil {
		msg := err.Error()
		if len(msg) > 2000 {
			msg = msg[:2000]
		}
		if uerr := db.Model(&AgentBinding{}).Where("id = ? AND state IN ?", b.ID, []string{BindingPending, BindingFailed, BindingReady}).
			Updates(map[string]any{"state": BindingFailed, "last_error": msg, "updated_at": time.Now().UTC()}).Error; uerr != nil {
			s.logger.Error("Failed to record a binding failure", "binding", b.ID, "error", uerr)
		}
		return err
	}
	cid, err := uuid.Parse(configID)
	if err != nil {
		return err
	}
	res := db.Model(&AgentBinding{}).Where("id = ? AND state IN ?", b.ID, []string{BindingPending, BindingFailed, BindingReady}).
		Updates(map[string]any{"state": BindingReady, "config_id": cid, "last_error": nil, "updated_at": time.Now().UTC()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		return nil
	}
	// The binding was dropped while it was provisioned: keep the configuration
	// on a row marked for removal, or revoke it when the row is gone.
	res = db.Model(&AgentBinding{}).Where("id = ? AND config_id IS NULL", b.ID).Update("config_id", cid)
	if res.Error != nil || res.RowsAffected > 0 {
		return res.Error
	}
	return s.deleteConfig(ctx, orgID, projectName, agentName, cid)
}

func (s *AgentGatewayService) createBindingConfig(ctx context.Context, orgID, projectName, agentName string, b AgentBinding) (string, error) {
	name := bindingConfigName(agentName, b)
	switch b.Kind {
	case bindingKindLLM:
		var mc gwspec.ModelConfigRequest
		if err := json.Unmarshal(b.Request, &mc); err != nil {
			return "", err
		}
		return s.createLLMConfig(ctx, orgID, projectName, agentName, models.DefaultEnvironmentName, name, mc)
	case bindingKindMCP:
		var mc gwspec.MCPConfigRequest
		if err := json.Unmarshal(b.Request, &mc); err != nil {
			return "", err
		}
		return s.createMCPConfig(ctx, orgID, projectName, agentName, []string{models.DefaultEnvironmentName}, name, mc)
	}
	return "", fmt.Errorf("unknown binding kind %q", b.Kind)
}

func (s *AgentGatewayService) findConfig(ctx context.Context, orgID, projectName, agentName, name string) (string, error) {
	list, err := s.agentConfigurationService.List(ctx, orgID, projectName, agentName, 1000, 0)
	if err != nil {
		return "", err
	}
	for _, c := range list.Configs {
		if c.Name == name {
			return c.UUID, nil
		}
	}
	return "", nil
}

func (s *AgentGatewayService) deleteConfig(ctx context.Context, orgID, projectName, agentName string, configID uuid.UUID) error {
	err := s.agentConfigurationService.Delete(ctx, configID, orgID, projectName, agentName)
	if errors.Is(err, utils.ErrAgentConfigNotFound) {
		return nil
	}
	return err
}

func (s *AgentGatewayService) removeBinding(ctx context.Context, orgID, projectName, agentName string, b AgentBinding) error {
	if b.ConfigID != nil {
		if err := s.deleteConfig(ctx, orgID, projectName, agentName, *b.ConfigID); err != nil {
			return fmt.Errorf("remove %s binding %s: %w", b.Kind, b.Target, err)
		}
	}
	return s.db.WithContext(ctx).Delete(&AgentBinding{}, "id = ? AND state = ?", b.ID, BindingRemoving).Error
}

// RecordAgentTeardown is the agent finalize hook's work: it keeps the agent's
// names for its teardown and deletes its bindings and settings.
func RecordAgentTeardown(tx *gorm.DB, req modkit.HookRequest) error {
	project, _ := req.Object["projectName"].(string)
	name, _ := req.Object["name"].(string)
	now := time.Now().UTC()
	row := AgentTeardown{
		AgentID: uuid.MustParse(req.ID), OrgID: uuid.MustParse(req.OrgID),
		ProjectName: project, AgentName: name, CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.Create(&row).Error; err != nil {
		return fmt.Errorf("record teardown of agent %s: %w", req.ID, err)
	}
	for _, table := range FinalizeTables {
		if err := tx.Exec("DELETE FROM "+table+" WHERE org_id = ? AND agent_id = ?", req.OrgID, req.ID).Error; err != nil {
			return fmt.Errorf("delete %s of agent %s: %w", table, req.ID, err)
		}
	}
	return nil
}

// TeardownAgent is the agent.deleted handler's work: it revokes the deleted
// agent's configurations and withdraws its exposure from the gateways, then
// deletes its remaining rows and the teardown row. It returns an error while
// anything is left, so the event is retried.
func (s *AgentGatewayService) TeardownAgent(ctx context.Context, agentID string) error {
	db := s.db.WithContext(ctx)
	var td AgentTeardown
	err := db.Where("agent_id = ?", agentID).Take(&td).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	orgID := td.OrgID.String()
	err = errors.Join(
		s.revokeAgentConfigurations(ctx, orgID, td.ProjectName, td.AgentName),
		s.deleteAgentAPIArtifact(ctx, orgID, td.ProjectName, td.AgentName, true),
	)
	if err != nil {
		msg := err.Error()
		if len(msg) > 2000 {
			msg = msg[:2000]
		}
		if uerr := db.Model(&AgentTeardown{}).Where("agent_id = ?", agentID).
			Updates(map[string]any{"last_error": msg, "updated_at": time.Now().UTC()}).Error; uerr != nil {
			s.logger.Error("Failed to record a teardown failure", "agentID", agentID, "error", uerr)
		}
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, table := range TeardownTables {
			if err := tx.Exec("DELETE FROM "+table+" WHERE org_id = ? AND agent_id = ?", orgID, agentID).Error; err != nil {
				return fmt.Errorf("delete %s of agent %s: %w", table, agentID, err)
			}
		}
		return tx.Delete(&AgentTeardown{}, "agent_id = ?", agentID).Error
	})
}

// AgentConfigurations reads an agent's gateway settings in an environment
// (nil when none are stored yet) and its bindings.
func (s *AgentGatewayService) AgentConfigurations(ctx context.Context, ouID, projectName, agentName, environment string) (*gwmodels.AgentConfig, []AgentBinding, error) {
	if _, err := services.GetProjectAgent(ctx, s.platform, ouID, projectName, agentName); err != nil {
		return nil, nil, err
	}
	cfg, err := s.agentConfigRepo.Get(ctx, ouID, projectName, agentName, environment)
	if errors.Is(err, gwrepo.ErrAgentConfigNotFound) {
		cfg, err = nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read gateway settings: %w", err)
	}
	bindings, err := s.AgentBindings(ctx, ouID, projectName, agentName)
	if err != nil {
		return nil, nil, err
	}
	return cfg, bindings, nil
}
