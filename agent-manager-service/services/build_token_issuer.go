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
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/requests"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// BuildTokenIssuer gives a source build the token its workload step calls the
// build publisher endpoint with.
//
// The token is an access token this service obtains for the build's organization
// from the identity provider with its own client (IDP_CLIENT_ID /
// IDP_CLIENT_SECRET): from the organization token endpoint
// (<IDP_TOKEN_URL without /oauth2/token>/ou/<ouId>/oauth2/token) when
// IDP_ORG_TOKEN_ENDPOINT_ENABLED is true, otherwise from IDP_TOKEN_URL itself,
// which issues tokens for the client's own organization only. It names that organization in its
// ouId claim and that client in its client_id claim, which is exactly what
// jwtassertion.BuildClientAuthMiddleware requires of a caller, so a build can only
// ever publish for the organization its token was issued for. It carries the
// agent build scope (amp:agent:build) and nothing else, which the endpoint also
// requires, so apart from publishing it can only start that organization's
// builds. The build is given the token only, through the secret store: never the
// client secret.
//
// Only the build's workload step is given the token.
type BuildTokenIssuer interface {
	// IssueBuildToken obtains a token for the organization, stores it in the
	// secret store and returns the SecretReference that holds it. The result names
	// where the token is; it never carries the token.
	IssueBuildToken(ctx context.Context, ouID string) (*client.BuildClientParams, error)
}

// BuildTokenIssuerBackgroundContextSetter receives the service's trusted
// background context once at startup. See buildTokenIssuer.storeContext.
type BuildTokenIssuerBackgroundContextSetter interface {
	SetBackgroundContext(ctx context.Context)
}

const (
	// buildTokenSecretEntity names the secret store entry holding an
	// organization's latest build token. There is one per organization: each
	// build replaces the token in it, and each run copies the token it finds there
	// when it starts (the workflow's ExternalSecret syncs once), so a later build
	// replacing it does not affect a run already started.
	buildTokenSecretEntity = "amp-build-token"
	// buildTokenSecretKey is the entry the workload step reads the token from.
	buildTokenSecretKey = "access-token"

	// buildTokenRequestTimeout bounds the identity provider call, including its
	// retries, and buildTokenStoreTimeout bounds storing the token. Issuing runs
	// inside the request that starts the build, so together they stay under the
	// usual 30s gateway request timeout in front of this service.
	buildTokenRequestTimeout = 10 * time.Second
	buildTokenStoreTimeout   = 15 * time.Second
	// buildTokenMinValidity is the least lifetime a token may have left when it is
	// issued. A build must still hold a valid token when its workload step
	// publishes, so a token that expires sooner is refused rather than handed to a
	// build that would fail at the end.
	buildTokenMinValidity = 10 * time.Minute
)

// errBuildToken marks a build token that could not be issued.
var errBuildToken = errors.New("build token could not be issued")

type buildTokenIssuer struct {
	// minter obtains the organization's token from the identity provider. Its
	// resource is config.BuildClient.Resource.
	minter       *orgClientTokenMinter
	secretClient secretmanagersvc.SecretManagementClient
	logger       *slog.Logger

	mu      sync.RWMutex
	baseCtx context.Context
}

// NewBuildTokenIssuer returns nil when build tokens are disabled
// (config.BuildClient.Enabled is false): builds then carry no build token and
// keep their previous path. Config validation guarantees a usable identity
// provider client whenever they are enabled.
func NewBuildTokenIssuer(
	cfg config.Config,
	logger *slog.Logger,
	secretClient secretmanagersvc.SecretManagementClient,
) BuildTokenIssuer {
	if !cfg.BuildClient.Enabled {
		logger.Info("Build tokens disabled; builds carry no organization-scoped build token")
		return nil
	}
	logger.Info("Build tokens enabled", "workflows", cfg.BuildClient.Workflows, "clientId", cfg.IDP.ClientID,
		"scope", jwtassertion.BuildTokenScope(), "resource", cfg.BuildClient.Resource)
	return newBuildTokenIssuer(cfg.IDP.TokenURL, cfg.IDP.ClientID, cfg.IDP.ClientSecret, cfg.BuildClient.Resource,
		cfg.IDP.OrgTokenEndpointEnabled, requests.NewRetryableHTTPClient(&http.Client{}), secretClient, logger)
}

func newBuildTokenIssuer(
	idpTokenURL, clientID, clientSecret, resource string,
	orgTokenEndpoint bool,
	httpClient requests.HttpClient,
	secretClient secretmanagersvc.SecretManagementClient,
	logger *slog.Logger,
) *buildTokenIssuer {
	return &buildTokenIssuer{
		minter:       newOrgClientTokenMinter(idpTokenURL, clientID, clientSecret, resource, orgTokenEndpoint, httpClient),
		secretClient: secretClient,
		logger:       logger,
		baseCtx:      context.Background(),
	}
}

