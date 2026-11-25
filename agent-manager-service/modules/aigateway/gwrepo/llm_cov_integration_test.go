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

package gwrepo

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsecret"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

func covStr(s string) *string { return &s }

// covProvider creates an LLM provider whose upstream carries the credential secret, and returns it and its handle.
func covProvider(t *testing.T, repo LLMProviderRepository, ou, secret string) (*gwmodels.LLMProvider, string) {
	t.Helper()
	handle := "prov-" + uuid.NewString()[:8]
	p := &gwmodels.LLMProvider{
		TemplateHandle: "openai",
		Configuration: gwmodels.LLMProviderConfig{
			Name: handle, Handle: handle, Version: "v1",
			Upstream: &gwmodels.UpstreamConfig{Main: &gwmodels.UpstreamEndpoint{
				URL: "https://llm.example.com", Auth: &gwmodels.UpstreamAuth{Type: covStr("api-key"), Value: covStr(secret)},
			}},
		},
	}
	require.NoError(t, repo.Create(db.GetDB(), p, handle, handle, "v1", ou))
	return p, handle
}

// An LLM provider seals its credential, is found by id and handle, listed, counted, and its deletion is guarded by
// its proxies and the deleting flag.
func TestCovLLMProviderRepoLifecycle(t *testing.T) {
	gdb := db.GetDB()
	secrets := gwtest.NewSecrets()
	repo := NewLLMProviderRepo(gdb, secrets)
	ou := newOrg(t)
	p, handle := covProvider(t, repo, ou, "sk-provider")
	covProvider(t, repo, ou, "sk-second")

	assert.Equal(t, gwmodels.StatusCreated, p.Status)
	stored, err := secrets.Get(context.Background(), ou, gwsecret.LLMProvider(p.UUID))
	require.NoError(t, err)
	assert.Equal(t, "sk-provider", stored, "the credential goes to the secret store")
	byHandle, err := repo.GetByHandle(handle, ou)
	require.NoError(t, err)
	assert.Equal(t, p.UUID, byHandle.UUID)
	assert.True(t, byHandle.InCatalog, "a provider is listed in the catalog")
	require.NotNil(t, byHandle.Configuration.Upstream.Main.Auth.SecretRef)
	assert.Nil(t, byHandle.Configuration.Upstream.Main.Auth.Value, "the row keeps only the reference")
	_, err = repo.GetByHandle(handle, newOrg(t))
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = repo.GetByUUID(uuid.NewString(), ou)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	exists, err := repo.Exists(handle, ou)
	require.NoError(t, err)
	assert.True(t, exists)
	n, err := repo.Count(ou)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	page1, err := repo.List(ou, 1, 0)
	require.NoError(t, err)
	page2, err := repo.List(ou, 1, 1)
	require.NoError(t, err)
	require.Len(t, page1, 1)
	require.Len(t, page2, 1)
	assert.NotEqual(t, page1[0].UUID, page2[0].UUID)

	// The deleting flag is set once, read under a lock, and cleared.
	marked, err := repo.MarkDeleting(p.UUID)
	require.NoError(t, err)
	assert.True(t, marked)
	marked, err = repo.MarkDeleting(p.UUID)
	require.NoError(t, err)
	assert.False(t, marked, "a second delete is a conflict")
	require.NoError(t, gdb.Transaction(func(tx *gorm.DB) error {
		deleting, err := repo.IsDeletingForUpdate(context.Background(), tx, p.UUID)
		require.NoError(t, err)
		assert.True(t, deleting)
		_, err = repo.IsDeletingForUpdate(context.Background(), tx, uuid.New())
		require.ErrorIs(t, err, gwerr.ErrLLMProviderNotFound)
		return nil
	}))
	require.NoError(t, repo.ClearDeleting(p.UUID))
	got, err := repo.GetByUUID(p.UUID.String(), ou)
	require.NoError(t, err)
	assert.Equal(t, gwmodels.StatusCreated, got.Status)

	// A provider with a proxy cannot be deleted.
	proxies := NewLLMProxyRepo(gdb, secrets)
	project := dbtest.Project(t, gdb, ou, "llm-"+uuid.NewString()[:8])
	require.NoError(t, proxies.Create(context.Background(), &gwmodels.LLMProxy{ProjectUUID: project, ProviderUUID: p.UUID},
		"px-"+uuid.NewString()[:8], "px", "v1", ou))
	has, err := repo.HasAssociatedProxies(context.Background(), p.UUID)
	require.NoError(t, err)
	assert.True(t, has)
	require.ErrorIs(t, repo.Delete(p.UUID.String(), ou), gwerr.ErrLLMProviderHasProxies)

	require.Error(t, repo.Delete("not-a-uuid", ou))
	require.Error(t, repo.DeleteTx(context.Background(), gdb, "not-a-uuid", ou))
	require.ErrorIs(t, repo.Delete(uuid.NewString(), ou), gorm.ErrRecordNotFound)

	lone, _ := covProvider(t, repo, ou, "sk-lone")
	require.NoError(t, gdb.Transaction(func(tx *gorm.DB) error {
		return repo.DeleteTx(context.Background(), tx, lone.UUID.String(), ou)
	}))
	_, err = repo.GetByUUID(lone.UUID.String(), ou)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = secrets.Get(context.Background(), ou, gwsecret.LLMProvider(lone.UUID))
	require.Error(t, err, "deleting the provider deletes its credential")
}

