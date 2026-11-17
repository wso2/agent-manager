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
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/wso2/agent-manager/agent-manager-service/clients/requests"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// orgClientTokenMinter obtains access tokens for an organization from the
// identity provider's organization token endpoint
// (<IDP_TOKEN_URL without /oauth2/token>/ou/<ouId>/oauth2/token) with this
// service's own client (IDP_CLIENT_ID / IDP_CLIENT_SECRET). Each token names that
// organization in its ouId claim and that client in its client_id claim, and
// carries exactly the one scope it was requested for.
type orgClientTokenMinter struct {
	idpTokenURL  string
	clientID     string
	clientSecret string
	// resource is the RFC 8707 resource indicator tokens are requested for; empty
	// requests none.
	resource   string
	httpClient requests.HttpClient
	now        func() time.Time
}

func newOrgClientTokenMinter(
	idpTokenURL, clientID, clientSecret, resource string,
	httpClient requests.HttpClient,
) *orgClientTokenMinter {
	return &orgClientTokenMinter{
		idpTokenURL:  idpTokenURL,
		clientID:     strings.TrimSpace(clientID),
		clientSecret: clientSecret,
		resource:     strings.TrimSpace(resource),
		httpClient:   httpClient,
		now:          time.Now,
	}
}

// orgClientTokenResponse is the identity provider's token response.
type orgClientTokenResponse struct {
	AccessToken string `json:"access_token"`
}

// mint obtains a token for the organization carrying scope and nothing else,
// valid for at least minValidity, and returns it with its expiry. The token
// itself never appears in an error.
func (m *orgClientTokenMinter) mint(
	ctx context.Context, ouID, scope string, minValidity time.Duration,
) (string, time.Time, error) {
	tokenURL, err := config.OrganizationTokenURL(m.idpTokenURL, ouID)
	if err != nil {
		return "", time.Time{}, err
	}
	token, err := m.requestToken(ctx, tokenURL, scope)
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt, err := m.checkToken(token, ouID, scope, minValidity)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

// requestToken asks the organization token endpoint for a client_credentials
// token, authenticating with client_secret_basic as this service's other token
// requests do. It requests scope only, for the configured resource server when
// there is one: an identity provider issues a scope only for the resource server
// that defines it, and drops it otherwise. Without a resource the token's
// audience is this service's client ID; with one it is the resource, which
// KEY_MANAGER_AUDIENCE must admit.
func (m *orgClientTokenMinter) requestToken(ctx context.Context, tokenURL, scope string) (string, error) {
	req := &requests.HttpRequest{
		Name:   "orgclienttoken.requestToken",
		URL:    tokenURL,
		Method: http.MethodPost,
	}
	form := map[string]string{
		"grant_type": "client_credentials",
		"scope":      scope,
	}
	if m.resource != "" {
		form["resource"] = m.resource
	}
	req.SetFormData(form)
	req.SetHeader("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(m.clientID+":"+m.clientSecret)))

	var resp orgClientTokenResponse
	if err := requests.SendRequest(ctx, m.httpClient, req).ScanResponse(&resp, http.StatusOK); err != nil {
		// An identity provider that cannot be reached, fails (5xx) or is rate
		// limiting (429) is a temporary outage: utils.ErrServiceUnavailable lets
		// the caller's operation be retried. Any other answer (a refused client,
		// say) is a configuration problem and is not marked.
		var httpErr *requests.HttpError
		if !errors.As(err, &httpErr) || httpErr.StatusCode >= http.StatusInternalServerError ||
			httpErr.StatusCode == http.StatusTooManyRequests {
			return "", fmt.Errorf("%w: token request failed: %w", utils.ErrServiceUnavailable, err)
		}
		return "", fmt.Errorf("token request failed: %w", err)
	}
	if resp.AccessToken == "" {
		return "", errors.New("the identity provider returned no access token")
	}
	return resp.AccessToken, nil
}

// checkToken refuses a token that is not exactly the token requested: issued to
// this service's client, naming the organization, carrying scope and nothing
// else, and valid for at least minValidity. It returns the token's expiry. The
// token comes straight from the identity provider, so its claims are read
// without verifying its signature (the endpoint the token is presented to
// verifies it); this check only makes sure no other token (one for another
// organization, or one naming no organization, which could act for any) is ever
// handed out. The token itself never appears in an error.
func (m *orgClientTokenMinter) checkToken(
	token, ouID, scope string, minValidity time.Duration,
) (time.Time, error) {
	// The claims are read through jwtassertion.TokenClaims, the type the
	// receiving endpoints read the same token with, so both sides agree on the
	// claim names.
	claims := &jwtassertion.TokenClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(token, claims); err != nil {
		return time.Time{}, errors.New("the identity provider's access token is not a readable JWT")
	}
	if got := claims.OuId; strings.TrimSpace(got) != ouID {
		return time.Time{}, fmt.Errorf("the token was issued for organization %q, not %q", got, ouID)
	}
	if got := claims.ClientID; strings.TrimSpace(got) != m.clientID {
		return time.Time{}, fmt.Errorf("the token was issued to client %q, not %q", got, m.clientID)
	}
	// A token the identity provider issued without the scope (a scope it does not
	// define or does not grant this client is dropped, not refused), or with
	// more, would not be admitted by an endpoint that requires the scope alone,
	// and would be broader than needed.
	if !jwtassertion.HasOnlyScope(claims.Scope, scope) {
		if strings.TrimSpace(claims.Scope) == "" {
			return time.Time{}, fmt.Errorf("the identity provider did not grant scope %q to client %q for organization %q",
				scope, m.clientID, ouID)
		}
		return time.Time{}, fmt.Errorf("the token carries scopes %q, not only %q", claims.Scope, scope)
	}
	expiry := claims.ExpiresAt
	if expiry == nil {
		return time.Time{}, errors.New("the token carries no valid expiry")
	}
	if remaining := expiry.Sub(m.now()); remaining < minValidity {
		return time.Time{}, fmt.Errorf("the token is valid for %s, less than the %s required",
			remaining.Truncate(time.Second), minValidity)
	}
	return expiry.Time, nil
}
