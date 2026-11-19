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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modules/gateway/gwrepo"
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
	load func(ctx context.Context, artifact models.ArtifactRef) (*models.LLMProvider, *repositories.UpstreamCredential, error)
}

// NewLLMProviderKind reads provider configuration and its credential version in the same transaction.
func NewLLMProviderKind(secrets *repositories.SecretStore) *LLMProviderKind {
	return &LLMProviderKind{load: func(ctx context.Context, artifact models.ArtifactRef) (*models.LLMProvider, *repositories.UpstreamCredential, error) {
		var provider *models.LLMProvider
		var credential *repositories.UpstreamCredential
		load := func(tx *gorm.DB) error {
			if err := dbx.LockRow(tx, "llm_providers", "id", "id = ?", artifact.ArtifactUUID); err != nil {
				return err
			}
			repo := gwrepo.NewLLMProviderRepo(tx, secrets)
			var err error
			provider, err = repo.GetByUUID(artifact.ArtifactUUID.String(), artifact.OUID)
			if err != nil {
				return err
			}
			credential, err = repo.SnapshotUpstreamCredential(ctx, tx, provider)
			return err
		}
		if tx := db.TxFrom(ctx); tx != nil {
			err := load(tx)
			return provider, credential, err
		}
		err := db.DB(ctx).Transaction(load)
		return provider, credential, err
	}}
}

// preparedProviderCredential carries the result of a synchronous deploy's external read into its transaction.
// Render verifies it against the current credential under the provider row lock.
type preparedProviderCredentialKey struct{}

type preparedProviderCredential struct {
	snapshot *repositories.UpstreamCredential
	value    string
}

var errProviderCredentialChanged = errors.New("provider upstream credential changed during deployment")

func resolveDeploymentCredential(ctx context.Context, credential *repositories.UpstreamCredential, auth *models.UpstreamAuth) error {
	if credential == nil {
		return nil
	}
	var value string
	if credential.External() && db.TxFrom(ctx) != nil {
		prepared, ok := ctx.Value(preparedProviderCredentialKey{}).(preparedProviderCredential)
		if !ok {
			return reconcile.InputNotReady("upstream credential requires an external vault read")
		}
		if !credential.SameVersion(prepared.snapshot) {
			return errProviderCredentialChanged
		}
		value = prepared.value
	} else {
		var err error
		value, err = credential.Open(ctx)
		if err != nil {
			return err
		}
	}
	auth.SecretRef = &value
	auth.Value = nil
	return nil
}

func validateDeploymentCredential(ctx context.Context, rendered *reconcile.Rendered, credential *repositories.UpstreamCredential) {
	if credential != nil && db.TxFrom(ctx) == nil {
		rendered.Validate = func(ctx context.Context, tx *gorm.DB) error {
			err := credential.LockVersion(ctx, tx)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return reconcile.InputNotReady("upstream credential changed during rendering")
			}
			return err
		}
	}
}

func (k *LLMProviderKind) Name() string        { return models.KindLLMProvider }
func (k *LLMProviderKind) RendersInline() bool { return true }

func (k *LLMProviderKind) GatewayArtifactID(artifact models.ArtifactRef) string {
	return artifact.ArtifactUUID.String()
}

func (k *LLMProviderKind) Render(ctx context.Context, artifact models.ArtifactRef) (reconcile.Rendered, error) {
	provider, credential, err := k.load(ctx, artifact)
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
	if credential != nil {
		if err := resolveDeploymentCredential(ctx, credential, provider.Configuration.Upstream.Main.Auth); err != nil {
			return reconcile.Rendered{}, err
		}
	}
	content, err := renderLLMProvider(provider)
	if err != nil {
		return reconcile.Rendered{}, reconcile.Permanent(fmt.Errorf("%w: invalid LLM provider configuration: %w", utils.ErrInvalidInput, err))
	}
	rendered := renderedDeployment(provider.Configuration.Handle, []byte(content))
	validateDeploymentCredential(ctx, &rendered, credential)
	return rendered, nil
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
