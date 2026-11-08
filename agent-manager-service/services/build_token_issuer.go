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
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

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
// from the identity provider's organization token endpoint
// (<IDP_TOKEN_URL without /oauth2/token>/ou/<ouId>/oauth2/token) with its own
// client (IDP_CLIENT_ID / IDP_CLIENT_SECRET). It names that organization in its
// ouId claim and that client in its client_id claim, which is exactly what
// jwtassertion.BuildClientAuthMiddleware requires of a caller, so a build can only
// ever publish for the organization its token was issued for. The build is given
// the token only, through the secret store: never the client secret.
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
	idpTokenURL  string
	clientID     string
	clientSecret string
	httpClient   requests.HttpClient
	secretClient secretmanagersvc.SecretManagementClient
	logger       *slog.Logger
	now          func() time.Time

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
	logger.Info("Build tokens enabled", "workflows", cfg.BuildClient.Workflows, "clientId", cfg.IDP.ClientID)
	return newBuildTokenIssuer(cfg.IDP.TokenURL, cfg.IDP.ClientID, cfg.IDP.ClientSecret,
		requests.NewRetryableHTTPClient(&http.Client{}), secretClient, logger)
}

func newBuildTokenIssuer(
	idpTokenURL, clientID, clientSecret string,
	httpClient requests.HttpClient,
	secretClient secretmanagersvc.SecretManagementClient,
	logger *slog.Logger,
) *buildTokenIssuer {
	return &buildTokenIssuer{
		idpTokenURL:  idpTokenURL,
		clientID:     strings.TrimSpace(clientID),
		clientSecret: clientSecret,
		httpClient:   httpClient,
		secretClient: secretClient,
		logger:       logger,
		now:          time.Now,
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
	tokenURL, err := config.OrganizationTokenURL(i.idpTokenURL, ouID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errBuildToken, err)
	}

	// Handing a live access token for the organization to a build: record the
	// intent first and refuse to proceed if the trail is unavailable.
	attempt, err := audit.Begin(ctx, audit.ActionBuildTokenIssue,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceBuildToken, buildTokenSecretEntity, buildTokenSecretEntity),
		audit.Detail("clientId", i.clientID),
	)
	if err != nil {
		return nil, err
	}
	params, err := i.issue(ctx, tokenURL, ouID)
	attempt.Complete(ctx, err)
	return params, err
}

func (i *buildTokenIssuer) issue(ctx context.Context, tokenURL, ouID string) (*client.BuildClientParams, error) {
	requestCtx, cancel := context.WithTimeout(ctx, buildTokenRequestTimeout)
	defer cancel()
	token, err := i.requestToken(requestCtx, tokenURL)
	if err != nil {
		return nil, err
	}
	if err := i.checkToken(token, ouID); err != nil {
		return nil, err
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
	i.logger.Info("Issued build token", "ouID", ouID, "clientId", i.clientID, "secretRefName", secretRefName)
	return &client.BuildClientParams{SecretRefName: secretRefName}, nil
}

// buildTokenResponse is the identity provider's token response.
type buildTokenResponse struct {
	AccessToken string `json:"access_token"`
}

// requestToken asks the organization token endpoint for a client_credentials
// token, authenticating with client_secret_basic as this service's other token
// requests do. No scope or resource is requested: the token's audience is then
// this service's client ID, which is what the build publisher endpoint accepts.
func (i *buildTokenIssuer) requestToken(ctx context.Context, tokenURL string) (string, error) {
	req := &requests.HttpRequest{
		Name:   "buildtoken.requestToken",
		URL:    tokenURL,
		Method: http.MethodPost,
	}
	req.SetFormData(map[string]string{"grant_type": "client_credentials"})
	req.SetHeader("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(i.clientID+":"+i.clientSecret)))

	var resp buildTokenResponse
	if err := requests.SendRequest(ctx, i.httpClient, req).ScanResponse(&resp, http.StatusOK); err != nil {
		// An identity provider that cannot be reached, fails (5xx) or is rate
		// limiting (429) is a temporary outage: utils.ErrServiceUnavailable lets
		// the build be retried. Any other answer (a refused client, say) is a
		// configuration problem and is not marked.
		var httpErr *requests.HttpError
		if !errors.As(err, &httpErr) || httpErr.StatusCode >= http.StatusInternalServerError ||
			httpErr.StatusCode == http.StatusTooManyRequests {
			return "", fmt.Errorf("%w: %w: token request failed: %w", errBuildToken, utils.ErrServiceUnavailable, err)
		}
		return "", fmt.Errorf("%w: token request failed: %w", errBuildToken, err)
	}
	if resp.AccessToken == "" {
		return "", fmt.Errorf("%w: the identity provider returned no access token", errBuildToken)
	}
	return resp.AccessToken, nil
}

// checkToken refuses a token that is not exactly a build token for the
// organization: issued to this service's client, naming the organization, and
// valid for at least buildTokenMinValidity. The token comes straight from the
// identity provider, so its claims are read without verifying its signature
// (the build publisher endpoint verifies it when the build presents it); this
// check only makes sure no other token (one for another organization, or one
// naming no organization, which could act for any) is ever handed to a build.
// The token itself never appears in an error.
func (i *buildTokenIssuer) checkToken(token, ouID string) error {
	// The claims are read through jwtassertion.TokenClaims, the type
	// BuildClientOrg reads the same token with, so both sides agree on the claim
	// names.
	claims := &jwtassertion.TokenClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(token, claims); err != nil {
		return fmt.Errorf("%w: the identity provider's access token is not a readable JWT", errBuildToken)
	}
	if got := claims.OuId; strings.TrimSpace(got) != ouID {
		return fmt.Errorf("%w: the token was issued for organization %q, not %q", errBuildToken, got, ouID)
	}
	if got := claims.ClientID; strings.TrimSpace(got) != i.clientID {
		return fmt.Errorf("%w: the token was issued to client %q, not %q", errBuildToken, got, i.clientID)
	}
	expiry := claims.ExpiresAt
	if expiry == nil {
		return fmt.Errorf("%w: the token carries no valid expiry", errBuildToken)
	}
	if remaining := expiry.Sub(i.now()); remaining < buildTokenMinValidity {
		return fmt.Errorf("%w: the token is valid for %s, less than the %s a build needs",
			errBuildToken, remaining.Truncate(time.Second), buildTokenMinValidity)
	}
	return nil
}
