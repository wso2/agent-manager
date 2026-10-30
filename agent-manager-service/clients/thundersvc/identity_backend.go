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
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

type IdentityManagementMode string

const (
	IdentityManagementLegacyDirect IdentityManagementMode = "LEGACY_DIRECT"
	IdentityManagementCPManaged    IdentityManagementMode = "CP_MANAGED"
	IdentityManagementMixed        IdentityManagementMode = "MIXED"
)

var ErrIdentityCapabilityUnavailable = errors.New("identity management capability unavailable")

// ErrIdentityEnvironmentUnsupported is returned for a CP management call to an
// environment the CP cannot yet target. It is never retried through the DP.
var ErrIdentityEnvironmentUnsupported = fmt.Errorf(
	"%w: control-plane management reaches only the default environment", ErrIdentityCapabilityUnavailable,
)

// EnvironmentIdentityTarget identifies the control-plane deployment and exact
// data-plane gateway for one Agent Manager environment.
type EnvironmentIdentityTarget struct {
	Mode          IdentityManagementMode
	OUID          string
	OrgNamespace  string
	Environment   string
	DeploymentID  string
	GatewayID     string
	ManagementURL string
	IssuerURL     string
	TokenURL      string
	JWKSURL       string
	// DefaultEnvironment reports that CP management writes reach this
	// environment's gateway. The CP applies changes to its default gateway only,
	// so every other environment receives identity through promotion.
	DefaultEnvironment bool
	// AgentOwnerID is the identity that owns the agents created through the CP.
	// An agent is imported into every other environment's data plane by
	// promotion, and a data plane refuses an agent whose owner it does not
	// know, so this must be an identity that exists in every data plane of the
	// deployment. Empty leaves the owner to the management credential's own
	// identity, which usually exists only where that credential was issued.
	AgentOwnerID string
}

// validateManagement checks the target carries everything a CP management
// request needs, so no call is made against a guessed deployment or gateway.
func (t EnvironmentIdentityTarget) validateManagement() error {
	if t.Mode != IdentityManagementCPManaged || t.OUID == "" || t.OrgNamespace == "" || t.Environment == "" ||
		t.DeploymentID == "" || t.GatewayID == "" {
		return errors.New("complete CP organization, environment, deployment, and gateway target is required")
	}
	if err := validateIdentityURL(t.ManagementURL); err != nil {
		return fmt.Errorf("invalid CP management URL: %w", err)
	}
	return nil
}

type EnvironmentIdentityTargetResolver interface {
	ResolveIdentityTarget(ctx context.Context, ouID, orgNamespace, environment string) (EnvironmentIdentityTarget, error)
}

// ManagementRequestAuthorizer authorizes one outbound management request.
// Implementations must bind the target organization to their credential.
type ManagementRequestAuthorizer interface {
	Authorize(ctx context.Context, target EnvironmentIdentityTarget, req *http.Request) error
}

// CPEnvironmentIdentityBackend creates clients for a resolved CP target.
// The backend owns the CP protocol and must not route management calls to DP.
type CPEnvironmentIdentityBackend interface {
	Resolve(ctx context.Context, target EnvironmentIdentityTarget) (ThunderClient, error)
	ResolveIdentity(ctx context.Context, target EnvironmentIdentityTarget) (EnvIdentityClient, error)
}

// EnvironmentIdentityActivator makes a successful CP management write visible
// on the exact environment gateway. Legacy clients do not implement it.
type EnvironmentIdentityActivator interface {
	ActivateAgentIdentity(ctx context.Context, agentID string) error
	ActivateMCPResourceServer(ctx context.Context, resourceServerID string) error
	ActivateDeletedMCPResourceServer(ctx context.Context, proxyHandle string) error
}

// EnvironmentIdentityChangeActivator is an optional capability of a CP managed
// client: it makes a role, group, assignment or group member write visible on the
// environment gateway. Activation is idempotent, so doing it again is safe. A
// legacy client does not implement it.
type EnvironmentIdentityChangeActivator interface {
	ActivateIdentityChange(ctx context.Context) error
}

// EnvironmentIdentityPlacement tells provisioning whether an environment's
// identity is created by promotion instead of at agent creation.
type EnvironmentIdentityPlacement interface {
	IdentityCreatedByPromotion(ctx context.Context, ouID, orgNamespace, environment string) (bool, error)
}

// EnvThunderResolverDecorator replaces the deployment's legacy resolver with a
// resolver that can select another backend. A nil decorator keeps legacy mode.
type EnvThunderResolverDecorator func(legacy EnvThunderResolver) (EnvThunderResolver, error)

type routingEnvThunderResolver struct {
	mode    IdentityManagementMode
	targets EnvironmentIdentityTargetResolver
	legacy  EnvThunderResolver
	cp      CPEnvironmentIdentityBackend
}

