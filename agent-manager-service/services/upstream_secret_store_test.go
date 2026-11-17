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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
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

// renderedSecretRefs collects every secretRef in a rendered artifact.
func renderedSecretRefs(t *testing.T, content []byte) []string {
	t.Helper()
	var doc any
	require.NoError(t, yaml.Unmarshal(content, &doc))
	var refs []string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if ref, ok := x["secretRef"].(string); ok {
				refs = append(refs, ref)
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
	return refs
}

// renderedCredential renders content's single secretRef back to the credential the gateway receives.
func renderedCredential(t *testing.T, content []byte) string {
	t.Helper()
	refs := renderedSecretRefs(t, content)
	require.Len(t, refs, 1, "the artifact carries one encrypted credential")
	plaintext, err := utils.DecryptSecretRef(refs[0], fixtureKey)
	require.NoError(t, err)
	return plaintext
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
	secretID := *row.UpstreamSecretID
	assert.NotContains(t, raw, "secretRef", "the configuration no longer carries the credential")
	assert.NotContains(t, raw, "sk-first")
	secret, ok := storedSecret(t, secretID)
	require.True(t, ok)
	assert.Equal(t, f.sync.ouID, secret.OrgID)
	assert.Equal(t, "llm_provider", secret.OwnerKind)
	assert.Equal(t, created.UUID, secret.OwnerID)

	content := f.render(t, created.UUID)
	assert.Equal(t, "sk-first", renderedCredential(t, content))
	assert.NotContains(t, string(content), secretID.String(), "no database id reaches the artifact")

	got, err := f.providers.Get(created.UUID.String(), f.sync.ouID)
	require.NoError(t, err)
	token := renderedSecretRefs(t, content)[0]
	assertNoCredentialIn(t, utils.ConvertModelToSpecLLMProviderResponse(got), "sk-first", token, secretID.String())
	assertNoCredentialIn(t, utils.ConvertModelToSpecLLMProviderListItemResponse(got), "sk-first", token, secretID.String())

	// An update that omits the credential keeps the secret as it is.
	got.Description = "edited"
	_, err = f.providers.Update(ctx, created.UUID.String(), f.sync.ouID, got)
	require.NoError(t, err)
	row, _ = storedProvider(t, created.UUID)
	require.NotNil(t, row.UpstreamSecretID)
	assert.Equal(t, secretID, *row.UpstreamSecretID)
	secret, _ = storedSecret(t, secretID)
	assert.Equal(t, 1, secret.Version, "an omitted credential is not rewritten")
	assert.Equal(t, "sk-first", renderedCredential(t, f.render(t, created.UUID)))

	// A new credential rewrites the same secret.
	got, err = f.providers.Get(created.UUID.String(), f.sync.ouID)
	require.NoError(t, err)
	withAuth(got, strValue("sk-second"))
	_, err = f.providers.Update(ctx, created.UUID.String(), f.sync.ouID, got)
	require.NoError(t, err)
	row, raw = storedProvider(t, created.UUID)
	assert.Equal(t, secretID, *row.UpstreamSecretID)
	assert.NotContains(t, raw, "secretRef")
	secret, _ = storedSecret(t, secretID)
	assert.Equal(t, 2, secret.Version)
	assert.Equal(t, "sk-second", renderedCredential(t, f.render(t, created.UUID)))

	// The direct render path reads the moved credential too.
	got, err = f.providers.Get(created.UUID.String(), f.sync.ouID)
	require.NoError(t, err)
	direct, err := f.deployment.generateLLMProviderDeploymentYAML(got, f.sync.ouID)
	require.NoError(t, err)
	assert.Equal(t, "sk-second", renderedCredential(t, []byte(direct)))

	// Removing the auth releases the secret.
	got, err = f.providers.Get(created.UUID.String(), f.sync.ouID)
	require.NoError(t, err)
	got.Configuration.Upstream.Main.Auth = nil
	_, err = f.providers.Update(ctx, created.UUID.String(), f.sync.ouID, got)
	require.NoError(t, err)
	row, _ = storedProvider(t, created.UUID)
	assert.Nil(t, row.UpstreamSecretID)
	_, ok = storedSecret(t, secretID)
	assert.False(t, ok)
	assert.Empty(t, renderedSecretRefs(t, f.render(t, created.UUID)))
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
	_, ok = storedSecret(t, *row.UpstreamSecretID)
	assert.False(t, ok)
}

// A provider saved before credentials moved keeps its encrypted credential inline. It is read as it is, and moves to
// the secret store the next time the provider is saved.
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

	assert.Equal(t, "sk-legacy", renderedCredential(t, f.render(t, created.UUID)))
	got, err := f.providers.Get(created.UUID.String(), f.sync.ouID)
	require.NoError(t, err)
	direct, err := f.deployment.generateLLMProviderDeploymentYAML(got, f.sync.ouID)
	require.NoError(t, err)
	assert.Equal(t, "sk-legacy", renderedCredential(t, []byte(direct)))
	assertNoCredentialIn(t, utils.ConvertModelToSpecLLMProviderResponse(got), "sk-legacy", token)

	// The controller carries the stored credential into an update that omits it; saving moves it.
	got.Description = "moved"
	_, err = f.providers.Update(ctx, created.UUID.String(), f.sync.ouID, got)
	require.NoError(t, err)
	row, raw := storedProvider(t, created.UUID)
	require.NotNil(t, row.UpstreamSecretID)
	assert.NotContains(t, raw, "secretRef", "the inline copy is gone once the secret holds it")
	assert.Equal(t, "sk-legacy", renderedCredential(t, f.render(t, created.UUID)))
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
	direct, err := f.proxies.generateLLMProxyDeploymentYAML(proxy, f.sync.ouID)
	require.NoError(t, err)
	assert.Equal(t, string(rendered.Content), direct)
}

