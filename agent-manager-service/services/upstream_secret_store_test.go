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

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/vault"
	"github.com/wso2/agent-manager/agent-manager-service/vault/vaulttest"
)

// fixtureKey is the encryption key the provider and MCP fixtures encrypt credentials with.
var fixtureKey = make([]byte, 32)

// renderedSecretRefs collects every secretRef in a rendered artifact: the encrypted form AMS resolves on fetch.
func renderedSecretRefs(t *testing.T, content []byte) []string {
	t.Helper()
	return renderedField(t, content, "secretRef")
}

// renderedField collects every string value of key in a rendered artifact.
func renderedField(t *testing.T, content []byte, key string) []string {
	t.Helper()
	var doc any
	require.NoError(t, yaml.Unmarshal(content, &doc))
	var out []string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if s, ok := x[key].(string); ok {
				out = append(out, s)
			}
			for _, child := range x {
				walk(child)
			}
		case []any:
			for _, child := range x {
				walk(child)
			}
		}
	}
	walk(doc)
	return out
}

var placeholderRe = regexp.MustCompile(`^\{\{ secret "(amp-[0-9a-f]{32})" \}\}$`)

// renderedHandles returns the handles the artifact's credential values name in {{ secret "handle" }} placeholders.
func renderedHandles(t *testing.T, content []byte) []string {
	t.Helper()
	var handles []string
	for _, v := range renderedField(t, content, "value") {
		if m := placeholderRe.FindStringSubmatch(v); m != nil {
			handles = append(handles, m[1])
		}
	}
	return handles
}

// credential is the plaintext the artifact's single credential resolves to: through its handle, as a gateway reads
// it, or from an inline secretRef, as AMS resolves one on fetch.
func (f *providerFixture) credential(t *testing.T, content []byte) string {
	t.Helper()
	if handles := renderedHandles(t, content); len(handles) > 0 {
		require.Len(t, handles, 1, "the artifact names one credential")
		assert.Empty(t, renderedSecretRefs(t, content), "a placeholder replaces the secretRef")
		return f.handleValue(t, handles[0])
	}
	refs := renderedSecretRefs(t, content)
	require.Len(t, refs, 1, "the artifact carries one credential")
	plaintext, err := utils.DecryptSecretRef(refs[0], fixtureKey)
	require.NoError(t, err)
	return plaintext
}

// handleValue resolves a handle to its plaintext the way the gateway's secret sync does, without its scoping.
func (f *providerFixture) handleValue(t *testing.T, handle string) string {
	t.Helper()
	var h models.GatewaySecretHandle
	require.NoError(t, db.GetDB().Where("handle = ?", handle).Take(&h).Error)
	value, found, err := f.secrets.Read(context.Background(), db.GetDB(), h.SecretID)
	require.NoError(t, err)
	require.True(t, found)
	plaintext, err := utils.DecryptSecretRef(string(value), fixtureKey)
	require.NoError(t, err)
	return plaintext
}

func handleRow(t *testing.T, handle string) (models.GatewaySecretHandle, bool) {
	t.Helper()
	var rows []models.GatewaySecretHandle
	require.NoError(t, db.GetDB().Where("handle = ?", handle).Find(&rows).Error)
	if len(rows) == 0 {
		return models.GatewaySecretHandle{}, false
	}
	return rows[0], true
}

// sweep releases the retired gateway secrets no gateway can still be running, as the secret job worker does.
func sweep(t *testing.T) {
	t.Helper()
	_, err := repositories.ReleaseRetiredGatewaySecrets(context.Background(), db.GetDB())
	require.NoError(t, err)
}

// settle sets every deployed placement of the artifact to status, as a gateway's ack does.
func settle(t *testing.T, ouID string, artifact uuid.UUID, status models.DeploymentStatus) {
	t.Helper()
	require.NoError(t, db.GetDB().Model(&models.DeploymentStatusRecord{}).
		Where("artifact_id = ? AND org_id = ? AND status_desired = ?", artifact, ouID, models.DeploymentStatusDeployed).
		Update("status", status).Error)
}

func storedSecret(t *testing.T, id uuid.UUID) (models.Secret, bool) {
	t.Helper()
	var rows []models.Secret
	require.NoError(t, db.GetDB().Where("id = ?", id).Find(&rows).Error)
	if len(rows) == 0 {
		return models.Secret{}, false
	}
	return rows[0], true
}

func storedProvider(t *testing.T, id uuid.UUID) (models.LLMProvider, string) {
	t.Helper()
	var row models.LLMProvider
	require.NoError(t, db.GetDB().Where("id = ?", id).Take(&row).Error)
	var raw struct{ Configuration string }
	require.NoError(t, db.GetDB().Table("llm_providers").Select("configuration").Where("id = ?", id).Take(&raw).Error)
	return row, raw.Configuration
}

