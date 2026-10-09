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
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	maxWebhooksPerTarget    = 10
	maxWebhookNameLength    = 100
	maxWebhookDescLength    = 500
	webhookSecretBytes      = 32
	defaultWebhookLogLimit  = 50
	maxWebhookLogLimit      = 100
	webhookTestEventMessage = "Test event from WSO2 Agent Manager. No action is needed."
)

// WebhookEndpointInput is the input for creating or updating an endpoint.
type WebhookEndpointInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
	// Environments are the environments an agent endpoint receives events
	// of: at least one is required for agent endpoints, none for others.
	Environments []string `json:"environments"`
	EventTypes   []string `json:"eventTypes"`
	Enabled      bool     `json:"enabled"`
	// CreatedBy is the caller recorded on a new endpoint.
	CreatedBy string `json:"createdBy"`
}

// WebhookTestResult is the outcome of a test send.
type WebhookTestResult struct {
	Delivered  bool   `json:"delivered"`
	StatusCode int    `json:"statusCode,omitempty"`
	Error      string `json:"error,omitempty"`
}

// WebhookCleaner removes the webhooks of a deleted project or agent, so a
// project or agent created later under the same name does not inherit them.
type WebhookCleaner interface {
	RemoveForProject(ctx context.Context, ouID, projectName string)
	RemoveForAgent(ctx context.Context, ouID, projectName, agentName string)
}

// WebhookStore holds webhook endpoints and their delivery log. The local
// store keeps them in this process's database (the dispatcher's, or the
// service's in "all" mode); the remote store forwards to a dispatcher's
// internal API. Neither records an audit trail: WebhookService does, where
// the caller is known.
type WebhookStore interface {
	List(ctx context.Context, target repositories.WebhookTarget) ([]models.WebhookEndpoint, error)
	Get(ctx context.Context, target repositories.WebhookTarget, id uuid.UUID) (*models.WebhookEndpoint, error)
	// Create returns the new endpoint and its signing secret, which is shown
	// once and never again.
	Create(ctx context.Context, target repositories.WebhookTarget, in WebhookEndpointInput) (*models.WebhookEndpoint, string, error)
	Update(ctx context.Context, target repositories.WebhookTarget, id uuid.UUID, in WebhookEndpointInput) (*models.WebhookEndpoint, error)
	Delete(ctx context.Context, target repositories.WebhookTarget, id uuid.UUID) error
	// RotateSecret replaces the signing secret and returns the new one.
	RotateSecret(ctx context.Context, target repositories.WebhookTarget, id uuid.UUID) (string, error)
	// Test sends a signed webhook.test event straight to the endpoint.
	Test(ctx context.Context, target repositories.WebhookTarget, id uuid.UUID) (*WebhookTestResult, error)
	ListDeliveries(ctx context.Context, target repositories.WebhookTarget, id uuid.UUID, limit int) ([]models.WebhookDelivery, error)

	WebhookCleaner
}

// WebhookService manages webhook endpoints at org, project and agent scope
// for API callers: it records each change in the audit trail and delegates
// storage to a WebhookStore.
type WebhookService interface {
	EventTypes(scope string) ([]events.Type, error)
	WebhookStore
}

type localWebhookStore struct {
	repo          repositories.WebhookRepository
	sender        WebhookSender
	encryptionKey []byte
	logger        *slog.Logger
	now           func() time.Time
}

// NewLocalWebhookStore creates a store on this process's database.
func NewLocalWebhookStore(
	repo repositories.WebhookRepository, sender WebhookSender, encryptionKey []byte, logger *slog.Logger,
) WebhookStore {
	return &localWebhookStore{repo: repo, sender: sender, encryptionKey: encryptionKey, logger: logger, now: time.Now}
}

func (s *localWebhookStore) List(ctx context.Context, t repositories.WebhookTarget) ([]models.WebhookEndpoint, error) {
	out, err := s.repo.ListEndpoints(ctx, t)
	if err != nil {
		return nil, fmt.Errorf("failed to list webhooks: %w", err)
	}
	return out, nil
}

func (s *localWebhookStore) Get(ctx context.Context, t repositories.WebhookTarget, id uuid.UUID) (*models.WebhookEndpoint, error) {
	e, err := s.repo.GetEndpoint(ctx, t, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrWebhookNotFound
		}
		return nil, fmt.Errorf("failed to get webhook: %w", err)
	}
	return e, nil
}

