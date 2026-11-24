//go:build integration

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

package gwctl_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

// intSyncedKey is one key as the bulk-sync routes serve it.
type intSyncedKey struct {
	UUID         string            `json:"uuid"`
	Name         string            `json:"name"`
	MaskedAPIKey string            `json:"maskedApiKey"`
	APIKeyHashes map[string]string `json:"apiKeyHashes"`
	ArtifactUUID string            `json:"artifactUuid"`
	Status       string            `json:"status"`
	CreatedAt    string            `json:"createdAt"`
	UpdatedAt    string            `json:"updatedAt"`
	ExpiresAt    *string           `json:"expiresAt"`
	Source       string            `json:"source"`
}

// intKeyOn stores a key named name on the artifact, in org, and returns its id.
func (a *intAPI) intKeyOn(org string, artifact uuid.UUID, name string, expires *time.Time) uuid.UUID {
	a.t.Helper()
	now := time.Now().UTC()
	key := &gwmodels.StoredAPIKey{
		Name: name, ArtifactUUID: artifact, OUID: org, APIKeyHash: "hash-" + name, MaskedAPIKey: "****" + name[len(name)-4:],
		Status: "active", CreatedAt: now, UpdatedAt: now, ExpiresAt: expires,
	}
	require.NoError(a.t, gwrepo.NewAPIKeyRepo(a.db).Upsert(key))
	return key.UUID
}

func intKeyArtifacts(keys []intSyncedKey) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k.ArtifactUUID)
	}
	return out
}

// TestINTRoutesAPIKeysServeTheOrganizationsKeysByKind pins the bulk-sync key
// routes: each serves its artifact kinds' keys of the gateway's organization,
// in the shape the gateway controller expects, and agent keys only for the
// gateway's environments.
func TestINTRoutesAPIKeysServeTheOrganizationsKeysByKind(t *testing.T) {
	a := newINTAPI(t)
	env := a.defaultEnv()
	unmapped := uuid.New()
	dbtest.EnvironmentWithID(t, a.db, a.org, unmapped)
	expires := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)

	provider := a.intArtifact(gwmodels.KindLLMProvider, unique("provider"))
	providerKey := a.intKeyOn(a.org, provider, unique("pk"), &expires)
	proxy := a.intArtifact(gwmodels.KindLLMProxy, unique("proxy"))
	a.intKeyOn(a.org, proxy, unique("xk"), nil)
	mcp := a.intArtifact(gwmodels.KindMCPProxy, unique("mcp"))
	a.intKeyOn(a.org, mcp, unique("mk"), nil)
	mapping := a.intArtifact(gwmodels.KindMCPMapping, unique("map"))
	a.intKeyOn(a.org, mapping, unique("mm"), nil)
	agent := a.intArtifact(gwmodels.KindAgent, "proj/agent/"+env.String())
	a.intKeyOn(a.org, agent, unique("ak"), nil)
	rest := a.intArtifact(gwmodels.KindRestAPI, "proj/rest/"+env.String())
	a.intKeyOn(a.org, rest, unique("rk"), nil)
	elsewhere := a.intArtifact(gwmodels.KindAgent, "proj/agent/"+unmapped.String())
	a.intKeyOn(a.org, elsewhere, unique("ek"), nil)
	otherOrg := dbtest.Org(t, a.db)
	foreign := a.intArtifactIn(otherOrg, gwmodels.KindLLMProvider, unique("foreign"))
	a.intKeyOn(otherOrg, foreign, unique("fk"), nil)

	providers := expectInto[[]intSyncedKey](t, a.call(http.MethodGet, "/llm-providers/api-keys", nil), http.StatusOK)
	require.Len(t, providers, 1, "another organization's keys are not served")
	k := providers[0]
	assert.Equal(t, providerKey.String(), k.UUID)
	assert.Equal(t, provider.String(), k.ArtifactUUID)
	assert.Equal(t, map[string]string{"sha256": "hash-" + k.Name}, k.APIKeyHashes)
	assert.Equal(t, "active", k.Status)
	assert.Equal(t, "external", k.Source)
	assert.NotEmpty(t, k.MaskedAPIKey)
	_, err := time.Parse(time.RFC3339, k.CreatedAt)
	require.NoError(t, err)
	require.NotNil(t, k.ExpiresAt)
	assert.Equal(t, expires.Format(time.RFC3339), *k.ExpiresAt)

	proxies := expectInto[[]intSyncedKey](t, a.call(http.MethodGet, "/llm-proxies/api-keys", nil), http.StatusOK)
	assert.ElementsMatch(t, []string{proxy.String(), mcp.String(), mapping.String()}, intKeyArtifacts(proxies))
	assert.Nil(t, proxies[0].ExpiresAt)

	agents := expectInto[[]intSyncedKey](t, a.call(http.MethodGet, "/apis/api-keys", nil), http.StatusOK)
	assert.ElementsMatch(t, []string{agent.String(), rest.String()}, intKeyArtifacts(agents),
		"an agent key of an environment the gateway doesn't serve is left out")
}