func (f *providerFixture) render(t *testing.T, id uuid.UUID) []byte {
	t.Helper()
	rendered, err := NewLLMProviderKind(f.secrets).Render(context.Background(),
		models.ArtifactRef{OUID: f.sync.ouID, ArtifactUUID: id, Kind: models.KindLLMProvider})
	require.NoError(t, err)
	return rendered.Content
}

func withAuth(p *models.LLMProvider, value *string) {
	authType, header := models.AuthTypeAPIKey, "Authorization"
	p.Configuration.Upstream.Main.Auth = &models.UpstreamAuth{Type: &authType, Header: &header, Value: value}
}

func strValue(s string) *string { return &s }

func timeHours(n int) time.Duration { return time.Duration(n) * time.Hour }

// assertNoCredentialIn fails when any of secrets appears in what a client is sent.
func assertNoCredentialIn(t *testing.T, response any, secrets ...string) {
	t.Helper()
	body, err := json.Marshal(response)
	require.NoError(t, err)
	for _, s := range secrets {
		assert.NotContains(t, string(body), s)
	}
}

func TestLLMProviderUpstreamCredentialLivesInTheSecretStore(t *testing.T) {
	f := newProviderFixture(t)
	ctx := context.Background()
	provider := f.newProvider(t)
	withAuth(provider, strValue("sk-first"))

	created, err := f.providers.Create(ctx, f.sync.ouID, "me", provider)
	require.NoError(t, err)

	row, raw := storedProvider(t, created.UUID)
	require.NotNil(t, row.UpstreamSecretID, "the credential has its own secret")
	first := *row.UpstreamSecretID
	assert.NotContains(t, raw, "secretRef", "the configuration no longer carries the credential")
	assert.NotContains(t, raw, "sk-first")
	secret, ok := storedSecret(t, first)
	require.True(t, ok)
	assert.Equal(t, f.sync.ouID, secret.OrgID)
	assert.Equal(t, "llm_provider", secret.OwnerKind)
	assert.Equal(t, created.UUID, secret.OwnerID)

	// The artifact names the credential by a gateway placeholder in spec.upstream.auth.value.
	content := f.render(t, created.UUID)
	var doc struct {
		Spec struct {
			Upstream struct {
				Auth struct {
					Value     string `yaml:"value"`
					SecretRef string `yaml:"secretRef"`
				} `yaml:"auth"`
			} `yaml:"upstream"`
		} `yaml:"spec"`
	}
	require.NoError(t, yaml.Unmarshal(content, &doc))
	handles := renderedHandles(t, content)
	require.Len(t, handles, 1)
	firstHandle := handles[0]
	assert.Equal(t, models.GatewaySecretPlaceholder(firstHandle), doc.Spec.Upstream.Auth.Value)
	assert.Empty(t, doc.Spec.Upstream.Auth.SecretRef)
	assert.Equal(t, "sk-first", f.credential(t, content))
	assert.NotContains(t, string(content), first.String(), "no database id reaches the artifact")
	assert.NotContains(t, string(content), "sk-first")

	got, err := f.providers.Get(created.UUID.String(), f.sync.ouID)
	require.NoError(t, err)
	assertNoCredentialIn(t, utils.ConvertModelToSpecLLMProviderResponse(got), "sk-first", first.String(), firstHandle)
	assertNoCredentialIn(t, utils.ConvertModelToSpecLLMProviderListItemResponse(got), "sk-first", first.String(), firstHandle)

	// An update that omits the credential keeps the secret and its handle.
	got.Description = "edited"
	_, err = f.providers.Update(ctx, created.UUID.String(), f.sync.ouID, got)
	require.NoError(t, err)
	row, _ = storedProvider(t, created.UUID)
	require.NotNil(t, row.UpstreamSecretID)
	assert.Equal(t, first, *row.UpstreamSecretID)
	secret, _ = storedSecret(t, first)
	assert.Equal(t, 1, secret.Version, "an omitted credential is not rewritten")
	assert.Equal(t, []string{firstHandle}, renderedHandles(t, f.render(t, created.UUID)))

	// A new credential gets a new secret and handle, so the artifact changes; the previous one is retired.
	got, err = f.providers.Get(created.UUID.String(), f.sync.ouID)
	require.NoError(t, err)
	withAuth(got, strValue("sk-second"))
	_, err = f.providers.Update(ctx, created.UUID.String(), f.sync.ouID, got)
	require.NoError(t, err)
	row, raw = storedProvider(t, created.UUID)
	second := *row.UpstreamSecretID
	assert.NotEqual(t, first, second)
	assert.NotContains(t, raw, "secretRef")
	content = f.render(t, created.UUID)
	secondHandle := renderedHandles(t, content)[0]
	assert.NotEqual(t, firstHandle, secondHandle)
	assert.Equal(t, "sk-second", f.credential(t, content))
	retired, ok := handleRow(t, firstHandle)
	require.True(t, ok, "a retired credential stays until no gateway can be running it")
	assert.NotNil(t, retired.RetiredAt)
	sweep(t)
	_, ok = storedSecret(t, first)
	assert.False(t, ok, "the provider is deployed nowhere, so its retired credential is released")
	_, ok = handleRow(t, firstHandle)
	assert.False(t, ok)

	// The direct render path names the moved credential too.
	got, err = f.providers.Get(created.UUID.String(), f.sync.ouID)
	require.NoError(t, err)
	direct, err := f.deployment.generateLLMProviderDeploymentYAML(got, f.sync.ouID)
	require.NoError(t, err)
	assert.Equal(t, []string{secondHandle}, renderedHandles(t, []byte(direct)))

	// Removing the auth retires the secret, which is released once nothing runs it.
	got, err = f.providers.Get(created.UUID.String(), f.sync.ouID)
	require.NoError(t, err)
	got.Configuration.Upstream.Main.Auth = nil
	_, err = f.providers.Update(ctx, created.UUID.String(), f.sync.ouID, got)
	require.NoError(t, err)
	row, _ = storedProvider(t, created.UUID)
	assert.Nil(t, row.UpstreamSecretID)
	content = f.render(t, created.UUID)
	assert.Empty(t, renderedHandles(t, content))
	assert.Empty(t, renderedSecretRefs(t, content))
	sweep(t)
	_, ok = storedSecret(t, second)
	assert.False(t, ok)
}