// An LLM proxy is created against a live provider, read by handle (in or out of its project), listed and counted
// by organization, project and provider, updated, and deleted with its credential.
func TestCovLLMProxyRepoLifecycle(t *testing.T) {
	gdb := db.GetDB()
	secrets := gwtest.NewSecrets()
	providers := NewLLMProviderRepo(gdb, secrets)
	repo := NewLLMProxyRepo(gdb, secrets)
	ou := newOrg(t)
	p1, _ := covProvider(t, providers, ou, "sk-1")
	p2, _ := covProvider(t, providers, ou, "sk-2")
	projA := dbtest.Project(t, gdb, ou, "a-"+uuid.NewString()[:8])
	projB := dbtest.Project(t, gdb, ou, "b-"+uuid.NewString()[:8])

	create := func(project, provider uuid.UUID, secret *string) (*gwmodels.LLMProxy, string) {
		handle := "px-" + uuid.NewString()[:8]
		px := &gwmodels.LLMProxy{ProjectUUID: project, ProviderUUID: provider, Description: "d"}
		if secret != nil {
			px.Configuration.UpstreamAuth = &gwmodels.UpstreamAuth{Type: covStr("api-key"), Value: secret}
		}
		require.NoError(t, repo.Create(context.Background(), px, handle, handle, "v1", ou))
		return px, handle
	}
	x1, h1 := create(projA, p1.UUID, covStr("sk-proxy"))
	_, h2 := create(projA, p2.UUID, nil)
	_, h3 := create(projB, p1.UUID, nil)

	assert.Equal(t, gwmodels.StatusCreated, x1.Status)
	stored, err := secrets.Get(context.Background(), ou, gwsecret.LLMProxy(x1.UUID))
	require.NoError(t, err)
	assert.Equal(t, "sk-proxy", stored)

	require.ErrorIs(t, repo.Create(context.Background(), &gwmodels.LLMProxy{ProjectUUID: projA, ProviderUUID: uuid.New()},
		"px-orphan", "x", "v1", ou), gwerr.ErrLLMProviderNotFound)
	marked, err := providers.MarkDeleting(p2.UUID)
	require.NoError(t, err)
	require.True(t, marked)
	require.ErrorIs(t, repo.Create(context.Background(), &gwmodels.LLMProxy{ProjectUUID: projA, ProviderUUID: p2.UUID},
		"px-late", "x", "v1", ou), gwerr.ErrLLMProviderBeingDeleted)
	require.NoError(t, providers.ClearDeleting(p2.UUID))

	got, err := repo.GetByID(h1, ou)
	require.NoError(t, err)
	assert.Equal(t, x1.UUID, got.UUID)
	assert.Equal(t, h1, got.Handle)
	assert.Equal(t, "v1", got.Version)
	_, err = repo.GetByID(h1, newOrg(t))
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	inProject, err := repo.GetByIDAndProject(h1, ou, projA.String())
	require.NoError(t, err)
	assert.Equal(t, x1.UUID, inProject.UUID)
	_, err = repo.GetByIDAndProject(h1, ou, projB.String())
	require.ErrorIs(t, err, gorm.ErrRecordNotFound, "another project's proxy is not found")

	handles := func(ps []*gwmodels.LLMProxy) []string {
		out := make([]string, 0, len(ps))
		for _, p := range ps {
			out = append(out, p.Handle)
		}
		return out
	}
	all, err := repo.List(ou, 10, 0)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{h1, h2, h3}, handles(all))
	ofA, err := repo.ListByProject(ou, projA.String(), 10, 0)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{h1, h2}, handles(ofA))
	ofP1, err := repo.ListByProvider(ou, p1.UUID.String(), 1, 0)
	require.NoError(t, err)
	ofP1rest, err := repo.ListByProvider(ou, p1.UUID.String(), 1, 1)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{h1, h3}, append(handles(ofP1), handles(ofP1rest)...), "paging covers each proxy once")
	for _, c := range []struct {
		name  string
		count func() (int, error)
		want  int
	}{
		{"org", func() (int, error) { return repo.Count(ou) }, 3},
		{"project", func() (int, error) { return repo.CountByProject(ou, projA.String()) }, 2},
		{"provider", func() (int, error) { return repo.CountByProvider(ou, p1.UUID.String()) }, 2},
	} {
		n, err := c.count()
		require.NoError(t, err, c.name)
		assert.Equal(t, c.want, n, c.name)
	}
	exists, err := repo.Exists(h2, ou)
	require.NoError(t, err)
	assert.True(t, exists)

	update := &gwmodels.LLMProxy{Description: "updated", ProviderUUID: p2.UUID, OpenAPISpec: "openapi: 3.0.0"}
	update.Configuration.Context = covStr("/ctx")
	require.NoError(t, repo.Update(update, h1, ou))
	got, err = repo.GetByID(h1, ou)
	require.NoError(t, err)
	assert.Equal(t, "updated", got.Description)
	assert.Equal(t, p2.UUID, got.ProviderUUID)
	require.NotNil(t, got.Configuration.Context)
	assert.Equal(t, "/ctx", *got.Configuration.Context)
	require.ErrorIs(t, repo.Update(update, "px-missing", ou), gorm.ErrRecordNotFound)

	// An agent's LLM configuration makes it an agent proxy; deleting it in its project removes the mapping too.
	mapped, err := repo.IsAgentOrMonitorProxy(context.Background(), h3, ou)
	require.NoError(t, err)
	assert.False(t, mapped)
	cfgID, _ := mapLLMProxyToAgent(t, ou, covProxyID(t, repo, h3, ou))
	mapped, err = repo.IsAgentOrMonitorProxy(context.Background(), h3, ou)
	require.NoError(t, err)
	assert.True(t, mapped)

	require.Error(t, repo.DeleteInProject(context.Background(), h3, ou, projA.String()), "not in that project")
	require.NoError(t, repo.DeleteInProject(context.Background(), h3, ou, projB.String()))
	var mappings int64
	require.NoError(t, gdb.Model(&gwmodels.EnvAgentModelMapping{}).Where("config_id = ?", cfgID).Count(&mappings).Error)
	assert.Zero(t, mappings)

	require.NoError(t, repo.Delete(h1, ou))
	_, err = repo.GetByID(h1, ou)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = secrets.Get(context.Background(), ou, gwsecret.LLMProxy(x1.UUID))
	require.Error(t, err, "deleting the proxy deletes its credential")
	require.ErrorIs(t, repo.Delete(h1, ou), gorm.ErrRecordNotFound)
	n, err := repo.Count(ou)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func covProxyID(t *testing.T, repo LLMProxyRepository, handle, ou string) uuid.UUID {
	t.Helper()
	p, err := repo.GetByIDCtx(context.Background(), handle, ou)
	require.NoError(t, err)
	return p.UUID
}

