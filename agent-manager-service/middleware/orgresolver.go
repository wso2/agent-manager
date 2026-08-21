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

package middleware

import (
	"context"
	"net/http"
	"sync"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
)

// ResolvedOrg holds the org identity injected into the request context by
// RequireOrgMatch after successful token validation. It lives in the leaf
// orgctx package so outbound clients can read it without importing middleware.
type ResolvedOrg = orgctx.ResolvedOrg

// WithResolvedOrg stores a ResolvedOrg in the context.
func WithResolvedOrg(ctx context.Context, org ResolvedOrg) context.Context {
	return orgctx.WithResolvedOrg(ctx, org)
}

// GetResolvedOrg retrieves the ResolvedOrg injected by RequireOrgMatch.
func GetResolvedOrg(ctx context.Context) (ResolvedOrg, bool) {
	return orgctx.GetResolvedOrg(ctx)
}

// OUIDFromRequest returns the token's OU ID injected by RequireOrgMatch.
// Returns "" (and logs) when the middleware did not run — that means the
// route was registered without org validation and is a programming error.
func OUIDFromRequest(r *http.Request) string {
	org, ok := GetResolvedOrg(r.Context())
	if !ok {
		logger.GetLogger(r.Context()).Error("resolved org missing from request context — RequireOrgMatch not applied")
		return ""
	}
	return org.OUID
}

// OrgHandleFromRequest returns the token's OU handle (the human-readable org
// name/handle, as opposed to OUIDFromRequest's UUID) injected by
// RequireOrgMatch. Returns "" (and logs) when the middleware did not run —
// that means the route was registered without org validation and is a
// programming error.
func OrgHandleFromRequest(r *http.Request) string {
	org, ok := GetResolvedOrg(r.Context())
	if !ok {
		logger.GetLogger(r.Context()).Error("resolved org missing from request context — RequireOrgMatch not applied")
		return ""
	}
	return org.OuHandle
}

// OrgResolver resolves an org handle to a Thunder OU ID.
type OrgResolver interface {
	ResolveOUID(ctx context.Context, orgName string) (string, error)
}

type thunderOrgResolver struct {
	client    thundersvc.IdentityClient
	mu        sync.RWMutex
	ouIDByOrg map[string]string
}

// NewOrgResolver returns an OrgResolver backed by Thunder, with a per-org cache.
func NewOrgResolver(client thundersvc.IdentityClient) OrgResolver {
	return &thunderOrgResolver{
		client:    client,
		ouIDByOrg: make(map[string]string),
	}
}

func (r *thunderOrgResolver) ResolveOUID(ctx context.Context, orgName string) (string, error) {
	r.mu.RLock()
	if id, ok := r.ouIDByOrg[orgName]; ok {
		r.mu.RUnlock()
		return id, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	if id, ok := r.ouIDByOrg[orgName]; ok {
		return id, nil
	}
	id, err := r.client.GetOUIDByHandle(ctx, orgName)
	if err != nil {
		return "", err
	}
	r.ouIDByOrg[orgName] = id
	return id, nil
}