func TestLLMProviderDeletionReleasesItsCredential(t *testing.T) {
	f := newProviderFixture(t)
	ctx := context.Background()

	// Through the provider delete.
	provider := f.newProvider(t)
	withAuth(provider, strValue("sk-delete"))
	created, err := f.providers.CreateAndDeploy(ctx, f.sync.ouID, "me", provider, []string{f.sync.gateway.String()}, f.deployment)
	require.NoError(t, err)
	row, _ := storedProvider(t, created.Provider.UUID)
	require.NotNil(t, row.UpstreamSecretID)
	require.NoError(t, f.providers.Delete(ctx, created.Provider.UUID.String(), f.sync.ouID, f.deployment))
	sweep(t)
	_, ok := storedSecret(t, *row.UpstreamSecretID)
	assert.False(t, ok)

	// Through the artifact the provider row cascades from.
	provider = f.newProvider(t)
	withAuth(provider, strValue("sk-cascade"))
	created2, err := f.providers.Create(ctx, f.sync.ouID, "me", provider)
	require.NoError(t, err)
	row, _ = storedProvider(t, created2.UUID)
	require.NotNil(t, row.UpstreamSecretID)
	require.NoError(t, db.GetDB().Transaction(func(tx *gorm.DB) error {
		return repositories.NewArtifactRepo(tx).Delete(tx, created2.UUID.String())
	}))
	sweep(t)
	_, ok = storedSecret(t, *row.UpstreamSecretID)
	assert.False(t, ok)
}

// A provider saved before credentials moved keeps its encrypted credential inline. It is rendered as it is, as an
// encrypted secretRef the gateway fetch path resolves, and moves to the secret store the next time it is saved.
func TestLLMProviderReadsAndMovesAnInlineCredential(t *testing.T) {
	f := newProviderFixture(t)
	ctx := context.Background()
	created, err := f.providers.Create(ctx, f.sync.ouID, "me", f.newProvider(t))
	require.NoError(t, err)
	token, err := utils.EncryptSecretRef("sk-legacy", fixtureKey)
	require.NoError(t, err)
	legacy := created.Configuration
	authType, header := models.AuthTypeAPIKey, "Authorization"
	legacy.Upstream.Main.Auth = &models.UpstreamAuth{Type: &authType, Header: &header, SecretRef: &token}
	require.NoError(t, db.GetDB().Model(&models.LLMProvider{}).Where("id = ?", created.UUID).
		Update("configuration", dbx.JSON(legacy)).Error)

	content := f.render(t, created.UUID)
	assert.Equal(t, []string{token}, renderedSecretRefs(t, content))
	assert.Equal(t, "sk-legacy", f.credential(t, content))
	got, err := f.providers.Get(created.UUID.String(), f.sync.ouID)
	require.NoError(t, err)
	direct, err := f.deployment.generateLLMProviderDeploymentYAML(got, f.sync.ouID)
	require.NoError(t, err)
	assert.Equal(t, "sk-legacy", f.credential(t, []byte(direct)))
	assertNoCredentialIn(t, utils.ConvertModelToSpecLLMProviderResponse(got), "sk-legacy", token)

	// The controller carries the stored credential into an update that omits it; saving moves it.
	got.Description = "moved"
	_, err = f.providers.Update(ctx, created.UUID.String(), f.sync.ouID, got)
	require.NoError(t, err)
	row, raw := storedProvider(t, created.UUID)
	require.NotNil(t, row.UpstreamSecretID)
	assert.NotContains(t, raw, "secretRef", "the inline copy is gone once the secret holds it")
	content = f.render(t, created.UUID)
	assert.Len(t, renderedHandles(t, content), 1)
	assert.Equal(t, "sk-legacy", f.credential(t, content))
}