// A template an LLM provider of its organization uses is not deleted; one used only in another organization is.
func TestLLMProviderTemplateRepoDeleteRefusesATemplateInUse(t *testing.T) {
	gdb := db.GetDB()
	providers := NewLLMProviderRepo(gdb, gwtest.NewSecrets())
	templates := NewLLMProviderTemplateRepo(gdb)
	ou, other := newOrg(t), newOrg(t)
	for _, org := range []string{ou, other} {
		require.NoError(t, templates.Create(&gwmodels.LLMProviderTemplate{OUID: org, Handle: "custom", Name: "Custom"}))
	}
	p, _ := covProvider(t, providers, ou, "sk-provider")
	require.NoError(t, gdb.Model(&gwmodels.LLMProvider{}).Where("id = ?", p.UUID).Update("template_handle", "custom").Error)

	require.ErrorIs(t, templates.Delete("custom", ou), gwerr.ErrLLMProviderTemplateInUse)
	exists, err := templates.Exists("custom", ou)
	require.NoError(t, err)
	require.True(t, exists, "a template in use is kept")

	require.NoError(t, templates.Delete("custom", other), "another organization's provider doesn't hold the template")

	require.NoError(t, providers.Delete(p.UUID.String(), ou))
	require.NoError(t, templates.Delete("custom", ou))
	require.ErrorIs(t, templates.Delete("custom", ou), gorm.ErrRecordNotFound)
}
