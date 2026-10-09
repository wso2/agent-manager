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
	"fmt"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// auditedWebhookService is the WebhookService API callers use. Each change is
// recorded fail-closed in the audit trail, where the caller is known, before
// the store applies it; the store may be local or the dispatcher's.
type auditedWebhookService struct {
	WebhookStore
}

// NewWebhookService wraps a store with the audit trail.
func NewWebhookService(store WebhookStore) WebhookService {
	return &auditedWebhookService{WebhookStore: store}
}

func (s *auditedWebhookService) EventTypes(scope string) ([]events.Type, error) {
	if scope != "" && !events.Scope(scope).Valid() {
		return nil, fmt.Errorf("%w: scope must be org, project or agent", utils.ErrInvalidInput)
	}
	return events.Types(events.Scope(scope)), nil
}

func (s *auditedWebhookService) Create(
	ctx context.Context, t repositories.WebhookTarget, in WebhookEndpointInput,
) (*models.WebhookEndpoint, string, error) {
	in.CreatedBy = audit.BuildEvent(ctx, audit.ActionWebhookCreate).ActorID
	attempt, err := audit.Begin(ctx, audit.ActionWebhookCreate,
		webhookAuditOpts(t, "", in.Name, in.URL, len(in.EventTypes), in.Enabled, in.Environments)...)
	if err != nil {
		return nil, "", err
	}
	e, secret, err := s.WebhookStore.Create(ctx, t, in)
	if err != nil {
		attempt.Complete(ctx, err)
		return nil, "", err
	}
	attempt.Complete(ctx, nil, audit.ResourceNamed(audit.ResourceWebhook, e.ID.String(), e.Name))
	return e, secret, nil
}

func (s *auditedWebhookService) Update(
	ctx context.Context, t repositories.WebhookTarget, id uuid.UUID, in WebhookEndpointInput,
) (*models.WebhookEndpoint, error) {
	attempt, err := audit.Begin(ctx, audit.ActionWebhookUpdate,
		webhookAuditOpts(t, id.String(), in.Name, in.URL, len(in.EventTypes), in.Enabled, in.Environments)...)
	if err != nil {
		return nil, err
	}
	e, err := s.WebhookStore.Update(ctx, t, id, in)
	attempt.Complete(ctx, err)
	return e, err
}

func (s *auditedWebhookService) Delete(ctx context.Context, t repositories.WebhookTarget, id uuid.UUID) error {
	e, err := s.Get(ctx, t, id)
	if err != nil {
		return err
	}
	attempt, err := audit.Begin(ctx, audit.ActionWebhookDelete,
		webhookAuditOpts(t, e.ID.String(), e.Name, e.URL, len(e.EventTypes), e.Enabled, e.Environments)...)
	if err != nil {
		return err
	}
	err = s.WebhookStore.Delete(ctx, t, id)
	attempt.Complete(ctx, err)
	return err
}

func (s *auditedWebhookService) RotateSecret(ctx context.Context, t repositories.WebhookTarget, id uuid.UUID) (string, error) {
	e, err := s.Get(ctx, t, id)
	if err != nil {
		return "", err
	}
	attempt, err := audit.Begin(ctx, audit.ActionWebhookRotateSecret,
		webhookAuditOpts(t, e.ID.String(), e.Name, e.URL, len(e.EventTypes), e.Enabled, e.Environments)...)
	if err != nil {
		return "", err
	}
	secret, err := s.WebhookStore.RotateSecret(ctx, t, id)
	attempt.Complete(ctx, err)
	return secret, err
}

func webhookAuditOpts(
	t repositories.WebhookTarget, id, name, rawURL string, eventCount int, enabled bool, envs []string,
) []audit.Option {
	opts := []audit.Option{
		audit.Org(t.OUID),
		audit.Detail("scope", t.Scope),
		// Scheme and host only: a webhook path is often a credential.
		audit.Detail("endpointUrl", webhookOrigin(rawURL)),
		audit.Detail("eventCount", eventCount),
		audit.Detail("enabled", enabled),
	}
	if id != "" {
		opts = append(opts, audit.ResourceNamed(audit.ResourceWebhook, id, name))
	}
	if t.ProjectName != "" {
		opts = append(opts, audit.Project(t.ProjectName))
	}
	if len(envs) > 0 {
		opts = append(opts, audit.Detail("environments", envs))
	}
	return opts
}
