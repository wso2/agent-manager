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

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// GatewayConfigApplier patches the gateway runtime configuration (the source of
// truth) when an identity provider changes. The AMS mirror row alone is not enough:
// agent OAuth deploy-time validation reads the mirror, so a mirror-only issuer passes
// validation while the gateway rejects its tokens at runtime (see the warning on
// UpsertGatewayIdentityProvider). An applier closes that gap by updating the gateway
// before the mirror is written.
//
// In open-source deployments this is nil and the gateway config is applied out of band:
// the operator adds the key manager to the gateway's config.toml (the console shows the
// entry) and restarts it. Cloud deployments inject an
// implementation (via app.Options) that patches the per-environment gateway config
// server-side, so a single API request keeps the gateway and the mirror in sync.
//
// Implementations must be idempotent: an upsert is keyed by provider name and a delete
// is a no-op when the provider is absent, so a client retry after a partial failure
// re-converges. Validation of the provider (issuer/JWKS, OIDC discovery, system-provider
// guards) happens in the OSS layer before the applier is called; the applier does not
// re-validate and only ever receives custom providers.
type GatewayConfigApplier interface {
	// ApplyIdentityProvider upserts the identity provider in the gateway config.
	ApplyIdentityProvider(ctx context.Context, gatewayID, ouID string, idp models.GatewayIdentityProvider) error
	// DeleteIdentityProvider removes the identity provider from the gateway config.
	DeleteIdentityProvider(ctx context.Context, gatewayID, ouID, name string) error
}