// TestINTRoutesApplicationsListTheOrganizationsAppsWithTheirKeys pins the
// applications bulk-sync: each application of the organization with the ids
// of the keys bound to it.
func TestINTRoutesApplicationsListTheOrganizationsAppsWithTheirKeys(t *testing.T) {
	a := newINTAPI(t)
	empty := expectInto[[]map[string]any](t, a.call(http.MethodGet, "/applications", nil), http.StatusOK)
	assert.Empty(t, empty)
	assert.NotNil(t, empty, "an empty JSON array, not null")

	agentID := a.agent("intapps", unique("agent"))
	env := a.defaultEnv()
	// An application's keys are bound to it by artifact id.
	appID := a.intArtifact(gwmodels.KindAgent, unique("app"))
	now := time.Now().UTC().Truncate(time.Second)
	handle := unique("app")
	require.NoError(t, a.db.Create(&gwmodels.AIApplication{
		UUID: appID, Handle: handle, Name: "Checkout app", AgentUUID: &agentID, EnvironmentUUID: env, OUID: a.org,
		CreatedAt: now, UpdatedAt: now,
	}).Error)
	k1 := a.intKeyOn(a.org, appID, unique("k1"), nil)
	k2 := a.intKeyOn(a.org, appID, unique("k2"), nil)

	apps := expectInto[[]struct {
		ApplicationID   string `json:"applicationId"`
		ApplicationUUID string `json:"applicationUuid"`
		ApplicationName string `json:"applicationName"`
		ApplicationType string `json:"applicationType"`
		Mappings        []struct {
			APIKeyUUID string `json:"apiKeyUuid"`
		} `json:"mappings"`
		CreatedAt time.Time `json:"createdAt"`
	}](t, a.call(http.MethodGet, "/applications", nil), http.StatusOK)
	require.Len(t, apps, 1)
	app := apps[0]
	assert.Equal(t, handle, app.ApplicationID)
	assert.Equal(t, appID.String(), app.ApplicationUUID)
	assert.Equal(t, "Checkout app", app.ApplicationName)
	assert.Equal(t, "ai-agent", app.ApplicationType)
	assert.WithinDuration(t, now, app.CreatedAt, time.Second)
	keys := make([]string, 0, len(app.Mappings))
	for _, m := range app.Mappings {
		keys = append(keys, m.APIKeyUUID)
	}
	assert.ElementsMatch(t, []string{k1.String(), k2.String()}, keys)
}

// TestINTRoutesManifestPushIsScopedToTheCallingGateway pins the manifest push:
// accepted for the token's own gateway, refused for another gateway's id, and
// a body that isn't a JSON object is rejected.
func TestINTRoutesManifestPushIsScopedToTheCallingGateway(t *testing.T) {
	a := newINTAPI(t)
	own := "/gateways/" + a.gateway.UUID.String() + "/manifest"

	rec := a.call(http.MethodPost, own, map[string]any{"mcp-auth": map[string]any{"version": "v1"}})
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	other := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleBoth).UUID.String()
	rec = a.call(http.MethodPost, "/gateways/"+other+"/manifest", map[string]any{})
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "does not match gateway ID")

	rec = a.call(http.MethodPost, own, "[1,2]")
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "Invalid gateway manifest")
}