// An LLM proxy fronts its provider by handle and carries its own credential, so it renders whether or not the
// provider's credential has moved.
func TestLLMProxyRendersWhenItsProvidersCredentialMoved(t *testing.T) {
	f := newProxyFixture(t)
	ctx := context.Background()
	got, err := f.providers.Get(f.provider.UUID.String(), f.sync.ouID)
	require.NoError(t, err)
	withAuth(got, strValue("sk-provider"))
	_, err = f.providers.Update(ctx, f.provider.UUID.String(), f.sync.ouID, got)
	require.NoError(t, err)
	row, _ := storedProvider(t, f.provider.UUID)
	require.NotNil(t, row.UpstreamSecretID)

	proxy := f.newProxy(t)
	rendered, err := f.kind.Render(ctx, models.ArtifactRef{
		OUID: f.sync.ouID, ArtifactUUID: proxy.UUID, Kind: models.KindLLMProxy, Handle: proxy.Handle,
	})
	require.NoError(t, err)
	assert.NotContains(t, string(rendered.Content), "sk-provider")
	assert.Empty(t, renderedHandles(t, rendered.Content), "the proxy carries no provider credential")
	direct, err := f.proxies.generateLLMProxyDeploymentYAML(proxy, f.sync.ouID)
	require.NoError(t, err)
	assert.Equal(t, string(rendered.Content), direct)
}

// gatewaySecretSync is the gateway's view of the secret sync.
func (f *providerFixture) gatewaySecretSync() *GatewayInternalAPIService {
	return NewGatewayInternalAPIService(nil, nil, f.sync.repo, nil, nil, nil, fixtureKey, nil,
		repositories.NewGatewaySecretRepository(db.GetDB(), f.secrets))
}

func syncedHandles(t *testing.T, svc *GatewayInternalAPIService, ouID string, gateway uuid.UUID) map[string]string {
	t.Helper()
	list, err := svc.ListGatewaySecrets(context.Background(), ouID, gateway.String(), nil, true)
	require.NoError(t, err)
	assert.Equal(t, len(list.List), list.Count)
	out := map[string]string{}
	for _, item := range list.List {
		assert.Equal(t, "ACTIVE", item.Status)
		assert.NotEmpty(t, item.Hash)
		require.NotNil(t, item.Value)
		out[item.Handle] = *item.Value
	}
	return out
}

