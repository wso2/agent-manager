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

package thundersvc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
)

// agentConflictCode is the error code Thunder returns (HTTP 409) when an /agents
// create request reuses a name that already exists.
const agentConflictCode = "AGT-1013"

// CreateAgentIdentity creates a client_credentials AgentID in Thunder via the
// /agents endpoint — a distinct resource type from /applications, not a typo.
// ouID is the Thunder organization unit to assign the agent to; owner is the
// Thunder subject recorded as the agent's owner (may be empty).
//
// Idempotent: if an agent with this name already exists, Thunder returns 409
// AGT-1013; in that case the existing agent is looked up by name and returned
// with created=false and no secret (Thunder only returns a secret at creation).
func (c *thunderClient) CreateAgentIdentity(ctx context.Context, ouID, name, owner string) (thunderAgentID, clientID, clientSecret string, created bool, err error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return "", "", "", false, fmt.Errorf("failed to get system token: %w", err)
	}

	payload := map[string]any{
		"ouId": ouID,
		"type": "default",
		"name": name,
		"inboundAuthConfig": []map[string]any{
			{
				"type": "oauth2",
				"config": map[string]any{
					"grantTypes":              []string{"client_credentials"},
					"tokenEndpointAuthMethod": "client_secret_basic",
				},
			},
		},
	}
	if owner != "" {
		payload["owner"] = owner
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/agents", bytes.NewReader(body))
	if err != nil {
		return "", "", "", false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", "", false, fmt.Errorf("thunder create agent: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusConflict && bytes.Contains(respBody, []byte(agentConflictCode)) {
		existingID, existingClientID, findErr := c.findAgentByName(ctx, token, name)
		if findErr != nil {
			return "", "", "", false, findErr
		}
		if existingID == "" {
			return "", "", "", false, fmt.Errorf("thunder reported agent %q already exists but it could not be found by name", name)
		}
		return existingID, existingClientID, "", false, nil
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", "", "", false, fmt.Errorf("thunder create agent returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID          string `json:"id"`
		InboundAuth []struct {
			Config struct {
				ClientID     string `json:"clientId"`
				ClientSecret string `json:"clientSecret"`
			} `json:"config"`
		} `json:"inboundAuthConfig"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", "", "", false, fmt.Errorf("thunder create agent decode: %w", err)
	}
	if result.ID == "" {
		return "", "", "", false, fmt.Errorf("thunder create agent: id not found in response: %s", string(respBody))
	}
	if len(result.InboundAuth) == 0 || result.InboundAuth[0].Config.ClientID == "" {
		return "", "", "", false, fmt.Errorf("thunder create agent: clientId not found in response: %s", string(respBody))
	}

	logger.GetLogger(ctx).Info("Thunder agent identity created", "name", name, "thunder_agent_id", result.ID)
	return result.ID, result.InboundAuth[0].Config.ClientID, result.InboundAuth[0].Config.ClientSecret, true, nil
}

// findAgentByName looks up an existing agent by exact name match, paginating
// through /agents. Returns empty strings if no agent with that name exists.
func (c *thunderClient) findAgentByName(ctx context.Context, token, name string) (thunderAgentID, clientID string, err error) {
	return c.findResourceByName(ctx, token, "agents", name)
}

// RegenerateAgentSecret generates a new client secret and applies it to the
// existing AgentID. Thunder's /agents API has no dedicated regenerate endpoint —
// PUT only auto-regenerates a secret when transitioning from a non-secret auth
// method, which does not apply to an already-secret-based client_credentials
// agent — so this explicitly supplies the new value, exactly like
// RegenerateClientSecret does for /applications.
func (c *thunderClient) RegenerateAgentSecret(ctx context.Context, thunderAgentID string) (string, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get system token: %w", err)
	}

	secret, err := c.regenerateResourceSecret(ctx, token, "agents", thunderAgentID)
	if err != nil {
		return "", err
	}

	logger.GetLogger(ctx).Info("Thunder agent client secret regenerated", "thunder_agent_id", thunderAgentID)
	return secret, nil
}

// DeleteAgentIdentity deletes the AgentID by its Thunder internal ID.
// Returns false (no error) if it did not exist — deletion is idempotent.
func (c *thunderClient) DeleteAgentIdentity(ctx context.Context, thunderAgentID string) (bool, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get system token: %w", err)
	}
	return c.deleteThunderResource(ctx, token, "agents", thunderAgentID)
}

// GetDefaultOUID returns the default organization unit ID from Thunder.
// Exported wrapper around getDefaultOUID for callers outside this package
// (e.g. the AgentID provisioning service) that already hold a system token
// indirectly via this client but need the OU ID to create an agent.
func (c *thunderClient) GetDefaultOUID(ctx context.Context) (string, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get system token: %w", err)
	}
	return c.getDefaultOUID(ctx, token)
}