func (s *localWebhookStore) Create(
	ctx context.Context, t repositories.WebhookTarget, in WebhookEndpointInput,
) (*models.WebhookEndpoint, string, error) {
	if err := s.validate(ctx, t, &in); err != nil {
		return nil, "", err
	}
	existing, err := s.repo.ListEndpoints(ctx, t)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list webhooks: %w", err)
	}
	if len(existing) >= maxWebhooksPerTarget {
		return nil, "", fmt.Errorf("%w: at most %d webhooks are allowed here", utils.ErrInvalidInput, maxWebhooksPerTarget)
	}
	secret, encrypted, err := s.newSecret()
	if err != nil {
		return nil, "", err
	}
	e := &models.WebhookEndpoint{
		ID:              uuid.New(),
		OUID:            t.OUID,
		Scope:           t.Scope,
		ProjectName:     t.ProjectName,
		AgentName:       t.AgentName,
		Environments:    in.Environments,
		Name:            in.Name,
		Description:     in.Description,
		URL:             in.URL,
		EventTypes:      in.EventTypes,
		Enabled:         in.Enabled,
		SecretEncrypted: encrypted,
		CreatedBy:       in.CreatedBy,
	}
	if err := s.repo.CreateEndpoint(ctx, e); err != nil {
		return nil, "", fmt.Errorf("failed to create webhook: %w", err)
	}
	return e, secret, nil
}

func (s *localWebhookStore) Update(
	ctx context.Context, t repositories.WebhookTarget, id uuid.UUID, in WebhookEndpointInput,
) (*models.WebhookEndpoint, error) {
	e, err := s.Get(ctx, t, id)
	if err != nil {
		return nil, err
	}
	if err := s.validate(ctx, t, &in); err != nil {
		return nil, err
	}
	e.Name, e.Description, e.URL = in.Name, in.Description, in.URL
	e.Environments, e.EventTypes, e.Enabled = in.Environments, in.EventTypes, in.Enabled
	if err := s.repo.UpdateEndpoint(ctx, e); err != nil {
		return nil, fmt.Errorf("failed to update webhook: %w", err)
	}
	return e, nil
}

func (s *localWebhookStore) Delete(ctx context.Context, t repositories.WebhookTarget, id uuid.UUID) error {
	deleted, err := s.repo.DeleteEndpoint(ctx, t, id)
	if err != nil {
		return fmt.Errorf("failed to delete webhook: %w", err)
	}
	if !deleted {
		return utils.ErrWebhookNotFound
	}
	return nil
}

func (s *localWebhookStore) RotateSecret(ctx context.Context, t repositories.WebhookTarget, id uuid.UUID) (string, error) {
	e, err := s.Get(ctx, t, id)
	if err != nil {
		return "", err
	}
	secret, encrypted, err := s.newSecret()
	if err != nil {
		return "", err
	}
	e.SecretEncrypted = encrypted
	if err := s.repo.UpdateEndpoint(ctx, e); err != nil {
		return "", fmt.Errorf("failed to rotate webhook secret: %w", err)
	}
	return secret, nil
}

func (s *localWebhookStore) Test(ctx context.Context, t repositories.WebhookTarget, id uuid.UUID) (*WebhookTestResult, error) {
	e, err := s.Get(ctx, t, id)
	if err != nil {
		return nil, err
	}
	target, err := webhookTargetFor(e, s.encryptionKey)
	if err != nil {
		return nil, err
	}
	evt := events.New("evt_"+uuid.NewString(), events.TypeWebhookTest, events.Scope(e.Scope),
		e.OUID, e.ProjectName, e.AgentName, s.now())
	evt.Subject = e.ID.String()
	evt.Data = map[string]any{"message": webhookTestEventMessage, "webhookId": e.ID.String()}
	body, err := json.Marshal(evt)
	if err != nil {
		return nil, fmt.Errorf("failed to encode test event: %w", err)
	}
	code, sendErr := s.sender.Send(ctx, target, evt.ID, evt.Type, body)
	result := &WebhookTestResult{Delivered: sendErr == nil, StatusCode: code}
	if sendErr != nil {
		result.Error = sendErr.Error()
	}
	return result, nil
}