// mcpEndpointRow is the fixture proxy's single endpoint as stored.
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
	gateway := f.mapGateway(t, f.env)
	require.NoError(t, f.deploy(t))
	return f, gateway
}

func TestMCPEndpointUpstreamCredentialLivesInTheSecretStore(t *testing.T) {
	f, gateway := newMCPSecretFixture(t, vaulttest.Local(t))
	ctx := context.Background()

	// The fixture's endpoint was saved with its credential inline, as before credentials moved: it renders as stored.
	_, raw := f.endpointRow(t)
	assert.Contains(t, raw, "secretRef")
	assert.Equal(t, []string{"mcp-backend-key"}, renderedSecretRefs(t, f.pulled(t, gateway)))

	// A PUT that omits the credential keeps it, moving it into the secret store.
	_, err := f.svc.Update(ctx, f.sync.ouID, mcpSourceHandle(f.proxy), f.updateRequest(mcpAuth(nil)))
	require.NoError(t, err)
	endpoint, raw := f.endpointRow(t)
	require.NotNil(t, endpoint.UpstreamSecretID)
	moved := *endpoint.UpstreamSecretID
	assert.NotContains(t, raw, "secretRef")
	assert.Equal(t, []string{"mcp-backend-key"}, renderedSecretRefs(t, f.pulled(t, gateway)))
	secret, ok := storedSecret(t, moved)
	require.True(t, ok)
	assert.Equal(t, "mcp_proxy_endpoint", secret.OwnerKind)
	assert.Equal(t, endpoint.UUID, secret.OwnerID)

	// Another omitted credential: the replacement endpoint row takes the same secret over, unchanged.
	_, err = f.svc.Update(ctx, f.sync.ouID, mcpSourceHandle(f.proxy), f.updateRequest(mcpAuth(nil)))
	require.NoError(t, err)
	endpoint, _ = f.endpointRow(t)
	require.NotNil(t, endpoint.UpstreamSecretID)
	assert.Equal(t, moved, *endpoint.UpstreamSecretID)
	secret, _ = storedSecret(t, moved)
	assert.Equal(t, 1, secret.Version)
	assert.Equal(t, endpoint.UUID, secret.OwnerID, "the secret names the endpoint row that holds it now")

	// A new credential gets a new secret and releases the old one.
	_, err = f.svc.Update(ctx, f.sync.ouID, mcpSourceHandle(f.proxy), f.updateRequest(mcpAuth(strValue("mcp-new"))))
	require.NoError(t, err)
	endpoint, raw = f.endpointRow(t)
	require.NotNil(t, endpoint.UpstreamSecretID)
	replaced := *endpoint.UpstreamSecretID
	assert.NotEqual(t, moved, replaced)
	assert.NotContains(t, raw, "secretRef")
	_, ok = storedSecret(t, moved)
	assert.False(t, ok)
	content := f.pulled(t, gateway)
	assert.Equal(t, "mcp-new", renderedCredential(t, content))
	assert.NotContains(t, string(content), replaced.String(), "no database id reaches the artifact")

	// No credential reaches a response.
	updated, err := f.svc.repo.GetByHandle(ctx, mcpSourceHandle(f.proxy), f.sync.ouID)
	require.NoError(t, err)
	token := renderedSecretRefs(t, content)[0]
	assertNoCredentialIn(t, convertModelMCPProxyToSpec(updated), "mcp-new", token, replaced.String())

	// A PUT that omits auth keeps the stored one, as it always has.
	_, err = f.svc.Update(ctx, f.sync.ouID, mcpSourceHandle(f.proxy), f.updateRequest(nil))
	require.NoError(t, err)
	endpoint, _ = f.endpointRow(t)
	require.NotNil(t, endpoint.UpstreamSecretID)
	assert.Equal(t, replaced, *endpoint.UpstreamSecretID)
	assert.Equal(t, "mcp-new", renderedCredential(t, f.pulled(t, gateway)))

	// Replacing the endpoint with one that has no auth releases the secret of the one it replaces.
	req := f.updateRequest(nil)
	req.Endpoints[0].ID = "secondary"
	_, err = f.svc.Update(ctx, f.sync.ouID, mcpSourceHandle(f.proxy), req)
	require.NoError(t, err)
	endpoint, _ = f.endpointRow(t)
	assert.Nil(t, endpoint.UpstreamSecretID)
	_, ok = storedSecret(t, replaced)
	assert.False(t, ok)
	assert.Empty(t, renderedSecretRefs(t, f.pulled(t, gateway)))
}