// SetBackgroundContext installs the context the secret store is called from: the
// application's background context, which the deployment's
// BackgroundContextDecorator marks as allowed to use the service's own
// credential. Call once at startup, before the HTTP server starts.
func (i *buildTokenIssuer) SetBackgroundContext(ctx context.Context) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.baseCtx = ctx
}

// storeContext returns the context the token is stored with.
//
// The token is this service's (it is issued to its own client), so it is stored
// with this service's own credential rather than with the token of the user who
// started the build: a user allowed to build must not also need to be allowed to
// write the organization's secrets. The context is derived from the background
// context, which carries no request token and lets the secret store client fall
// back to the service's credential, and is given the organization, so the write
// is made for that organization only. It is cancelled with the request (the
// build cannot start once the request is gone), and in any case after
// buildTokenStoreTimeout.
func (i *buildTokenIssuer) storeContext(reqCtx context.Context, ouID string) (context.Context, context.CancelFunc) {
	i.mu.RLock()
	base := i.baseCtx
	i.mu.RUnlock()

	ctx, cancel := context.WithCancelCause(base)
	ctx, cancelTimeout := context.WithTimeout(ctx, buildTokenStoreTimeout)
	stop := context.AfterFunc(reqCtx, func() { cancel(context.Cause(reqCtx)) })
	ctx = orgctx.WithResolvedOrg(ctx, orgctx.ResolvedOrg{OUID: ouID, OuHandle: ""})
	ctx = logger.WithLogger(ctx, logger.GetLogger(reqCtx))
	return ctx, func() {
		stop()
		cancelTimeout()
		cancel(nil)
	}
}

// IssueBuildToken obtains and stores a build token for the organization, inside a
// fail-closed audit record.
func (i *buildTokenIssuer) IssueBuildToken(ctx context.Context, ouID string) (*client.BuildClientParams, error) {
	if strings.TrimSpace(ouID) == "" {
		return nil, fmt.Errorf("%w: organization is required", errBuildToken)
	}
	if _, err := config.OrganizationTokenURL(i.minter.idpTokenURL, ouID); err != nil {
		return nil, fmt.Errorf("%w: %w", errBuildToken, err)
	}

	// Handing a live access token for the organization to a build: record the
	// intent first and refuse to proceed if the trail is unavailable.
	attempt, err := audit.Begin(ctx, audit.ActionBuildTokenIssue,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceBuildToken, buildTokenSecretEntity, buildTokenSecretEntity),
		audit.Detail("clientId", i.minter.clientID),
	)
	if err != nil {
		return nil, err
	}
	params, err := i.issue(ctx, ouID)
	attempt.Complete(ctx, err)
	return params, err
}

// issue obtains the organization's build token and stores it. The token must be
// exactly a build token for the organization: one for another organization, or
// one naming no organization, which could act for any, is never handed to a
// build. The build publisher endpoint admits only a token carrying the build
// scope alone and verifies its signature when the build presents it, and a build
// must still hold a valid token when its workload step publishes, so the token
// must be valid for at least buildTokenMinValidity.
func (i *buildTokenIssuer) issue(ctx context.Context, ouID string) (*client.BuildClientParams, error) {
	requestCtx, cancel := context.WithTimeout(ctx, buildTokenRequestTimeout)
	defer cancel()
	token, _, err := i.minter.mint(requestCtx, ouID, jwtassertion.BuildTokenScope(), buildTokenMinValidity)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errBuildToken, err)
	}

	storeCtx, cancelStore := i.storeContext(ctx, ouID)
	defer cancelStore()
	secretRefName, err := i.secretClient.CreateSecret(storeCtx, secretmanagersvc.SecretLocation{
		OrgName:    ouID,
		EntityName: buildTokenSecretEntity,
	}, map[string]string{buildTokenSecretKey: token})
	if err != nil {
		// A secret store failure is temporary, except an entry of that name this
		// client does not manage, which no retry resolves.
		if !errors.Is(err, secretmanagersvc.ErrNotManaged) {
			return nil, fmt.Errorf("%w: %w: failed to store the token for organization %s: %w",
				errBuildToken, utils.ErrServiceUnavailable, ouID, err)
		}
		return nil, fmt.Errorf("%w: failed to store the token for organization %s: %w", errBuildToken, ouID, err)
	}
	if secretRefName == "" {
		return nil, fmt.Errorf("%w: the secret store returned no SecretReference for organization %s", errBuildToken, ouID)
	}
	i.logger.Info("Issued build token", "ouID", ouID, "clientId", i.minter.clientID, "secretRefName", secretRefName)
	return &client.BuildClientParams{SecretRefName: secretRefName}, nil
}