// The secret sync serves a gateway the credentials its deployments name, and nothing to another gateway or org. A
// rotated credential stays served, and stored, until the gateway has applied the deployment that replaces it.
func TestGatewaySecretSyncServesOnlyTheGatewaysDeployments(t *testing.T) {
	f := newProviderFixture(t)
	ctx := context.Background()
	svc := f.gatewaySecretSync()
	gw := f.sync.gateway
	provider := f.newProvider(t)
	withAuth(provider, strValue("sk-a"))
	created, err := f.providers.CreateAndDeploy(ctx, f.sync.ouID, "me", provider, []string{gw.String()}, f.deployment)
	require.NoError(t, err)
	id := created.Provider.UUID
	pulled, err := f.sync.repo.GetPullable(ctx, f.sync.ouID, id, gw.String())
	require.NoError(t, err)
	handles := renderedHandles(t, pulled.Content)
	require.Len(t, handles, 1, "the persisted deployment carries the placeholder")
	handleA := handles[0]
	assert.NotContains(t, string(pulled.Content), "sk-a")

	// The startup sync carries values; the reconnect listing carries none.
	assert.Equal(t, map[string]string{handleA: "sk-a"}, syncedHandles(t, svc, f.sync.ouID, gw))
	meta, err := svc.ListGatewaySecrets(ctx, f.sync.ouID, gw.String(), nil, false)
	require.NoError(t, err)
	require.Len(t, meta.List, 1)
	assert.Nil(t, meta.List[0].Value)
	row, _ := storedProvider(t, id)
	assert.NotEqual(t, row.UpstreamSecretID.String(), meta.List[0].ID, "no database id leaves AMS")
	value, err := svc.GetGatewaySecretValue(ctx, f.sync.ouID, gw.String(), handleA)
	require.NoError(t, err)
	assert.Equal(t, "sk-a", value)
	body, err := json.Marshal(meta)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "sk-a")

	// Another gateway of the org, and a gateway of another org, are told nothing.
	other := f.sync.seedGateway(t, f.sync.ouID)
	assert.Empty(t, syncedHandles(t, svc, f.sync.ouID, other))
	_, err = svc.GetGatewaySecretValue(ctx, f.sync.ouID, other.String(), handleA)
	assert.ErrorIs(t, err, ErrGatewaySecretNotFound)
	otherOrg := dbtest.Org(t, db.GetDB())
	foreign := f.sync.seedGateway(t, otherOrg)
	_, err = svc.GetGatewaySecretValue(ctx, otherOrg, foreign.String(), handleA)
	assert.ErrorIs(t, err, ErrGatewaySecretNotFound)
	_, err = svc.GetGatewaySecretValue(ctx, otherOrg, gw.String(), handleA)
	assert.ErrorIs(t, err, ErrGatewaySecretNotFound, "the gateway's id under another org reads nothing")
	_, err = svc.GetGatewaySecretValue(ctx, f.sync.ouID, gw.String(), "amp-00000000000000000000000000000000")
	assert.ErrorIs(t, err, ErrGatewaySecretNotFound)

	// Rotate: the new deployment names a new handle, and the old one stays served while the gateway may run it.
	settle(t, f.sync.ouID, id, models.DeploymentStatusDeployed)
	got, err := f.providers.Get(id.String(), f.sync.ouID)
	require.NoError(t, err)
	withAuth(got, strValue("sk-b"))
	_, err = f.providers.UpdateAndSync(ctx, id.String(), f.sync.ouID, got, []string{gw.String()}, f.deployment)
	require.NoError(t, err)
	pulled, err = f.sync.repo.GetPullable(ctx, f.sync.ouID, id, gw.String())
	require.NoError(t, err)
	handleB := renderedHandles(t, pulled.Content)[0]
	require.NotEqual(t, handleA, handleB, "a new credential is a new deployment revision")
	assert.Equal(t, map[string]string{handleA: "sk-a", handleB: "sk-b"}, syncedHandles(t, svc, f.sync.ouID, gw),
		"the gateway may still be running the previous revision")
	sweep(t)
	value, err = svc.GetGatewaySecretValue(ctx, f.sync.ouID, gw.String(), handleA)
	require.NoError(t, err)
	assert.Equal(t, "sk-a", value, "not released while the replacement is in flight")

	// A failed replacement keeps the previous credential.
	settle(t, f.sync.ouID, id, models.DeploymentStatusFailed)
	sweep(t)
	value, err = svc.GetGatewaySecretValue(ctx, f.sync.ouID, gw.String(), handleA)
	require.NoError(t, err)
	assert.Equal(t, "sk-a", value)

	// Once the gateway has applied it, the previous credential is released and served no more.
	settle(t, f.sync.ouID, id, models.DeploymentStatusDeployed)
	sweep(t)
	_, err = svc.GetGatewaySecretValue(ctx, f.sync.ouID, gw.String(), handleA)
	assert.ErrorIs(t, err, ErrGatewaySecretNotFound)
	_, ok := handleRow(t, handleA)
	assert.False(t, ok)
	assert.Equal(t, map[string]string{handleB: "sk-b"}, syncedHandles(t, svc, f.sync.ouID, gw))
}

// endpointRow is the fixture proxy's single endpoint as stored.
func (f *mcpFixture) endpointRow(t *testing.T) (models.MCPProxyEndpoint, string) {
	t.Helper()
	var rows []models.MCPProxyEndpoint
	require.NoError(t, db.GetDB().Where("mcp_proxy_id = ?", f.proxy.UUID).Find(&rows).Error)
	require.Len(t, rows, 1)
	var raw struct{ Configuration string }
	require.NoError(t, db.GetDB().Table("mcp_proxy_endpoints").Select("configuration").Where("id = ?", rows[0].UUID).
		Take(&raw).Error)
	return rows[0], raw.Configuration
}

func (f *mcpFixture) pulled(t *testing.T, gateway uuid.UUID) []byte {
	t.Helper()
	pulled, err := f.sync.repo.GetPullable(context.Background(), f.sync.ouID, f.artifact, gateway.String())
	require.NoError(t, err)
	return pulled.Content
}

// renderMCP renders the fixture's artifact the way the worker does.
func (f *mcpFixture) renderMCP(t *testing.T) []byte {
	t.Helper()
	rendered, err := f.svc.kind.Render(context.Background(), models.ArtifactRef{OUID: f.sync.ouID, ArtifactUUID: f.artifact})
	require.NoError(t, err)
	return rendered.Content
}

