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

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsecret"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

func emptyAuth() *gwmodels.UpstreamAuth {
	authType, header, empty := "api-key", "Authorization", ""
	return &gwmodels.UpstreamAuth{Type: &authType, Header: &header, Value: &empty}
}

func valueAuth(v string) *gwmodels.UpstreamAuth {
	authType, header := "api-key", "Authorization"
	return &gwmodels.UpstreamAuth{Type: &authType, Header: &header, Value: &v}
}

func requireNoCredential(t *testing.T, auth *gwmodels.UpstreamAuth) {
	t.Helper()
	if auth == nil {
		return
	}
	require.Nil(t, auth.Value, "the empty value is not serialized")
	require.Nil(t, auth.SecretRef, "no reference is left dangling")
}

// An explicitly empty upstream value means no credential: an external vault
// that refuses unstaged writes must accept the create and the update, and an
// update that clears a credential deletes it.
func TestLLMProviderRepo_EmptyUpstreamAuth(t *testing.T) {
	gdb := db.GetDB()
	secrets := gwtest.NewStagingSecrets()
	repo := NewLLMProviderRepo(gdb, secrets)
	ctx := context.Background()
	org := newOrg(t)
	provider := func(auth *gwmodels.UpstreamAuth) *gwmodels.LLMProvider {
		return &gwmodels.LLMProvider{
			TemplateHandle: "openai", Status: "CREATED",
			Configuration: gwmodels.LLMProviderConfig{
				Name: "Empty Auth", Handle: "empty-auth", Version: "v1.0", Template: "openai",
				Upstream: &gwmodels.UpstreamConfig{Main: &gwmodels.UpstreamEndpoint{URL: "https://api.example.com", Auth: auth}},
			},
		}
	}

	p := provider(emptyAuth())
	ctx, err := repo.PrepareCreate(ctx, p, org)
	require.NoError(t, err)
	require.NoError(t, gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return repo.Create(tx, p, "empty-auth", "Empty Auth", "v1.0", org)
	}))
	stored, err := repo.GetByUUID(p.UUID.String(), org)
	require.NoError(t, err)
	requireNoCredential(t, stored.Configuration.Upstream.Main.Auth)
	require.Zero(t, secrets.Len())

	require.NoError(t, repo.Update(context.Background(), provider(valueAuth("sk-1")), p.UUID.String(), org))
	require.Equal(t, 1, secrets.Len())

	require.NoError(t, repo.Update(context.Background(), provider(emptyAuth()), p.UUID.String(), org))
	stored, err = repo.GetByUUID(p.UUID.String(), org)
	require.NoError(t, err)
	requireNoCredential(t, stored.Configuration.Upstream.Main.Auth)
	require.Zero(t, secrets.Len(), "the explicit clear deletes the old credential")
	_, err = secrets.Get(context.Background(), org, gwsecret.LLMProvider(p.UUID))
	require.Error(t, err)
}

func TestLLMProxyRepo_EmptyUpstreamAuth(t *testing.T) {
	gdb := db.GetDB()
	secrets := gwtest.NewStagingSecrets()
	repo := NewLLMProxyRepo(gdb, secrets)
	org := newOrg(t)
	_, seeded := seedLLMProxy(t, org)
	var base gwmodels.LLMProxy
	require.NoError(t, gdb.Where("id = ?", seeded).Take(&base).Error)

	handle := "empty-auth-proxy"
	p := &gwmodels.LLMProxy{
		ProjectUUID: base.ProjectUUID, ProviderUUID: base.ProviderUUID,
		Configuration: gwmodels.LLMProxyConfig{Name: "p", Version: "v1", UpstreamAuth: emptyAuth()},
	}
	require.NoError(t, repo.Create(context.Background(), p, handle, "p", "v1", org))
	var stored gwmodels.LLMProxy
	require.NoError(t, gdb.Where("id = ?", p.UUID).Take(&stored).Error)
	requireNoCredential(t, stored.Configuration.UpstreamAuth)
	require.Zero(t, secrets.Len())

	update := func(auth *gwmodels.UpstreamAuth) {
		require.NoError(t, repo.Update(&gwmodels.LLMProxy{
			ProjectUUID: base.ProjectUUID, ProviderUUID: base.ProviderUUID,
			Configuration: gwmodels.LLMProxyConfig{Name: "p", Version: "v1", UpstreamAuth: auth},
		}, handle, org))
	}
	update(valueAuth("sk-1"))
	require.Equal(t, 1, secrets.Len())

	update(emptyAuth())
	require.NoError(t, gdb.Where("id = ?", p.UUID).Take(&stored).Error)
	requireNoCredential(t, stored.Configuration.UpstreamAuth)
	require.Zero(t, secrets.Len(), "the explicit clear deletes the old credential")
}