func (s *localWebhookStore) ListDeliveries(
	ctx context.Context, t repositories.WebhookTarget, id uuid.UUID, limit int,
) ([]models.WebhookDelivery, error) {
	if _, err := s.Get(ctx, t, id); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = defaultWebhookLogLimit
	}
	out, err := s.repo.ListDeliveries(ctx, id, min(limit, maxWebhookLogLimit))
	if err != nil {
		return nil, fmt.Errorf("failed to list webhook deliveries: %w", err)
	}
	return out, nil
}

// validate checks and normalises an input for target's scope.
func (s *localWebhookStore) validate(ctx context.Context, t repositories.WebhookTarget, in *WebhookEndpointInput) error {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	in.URL = strings.TrimSpace(in.URL)
	if in.Name == "" || len(in.Name) > maxWebhookNameLength {
		return fmt.Errorf("%w: name is required and must be at most %d characters", utils.ErrInvalidInput, maxWebhookNameLength)
	}
	if len(in.Description) > maxWebhookDescLength {
		return fmt.Errorf("%w: description must be at most %d characters", utils.ErrInvalidInput, maxWebhookDescLength)
	}
	if err := s.sender.ValidateURL(ctx, in.URL); err != nil {
		return err
	}
	if t.Scope == models.WebhookScopeAgent {
		envs := make([]string, 0, len(in.Environments))
		seenEnv := make(map[string]bool, len(in.Environments))
		for _, env := range in.Environments {
			env = strings.TrimSpace(env)
			if env != "" && !seenEnv[env] {
				seenEnv[env] = true
				envs = append(envs, env)
			}
		}
		if len(envs) == 0 {
			return fmt.Errorf("%w: select at least one environment for an agent webhook", utils.ErrInvalidInput)
		}
		in.Environments = envs
	} else {
		in.Environments = []string{}
	}
	seen := make(map[string]bool, len(in.EventTypes))
	types := make([]string, 0, len(in.EventTypes))
	for _, name := range in.EventTypes {
		def, ok := events.Lookup(name)
		if !ok || string(def.Scope) != t.Scope {
			return fmt.Errorf("%w: %q is not a %s event", utils.ErrInvalidInput, name, t.Scope)
		}
		if !seen[name] {
			seen[name] = true
			types = append(types, name)
		}
	}
	in.EventTypes = types
	return nil
}

func (s *localWebhookStore) newSecret() (string, []byte, error) {
	buf := make([]byte, webhookSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, fmt.Errorf("failed to generate webhook secret: %w", err)
	}
	secret := WebhookSecretPrefix + base64.StdEncoding.EncodeToString(buf)
	encrypted, err := utils.EncryptBytes([]byte(secret), s.encryptionKey)
	if err != nil {
		return "", nil, fmt.Errorf("failed to encrypt webhook secret: %w", err)
	}
	return secret, encrypted, nil
}

// webhookTargetFor decrypts an endpoint's signing secret.
func webhookTargetFor(e *models.WebhookEndpoint, key []byte) (WebhookTarget, error) {
	secret, err := utils.DecryptBytes(e.SecretEncrypted, key)
	if err != nil {
		return WebhookTarget{}, fmt.Errorf("failed to decrypt webhook secret: %w", err)
	}
	return WebhookTarget{URL: e.URL, Secret: string(secret)}, nil
}

// webhookOrigin returns scheme://host for a URL.
func webhookOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// RemoveForProject deletes every webhook of a project (its own and its
// agents'). Errors are logged: the project is already gone.
func (s *localWebhookStore) RemoveForProject(ctx context.Context, ouID, projectName string) {
	if err := s.repo.DeleteEndpointsUnder(ctx, ouID, projectName, ""); err != nil {
		s.logger.Error("Failed to remove webhooks of a deleted project", "project", projectName, "error", err)
	}
}

// RemoveForAgent deletes an agent's webhooks. Errors are logged: the agent is
// already gone.
func (s *localWebhookStore) RemoveForAgent(ctx context.Context, ouID, projectName, agentName string) {
	if err := s.repo.DeleteEndpointsUnder(ctx, ouID, projectName, agentName); err != nil {
		s.logger.Error("Failed to remove webhooks of a deleted agent", "agent", agentName, "error", err)
	}
}
