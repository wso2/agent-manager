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

package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// A test send waits for the endpoint (up to webhookSendTimeout), so the
// client allows more than that.
const remoteWebhookTimeout = webhookSendTimeout + 5*time.Second

// remoteWebhookStore is the WebhookStore of an "api" instance: every call
// goes to the dispatcher's internal API, which owns the webhook database.
type remoteWebhookStore struct {
	baseURL string
	apiKey  string
	client  *http.Client
	logger  *slog.Logger
}

// NewRemoteWebhookStore creates a store backed by the dispatcher at baseURL.
func NewRemoteWebhookStore(baseURL, apiKey string, logger *slog.Logger) WebhookStore {
	return &remoteWebhookStore{
		baseURL: baseURL + webhookInternalBase,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: remoteWebhookTimeout},
		logger:  logger,
	}
}

func targetQuery(t repositories.WebhookTarget) url.Values {
	q := url.Values{"ouId": {t.OUID}, "scope": {t.Scope}}
	if t.ProjectName != "" {
		q.Set("project", t.ProjectName)
	}
	if t.AgentName != "" {
		q.Set("agent", t.AgentName)
	}
	return q
}

// call sends a request and decodes a 2xx body into out. Error answers map
// back to the errors the local store returns.
func (s *remoteWebhookStore) call(ctx context.Context, method, path string, q url.Values, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("failed to encode dispatcher request: %w", err)
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.baseURL+path+"?"+q.Encode(), body)
	if err != nil {
		return fmt.Errorf("failed to build dispatcher request: %w", err)
	}
	req.Header.Set(WebhookInternalAPIKeyHeader, s.apiKey)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook dispatcher unreachable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxInternalRequestBytes))

	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		if out == nil || len(raw) == 0 {
			return nil
		}
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("failed to decode dispatcher response: %w", err)
		}
		return nil
	}
	var apiErr webhookAPIError
	_ = json.Unmarshal(raw, &apiErr)
	switch apiErr.Code {
	case webhookErrNotFound:
		return utils.ErrWebhookNotFound
	case webhookErrInvalidInput:
		if resp.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("webhook dispatcher rejected the API key")
		}
		return fmt.Errorf("%w: %s", utils.ErrInvalidInput, trimInvalidPrefix(apiErr.Message))
	default:
		return fmt.Errorf("webhook dispatcher returned HTTP %d", resp.StatusCode)
	}
}

// trimInvalidPrefix drops the "invalid input: " prefix the dispatcher's error
// already carries, so it is not repeated when wrapped again here.
func trimInvalidPrefix(msg string) string {
	prefix := utils.ErrInvalidInput.Error() + ": "
	if len(msg) > len(prefix) && msg[:len(prefix)] == prefix {
		return msg[len(prefix):]
	}
	return msg
}

func (s *remoteWebhookStore) List(ctx context.Context, t repositories.WebhookTarget) ([]models.WebhookEndpoint, error) {
	var out []models.WebhookEndpoint
	err := s.call(ctx, http.MethodGet, "", targetQuery(t), nil, &out)
	return out, err
}

func (s *remoteWebhookStore) Get(ctx context.Context, t repositories.WebhookTarget, id uuid.UUID) (*models.WebhookEndpoint, error) {
	var out models.WebhookEndpoint
	if err := s.call(ctx, http.MethodGet, "/"+id.String(), targetQuery(t), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *remoteWebhookStore) Create(
	ctx context.Context, t repositories.WebhookTarget, in WebhookEndpointInput,
) (*models.WebhookEndpoint, string, error) {
	var out webhookCreateResponse
	if err := s.call(ctx, http.MethodPost, "", targetQuery(t), in, &out); err != nil {
		return nil, "", err
	}
	return &out.Webhook, out.SigningSecret, nil
}

func (s *remoteWebhookStore) Update(
	ctx context.Context, t repositories.WebhookTarget, id uuid.UUID, in WebhookEndpointInput,
) (*models.WebhookEndpoint, error) {
	var out models.WebhookEndpoint
	if err := s.call(ctx, http.MethodPut, "/"+id.String(), targetQuery(t), in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *remoteWebhookStore) Delete(ctx context.Context, t repositories.WebhookTarget, id uuid.UUID) error {
	return s.call(ctx, http.MethodDelete, "/"+id.String(), targetQuery(t), nil, nil)
}

func (s *remoteWebhookStore) RotateSecret(ctx context.Context, t repositories.WebhookTarget, id uuid.UUID) (string, error) {
	var out webhookSecretResponse
	if err := s.call(ctx, http.MethodPost, "/"+id.String()+"/rotate-secret", targetQuery(t), nil, &out); err != nil {
		return "", err
	}
	return out.SigningSecret, nil
}

func (s *remoteWebhookStore) Test(ctx context.Context, t repositories.WebhookTarget, id uuid.UUID) (*WebhookTestResult, error) {
	var out WebhookTestResult
	if err := s.call(ctx, http.MethodPost, "/"+id.String()+"/test", targetQuery(t), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *remoteWebhookStore) ListDeliveries(
	ctx context.Context, t repositories.WebhookTarget, id uuid.UUID, limit int,
) ([]models.WebhookDelivery, error) {
	q := targetQuery(t)
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var out []models.WebhookDelivery
	err := s.call(ctx, http.MethodGet, "/"+id.String()+"/deliveries", q, nil, &out)
	return out, err
}

func (s *remoteWebhookStore) RemoveForProject(ctx context.Context, ouID, projectName string) {
	s.removeUnder(ctx, url.Values{"ouId": {ouID}, "project": {projectName}})
}

func (s *remoteWebhookStore) RemoveForAgent(ctx context.Context, ouID, projectName, agentName string) {
	s.removeUnder(ctx, url.Values{"ouId": {ouID}, "project": {projectName}, "agent": {agentName}})
}

func (s *remoteWebhookStore) removeUnder(ctx context.Context, q url.Values) {
	if err := s.call(ctx, http.MethodDelete, "", q, nil, nil); err != nil {
		s.logger.Error("Failed to remove webhooks of a deleted resource", "project", q.Get("project"),
			"agent", q.Get("agent"), "error", err)
	}
}
