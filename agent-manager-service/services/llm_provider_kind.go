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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// renderedDeployment names a rendered artifact after its handle and its content, so two renders of the same
// configuration share a name and a changed one gets a new one.
func renderedDeployment(handle string, content []byte) reconcile.Rendered {
	sum := sha256.Sum256(content)
	return reconcile.Rendered{Name: handle + "-" + hex.EncodeToString(sum[:4]), Content: content}
}

// LLMProviderKind deploys LLM providers. Gateways store a provider under its UUID.
type LLMProviderKind struct {
	load func(ctx context.Context, artifact models.ArtifactRef) (*models.LLMProvider, error)
}

// NewLLMProviderKind reads providers through the request's transaction when there is one.
func NewLLMProviderKind() *LLMProviderKind {
	return &LLMProviderKind{load: func(ctx context.Context, artifact models.ArtifactRef) (*models.LLMProvider, error) {
		return repositories.NewLLMProviderRepo(db.DB(ctx)).GetByUUID(artifact.ArtifactUUID.String(), artifact.OUID)
	}}
}

func (k *LLMProviderKind) Name() string        { return models.KindLLMProvider }
func (k *LLMProviderKind) RendersInline() bool { return true }

func (k *LLMProviderKind) GatewayArtifactID(artifact models.ArtifactRef) string {
	return artifact.ArtifactUUID.String()
}

func (k *LLMProviderKind) Render(ctx context.Context, artifact models.ArtifactRef) (reconcile.Rendered, error) {
	provider, err := k.load(ctx, artifact)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return reconcile.Rendered{}, reconcile.Permanent(fmt.Errorf("LLM provider %s not found", artifact.ArtifactUUID))
	}
	if err != nil {
		return reconcile.Rendered{}, fmt.Errorf("failed to get provider: %w", err)
	}
	if provider.ModelList != "" {
		if err := json.Unmarshal([]byte(provider.ModelList), &provider.ModelProviders); err != nil {
			return reconcile.Rendered{}, fmt.Errorf("failed to parse model providers: %w", err)
		}
	}
	content, err := renderLLMProvider(provider)
	if err != nil {
		return reconcile.Rendered{}, reconcile.Permanent(fmt.Errorf("%w: invalid LLM provider configuration: %w", utils.ErrInvalidInput, err))
	}
	return renderedDeployment(provider.Configuration.Handle, []byte(content)), nil
}

func (k *LLMProviderKind) DeployEvent(ref reconcile.EventRef) eventhub.Event {
	payload := providerDeploymentEvent(ref.Placement, ref.DeploymentID, ref.PerformedAt)
	return eventhub.NewGatewayEvent("llmprovider.deployed", "CREATE", payload.ProviderID, payload)
}

func (k *LLMProviderKind) UndeployEvent(ref reconcile.EventRef) eventhub.Event {
	payload := &models.LLMProviderUndeploymentEvent{
		ProviderID:     ref.ArtifactUUID.String(),
		PerformedAt:    ref.PerformedAt,
		GatewayID:      ref.GatewayUUID.String(),
		OrganizationID: ref.OUID,
	}
	return eventhub.NewGatewayEvent("llmprovider.undeployed", "DELETE", payload.ProviderID, payload)
}

// DeleteEvent drops the provider's config from the gateway: an undeploy keeps it, its keys and its policies.
func (k *LLMProviderKind) DeleteEvent(artifact models.ArtifactRef) eventhub.Event {
	payload := &models.LLMProviderDeletionEvent{ProviderID: artifact.ArtifactUUID.String()}
	return eventhub.NewGatewayEvent("llmprovider.deleted", "DELETE", payload.ProviderID, payload)
}