// updateRequest is a PUT of the fixture proxy with auth as its endpoint's upstream auth.
func (f *mcpFixture) updateRequest(auth *models.UpstreamAuth) *models.MCPProxyDTO {
	cfg := f.proxy.Endpoints[0].Configuration
	return &models.MCPProxyDTO{
		ID: mcpSourceHandle(f.proxy), Name: "Weather", Version: "v1.0",
		Context: f.proxy.Configuration.Context, Vhost: f.proxy.Configuration.Vhost,
		Endpoints: []models.MCPProxyEndpointDTO{{
			ID:       "primary",
			Upstream: models.UpstreamConfig{Main: &models.UpstreamEndpoint{URL: cfg.Upstream.URL, Auth: auth}},
			Security: cfg.Security,
			Environments: []models.MCPEndpointEnvironmentDTO{{
				EnvironmentUUID: f.env.String(),
			}},
		}},
	}
}

func mcpAuth(value *string) *models.UpstreamAuth {
	authType, header := models.AuthTypeAPIKey, "X-Backend-Key"
	return &models.UpstreamAuth{Type: &authType, Header: &header, Value: value}
}

// newMCPSecretFixture is an MCP proxy deployed with its credential inline, as saved before credentials moved.
func newMCPSecretFixture(t *testing.T, vaults *vault.Vaults) (*mcpFixture, uuid.UUID) {
	t.Helper()
	f := newMCPFixture(t)
	f.svc.encryptionKey = fixtureKey
	store := repositories.NewSecretStore(vaults)
	f.secrets = store
	f.svc.endpointRepo = repositories.NewMCPProxyEndpointRepository(db.GetDB(), store)
	f.svc.kind.load = func(ctx context.Context, artifact models.ArtifactRef) (*mcpArtifact, error) {
		return loadMCPArtifact(ctx, store, artifact)
	}
	token, err := utils.EncryptSecretRef("mcp-legacy", fixtureKey)
	require.NoError(t, err)
	cfg := f.proxy.Endpoints[0].Configuration
	cfg.Upstream.Auth.SecretRef = &token
	require.NoError(t, db.GetDB().Model(&models.MCPProxyEndpoint{}).Where("id = ?", f.proxy.Endpoints[0].UUID).
		Update("configuration", dbx.JSON(cfg)).Error)
	gateway := f.mapGateway(t, f.env)
	require.NoError(t, f.deploy(t))
	return f, gateway
}

func TestMCPEndpointUpstreamCredentialLivesInTheSecretStore(t *testing.T) {
	f, gateway := newMCPSecretFixture(t, vaulttest.Local(t))
	ctx := context.Background()

	// Saved with its credential inline, as before credentials moved: it renders as the stored secretRef.
	_, raw := f.endpointRow(t)
	assert.Contains(t, raw, "secretRef")
	assert.Len(t, renderedSecretRefs(t, f.pulled(t, gateway)), 1)
	assert.Equal(t, "mcp-legacy", f.credential(t, f.pulled(t, gateway)))

	// A PUT that omits the credential keeps it, moving it into the secret store under a handle.
	_, err := f.svc.Update(ctx, f.sync.ouID, mcpSourceHandle(f.proxy), f.updateRequest(mcpAuth(nil)))
	require.NoError(t, err)
	endpoint, raw := f.endpointRow(t)
	require.NotNil(t, endpoint.UpstreamSecretID)
	moved := *endpoint.UpstreamSecretID
	assert.NotContains(t, raw, "secretRef")
	content := f.pulled(t, gateway)
	handles := renderedHandles(t, content)
	require.Len(t, handles, 1, "the new deployment carries the placeholder in the set-headers policy")
	movedHandle := handles[0]
	assert.Contains(t, string(content), "X-Backend-Key")
	assert.Equal(t, "mcp-legacy", f.credential(t, content))
	secret, ok := storedSecret(t, moved)
	require.True(t, ok)
	assert.Equal(t, "mcp_proxy_endpoint", secret.OwnerKind)
	assert.Equal(t, endpoint.UUID, secret.OwnerID)

	// Another omitted credential: the replacement endpoint row takes the same secret and handle over.
	_, err = f.svc.Update(ctx, f.sync.ouID, mcpSourceHandle(f.proxy), f.updateRequest(mcpAuth(nil)))
	require.NoError(t, err)
	endpoint, _ = f.endpointRow(t)
	require.NotNil(t, endpoint.UpstreamSecretID)
	assert.Equal(t, moved, *endpoint.UpstreamSecretID)
	secret, _ = storedSecret(t, moved)
	assert.Equal(t, 1, secret.Version)
	assert.Equal(t, endpoint.UUID, secret.OwnerID, "the secret names the endpoint row that holds it now")
	assert.Equal(t, []string{movedHandle}, renderedHandles(t, f.pulled(t, gateway)))

	// A new credential gets a new secret and handle. The previous one stays until the gateway has applied it.
	_, err = f.svc.Update(ctx, f.sync.ouID, mcpSourceHandle(f.proxy), f.updateRequest(mcpAuth(strValue("mcp-new"))))
	require.NoError(t, err)
	endpoint, raw = f.endpointRow(t)
	require.NotNil(t, endpoint.UpstreamSecretID)
	replaced := *endpoint.UpstreamSecretID
	assert.NotEqual(t, moved, replaced)
	assert.NotContains(t, raw, "secretRef")
	content = f.pulled(t, gateway)
	newHandle := renderedHandles(t, content)[0]
	assert.NotEqual(t, movedHandle, newHandle)
	assert.Equal(t, "mcp-new", f.credential(t, content))
	assert.NotContains(t, string(content), replaced.String(), "no database id reaches the artifact")
	sweep(t)
	_, ok = storedSecret(t, moved)
	assert.True(t, ok, "kept while the gateway may still be running the previous revision")
	settle(t, f.sync.ouID, f.artifact, models.DeploymentStatusDeployed)
	sweep(t)
	_, ok = storedSecret(t, moved)
	assert.False(t, ok)

	// No credential reaches a response.
	updated, err := f.svc.repo.GetByHandle(ctx, mcpSourceHandle(f.proxy), f.sync.ouID)
	require.NoError(t, err)
	assertNoCredentialIn(t, convertModelMCPProxyToSpec(updated), "mcp-new", replaced.String(), newHandle)

	// A PUT that omits auth keeps the stored one, as it always has.
	_, err = f.svc.Update(ctx, f.sync.ouID, mcpSourceHandle(f.proxy), f.updateRequest(nil))
	require.NoError(t, err)
	endpoint, _ = f.endpointRow(t)
	require.NotNil(t, endpoint.UpstreamSecretID)
	assert.Equal(t, replaced, *endpoint.UpstreamSecretID)
	assert.Equal(t, "mcp-new", f.credential(t, f.renderMCP(t)))

	// Replacing the endpoint with one that has no auth retires the secret of the one it replaces.
	req := f.updateRequest(nil)
	req.Endpoints[0].ID = "secondary"
	_, err = f.svc.Update(ctx, f.sync.ouID, mcpSourceHandle(f.proxy), req)
	require.NoError(t, err)
	endpoint, _ = f.endpointRow(t)
	assert.Nil(t, endpoint.UpstreamSecretID)
	assert.Empty(t, renderedHandles(t, f.pulled(t, gateway)))
	settle(t, f.sync.ouID, f.artifact, models.DeploymentStatusDeployed)
	sweep(t)
	_, ok = storedSecret(t, replaced)
	assert.False(t, ok)
}