func TestMCPEndpointDeletionReleasesItsCredential(t *testing.T) {
	f, _ := newMCPSecretFixture(t, vaulttest.Local(t))
	ctx := context.Background()
	_, err := f.svc.Update(ctx, f.sync.ouID, mcpSourceHandle(f.proxy), f.updateRequest(mcpAuth(strValue("mcp-owned"))))
	require.NoError(t, err)
	endpoint, _ := f.endpointRow(t)
	require.NotNil(t, endpoint.UpstreamSecretID)

	// The endpoint rows cascade from the proxy's artifact.
	require.NoError(t, db.GetDB().Transaction(func(tx *gorm.DB) error {
		return repositories.NewArtifactRepo(tx).Delete(tx, f.proxy.UUID.String())
	}))
	_, ok := storedSecret(t, *endpoint.UpstreamSecretID)
	assert.False(t, ok)
}

// errInjected fails a render on purpose.
var errInjected = reconcile.Permanent(fmt.Errorf("%w: injected render failure", utils.ErrInvalidInput))

// With an external vault, a PUT that fails after replacing the endpoint rows leaves the credential the restored rows
// point to readable, and removal jobs never remove a value a committed row uses.
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
	endpoint, _ := f.endpointRow(t)
	require.NotNil(t, endpoint.UpstreamSecretID)
	secretID := *endpoint.UpstreamSecretID
	secret, _ := storedSecret(t, secretID)
	require.Equal(t, vaulttest.ExternalProvider, secret.Provider, "the credential was promoted to the external vault")
	active := *secret.ExternalRef
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
		assert.Equal(t, "v1", renderedCredential(t, f.pulled(t, gateway)))
		rendered, err := f.svc.kind.Render(ctx, models.ArtifactRef{OUID: f.sync.ouID, ArtifactUUID: f.artifact})
		require.NoError(t, err)
		assert.Equal(t, "v1", renderedCredential(t, rendered.Content))
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
		runJobs(2)
		assertActive()
	}

	// A failed redeploy changes nothing either.
	restore := failRenders()
	proxy, err := f.svc.repo.GetByHandle(ctx, mcpSourceHandle(f.proxy), f.sync.ouID)
	require.NoError(t, err)
	require.Error(t, f.svc.RedeployMCPProxy(ctx, proxy, f.sync.ouID))
	restore()
	runJobs(4)
	assertActive()

	// A removal of a value a committed row uses, queued twice and failing once, never removes it.
	external.FailDestroy = 1
	require.NoError(t, db.GetDB().Create(&models.SecretJob{
		ID: uuid.New(), OrgID: f.sync.ouID, SecretID: secretID, OwnerKind: "mcp_proxy_endpoint",
		Action: models.SecretJobDestroy, Provider: strValue(vaulttest.ExternalProvider), ExternalRef: &active,
		AvailableAt: dbx.Now(), CreatedAt: dbx.Now(),
	}).Error)
	runJobs(6)
	runJobs(8)
	assertActive()
	assert.Equal(t, 1, external.FailDestroy, "the value in use was never sent for removal")

	// A committed replacement removes the old value only after the new one is in place, retrying a failure.
	_, err = f.svc.Update(ctx, f.sync.ouID, mcpSourceHandle(f.proxy), f.updateRequest(mcpAuth(strValue("v3"))))
	require.NoError(t, err)
	endpoint, _ = f.endpointRow(t)
	assert.True(t, external.Has(active), "the old value outlives the commit until the job runs")
	_, failed, err := runner.RunOnce(ctx, dbx.Now().Add(timeHours(10)))
	require.NoError(t, err)
	assert.Positive(t, failed, "the removal failed once")
	assert.True(t, external.Has(active))
	runJobs(14)
	runJobs(18)
	assert.False(t, external.Has(active))
	assert.Equal(t, "v3", renderedCredential(t, f.pulled(t, gateway)))
	newSecret, ok := storedSecret(t, *endpoint.UpstreamSecretID)
	require.True(t, ok)
	require.NotNil(t, newSecret.ExternalRef)
	assert.True(t, external.Has(*newSecret.ExternalRef))
}