// NewRoutingEnvThunderResolver selects one backend per environment. A CP
// failure is returned to the caller; it never retries through the legacy path.
func NewRoutingEnvThunderResolver(mode IdentityManagementMode, targets EnvironmentIdentityTargetResolver,
	legacy EnvThunderResolver, cp CPEnvironmentIdentityBackend,
) (EnvThunderResolver, error) {
	if legacy == nil {
		return nil, errors.New("legacy identity resolver is required")
	}
	if mode == "" {
		mode = IdentityManagementLegacyDirect
	}
	switch mode {
	case IdentityManagementLegacyDirect:
	case IdentityManagementCPManaged, IdentityManagementMixed:
		if targets == nil || cp == nil {
			return nil, errors.New("CP identity target resolver and backend are required")
		}
	default:
		return nil, fmt.Errorf("unsupported identity management mode %q", mode)
	}
	return &routingEnvThunderResolver{mode: mode, targets: targets, legacy: legacy, cp: cp}, nil
}

func (r *routingEnvThunderResolver) Resolve(ctx context.Context, ouID, orgNamespace, environment string) (ThunderClient, error) {
	target, cpManaged, err := r.selectTarget(ctx, ouID, orgNamespace, environment)
	if err != nil {
		return nil, err
	}
	if !cpManaged {
		return r.legacy.Resolve(ctx, ouID, orgNamespace, environment)
	}
	if !target.DefaultEnvironment {
		return nil, ErrIdentityEnvironmentUnsupported
	}
	client, err := r.cp.Resolve(ctx, target)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New("CP identity backend returned no client")
	}
	if _, ok := client.(EnvironmentIdentityActivator); !ok {
		return nil, errors.New("CP identity backend does not support exact-gateway activation")
	}
	return client, nil
}

func (r *routingEnvThunderResolver) ResolveIdentity(ctx context.Context, ouID, orgNamespace, environment string) (EnvIdentityClient, error) {
	target, cpManaged, err := r.selectTarget(ctx, ouID, orgNamespace, environment)
	if err != nil {
		return nil, err
	}
	if !cpManaged {
		return r.legacy.ResolveIdentity(ctx, ouID, orgNamespace, environment)
	}
	if !target.DefaultEnvironment {
		return nil, ErrIdentityEnvironmentUnsupported
	}
	client, err := r.cp.ResolveIdentity(ctx, target)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New("CP identity backend returned no identity client")
	}
	if _, ok := client.(EnvironmentIdentityActivator); !ok {
		return nil, errors.New("CP identity backend does not support exact-gateway activation")
	}
	return client, nil
}

// IdentityCreatedByPromotion reports whether the environment is CP-managed but
// not reachable by CP writes, so its identity arrives through promotion.
func (r *routingEnvThunderResolver) IdentityCreatedByPromotion(ctx context.Context, ouID, orgNamespace, environment string) (bool, error) {
	target, cpManaged, err := r.selectTarget(ctx, ouID, orgNamespace, environment)
	if err != nil {
		return false, err
	}
	return cpManaged && !target.DefaultEnvironment, nil
}

func (r *routingEnvThunderResolver) selectTarget(ctx context.Context, ouID, orgNamespace, environment string) (EnvironmentIdentityTarget, bool, error) {
	if ouID == "" || orgNamespace == "" || environment == "" {
		return EnvironmentIdentityTarget{}, false, errors.New("identity organization and environment are required")
	}
	if r.mode == IdentityManagementLegacyDirect {
		return EnvironmentIdentityTarget{}, false, nil
	}
	target, err := r.targets.ResolveIdentityTarget(ctx, ouID, orgNamespace, environment)
	if err != nil {
		return EnvironmentIdentityTarget{}, false, fmt.Errorf("resolve environment identity target: %w", err)
	}
	if target.OUID != ouID || target.OrgNamespace != orgNamespace || target.Environment != environment {
		return EnvironmentIdentityTarget{}, false, errors.New("environment identity target does not match requested organization and environment")
	}
	if r.mode == IdentityManagementCPManaged && target.Mode != IdentityManagementCPManaged {
		return EnvironmentIdentityTarget{}, false, errors.New("CP-managed identity target is required")
	}
	switch target.Mode {
	case IdentityManagementLegacyDirect:
		if r.mode != IdentityManagementMixed {
			return EnvironmentIdentityTarget{}, false, errors.New("legacy identity target is unavailable in CP-managed mode")
		}
		return target, false, nil
	case IdentityManagementCPManaged:
		if target.DeploymentID == "" || target.GatewayID == "" {
			return EnvironmentIdentityTarget{}, false, errors.New("CP identity target lacks management, gateway, or runtime metadata")
		}
		for _, value := range []string{target.ManagementURL, target.IssuerURL, target.TokenURL, target.JWKSURL} {
			if err := validateIdentityURL(value); err != nil {
				return EnvironmentIdentityTarget{}, false, fmt.Errorf("invalid CP identity target URL: %w", err)
			}
		}
		return target, true, nil
	default:
		return EnvironmentIdentityTarget{}, false, fmt.Errorf("unsupported environment identity mode %q", target.Mode)
	}
}

func validateIdentityURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("expected an absolute HTTP(S) URL without credentials, query, or fragment")
	}
	return nil
}