func TestMCPEndpointDeletionReleasesItsCredential(t *testing.T) {
	f, _ := newMCPSecretFixture(t, vaulttest.Local(t))
	ctx := context.Background()
	_, err := f.svc.Update(ctx, f.sync.ouID, mcpSourceHandle(f.proxy), f.updateRequest(mcpAuth(strValue("mcp-owned"))))
	require.NoError(t, err)
	endpoint, _ := f.endpointRow(t)
	require.NotNil(t, endpoint.UpstreamSecretID)

	// The endpoint rows cascade from the proxy's artifact. Its per-environment artifact is still deployed, so the
	// gateway may still be running the credential.
	require.NoError(t, db.GetDB().Transaction(func(tx *gorm.DB) error {
		return repositories.NewArtifactRepo(tx).Delete(tx, f.proxy.UUID.String())
	}))
	sweep(t)
	_, ok := storedSecret(t, *endpoint.UpstreamSecretID)
	assert.True(t, ok)

	// Deleting that too, as a proxy delete does, releases it.
	require.NoError(t, db.GetDB().Transaction(func(tx *gorm.DB) error {
		return deleteMCPArtifactRows(tx, f.artifact, f.sync.ouID)
	}))
	sweep(t)
	_, ok = storedSecret(t, *endpoint.UpstreamSecretID)
	assert.False(t, ok)
}

// errInjected fails a render on purpose.
var errInjected = reconcile.Permanent(fmt.Errorf("%w: injected render failure", utils.ErrInvalidInput))

// With an external vault, a PUT that fails after replacing the endpoint rows leaves the credential the restored rows
// point to readable, and removal jobs never remove a value a committed row, or a deployment a gateway may run, uses.
func TestMCPFailedUpdateKeepsTheExternalCredentialReadable(t *testing.T) {
	vaults, external := vaulttest.LocalAndExternal(t)
	f, gateway := newMCPSecretFixture(t, vaults)
	runner := repositories.NewSecretJobRunner(db.GetDB(), vaults, slog.New(slog.DiscardHandler))
	ctx := context.Background()
	runJobs := func(offset int) {
		t.Helper()
		_, _, err := runner.RunOnce(ctx, dbx.Now().Add(timeHours(offset)))
		require.NoError(t, err)
	}

	_, err := f.svc.Update(ctx, f.sync.ouID, mcpSourceHandle(f.proxy), f.updateRequest(mcpAuth(strValue("v1"))))
	require.NoError(t, err)
	runJobs(0)
	settle(t, f.sync.ouID, f.artifact, models.DeploymentStatusDeployed)
	endpoint, _ := f.endpointRow(t)
	require.NotNil(t, endpoint.UpstreamSecretID)
	secretID := *endpoint.UpstreamSecretID
	secret, _ := storedSecret(t, secretID)
	require.Equal(t, vaulttest.ExternalProvider, secret.Provider, "the credential was promoted to the external vault")
	active := *secret.ExternalRef
	activeHandle := renderedHandles(t, f.pulled(t, gateway))[0]
	t.Cleanup(func() {
		_ = db.GetDB().Where("secret_id = ?", secretID).Delete(&models.SecretJob{}).Error
	})

	failRenders := func() func() {
		real := f.svc.kind.load
		f.svc.kind.load = func(context.Context, models.ArtifactRef) (*mcpArtifact, error) { return nil, errInjected }
		return func() { f.svc.kind.load = real }
	}
	assertActive := func() {
		t.Helper()
		endpoint, _ := f.endpointRow(t)
		require.NotNil(t, endpoint.UpstreamSecretID)
		assert.Equal(t, secretID, *endpoint.UpstreamSecretID)
		secret, ok := storedSecret(t, secretID)
		require.True(t, ok)
		require.NotNil(t, secret.ExternalRef)
		assert.Equal(t, active, *secret.ExternalRef)
		assert.True(t, external.Has(active))
		assert.Equal(t, []string{activeHandle}, renderedHandles(t, f.pulled(t, gateway)))
		assert.Equal(t, "v1", f.credential(t, f.pulled(t, gateway)))
		assert.Equal(t, "v1", f.credential(t, f.renderMCP(t)))
	}

	// A failed replacement, a failed removal and a failed update that keeps the credential all roll back.
	withoutAuth := f.updateRequest(nil)
	withoutAuth.Endpoints[0].ID = "secondary"
	for _, req := range []*models.MCPProxyDTO{
		f.updateRequest(mcpAuth(strValue("v2"))), withoutAuth, f.updateRequest(mcpAuth(nil)),
	} {
		restore := failRenders()
		_, err := f.svc.Update(ctx, f.sync.ouID, mcpSourceHandle(f.proxy), req)
		restore()
		require.Error(t, err)
		sweep(t)
		runJobs(2)
		assertActive()
	}

	// A failed redeploy changes nothing either.
	restore := failRenders()
	proxy, err := f.svc.repo.GetByHandle(ctx, mcpSourceHandle(f.proxy), f.sync.ouID)
	require.NoError(t, err)
	require.Error(t, f.svc.RedeployMCPProxy(ctx, proxy, f.sync.ouID))
	restore()
	sweep(t)
	runJobs(4)
	assertActive()

	// A removal of a value a committed row uses, queued twice and failing once, never removes it.
	external.FailDestroy = 1
	for range 2 {
		require.NoError(t, db.GetDB().Create(&models.SecretJob{
			ID: uuid.New(), OrgID: f.sync.ouID, SecretID: secretID, OwnerKind: "mcp_proxy_endpoint",
			Action: models.SecretJobDestroy, Provider: strValue(vaulttest.ExternalProvider), ExternalRef: &active,
			AvailableAt: dbx.Now(), CreatedAt: dbx.Now(),
		}).Error)
	}
	runJobs(6)
	runJobs(8)
	assertActive()
	assert.Equal(t, 1, external.FailDestroy, "the value in use was never sent for removal")

	// A committed replacement keeps the old value until the gateway has applied it, then removes it, retrying a
	// failed removal.
	_, err = f.svc.Update(ctx, f.sync.ouID, mcpSourceHandle(f.proxy), f.updateRequest(mcpAuth(strValue("v3"))))
	require.NoError(t, err)
	endpoint, _ = f.endpointRow(t)
	sweep(t)
	runJobs(10)
	assert.True(t, external.Has(active), "kept while the gateway may still be running the previous revision")
	settle(t, f.sync.ouID, f.artifact, models.DeploymentStatusDeployed)
	sweep(t)
	_, failed, err := runner.RunOnce(ctx, dbx.Now().Add(timeHours(12)))
	require.NoError(t, err)
	assert.Positive(t, failed, "the removal failed once")
	assert.True(t, external.Has(active))
	runJobs(16)
	runJobs(20)
	assert.False(t, external.Has(active))
	assert.Equal(t, "v3", f.credential(t, f.pulled(t, gateway)))
	newSecret, ok := storedSecret(t, *endpoint.UpstreamSecretID)
	require.True(t, ok)
	require.NotNil(t, newSecret.ExternalRef)
	assert.True(t, external.Has(*newSecret.ExternalRef))
}
