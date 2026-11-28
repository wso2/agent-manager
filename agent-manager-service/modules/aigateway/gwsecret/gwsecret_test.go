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

// Package gwsecret keeps the gateway's credentials in the module's secret
package gwsecret_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsecret"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

func TestSealStoresTheValueAndLeavesItsPath(t *testing.T) {
	ctx, secrets := context.Background(), gwtest.NewSecrets()
	path := gwsecret.LLMProxy(uuid.New())
	value := "sk-upstream"
	auth := &gwmodels.UpstreamAuth{Value: &value}

	require.NoError(t, gwsecret.Seal(ctx, secrets, "org-1", path, auth))

	assert.Nil(t, auth.Value, "the row never holds the value")
	require.NotNil(t, auth.SecretRef)
	assert.Equal(t, path, *auth.SecretRef)
	got, err := secrets.Get(ctx, "org-1", path)
	require.NoError(t, err)
	assert.Equal(t, value, got)

	require.NoError(t, gwsecret.Seal(ctx, secrets, "org-1", path, auth), "a kept reference is left alone")
	assert.Equal(t, 1, secrets.Len())
}

func TestSealDeletesTheValueOfARowWithoutCredential(t *testing.T) {
	ctx, secrets := context.Background(), gwtest.NewSecrets()
	path := gwsecret.LLMProvider(uuid.New())
	require.NoError(t, secrets.Put(ctx, "org-1", path, "old"))

	require.NoError(t, gwsecret.Seal(ctx, secrets, "org-1", path, &gwmodels.UpstreamAuth{}))

	assert.Zero(t, secrets.Len())
}

func TestSealRefusesAnotherResourcesReference(t *testing.T) {
	other := gwsecret.LLMProxy(uuid.New())
	err := gwsecret.Seal(context.Background(), gwtest.NewSecrets(), "org-1", gwsecret.LLMProxy(uuid.New()),
		&gwmodels.UpstreamAuth{SecretRef: &other})
	assert.ErrorContains(t, err, "does not belong to this resource")
}

func TestStoreRefusesUntilActivated(t *testing.T) {
	st := gwsecret.NewStore()
	_, err := st.In(nil).Get(context.Background(), "org-1", "a")
	require.Error(t, err)

	st.Use(gwtest.NewSecrets())
	require.NoError(t, st.In(nil).Put(context.Background(), "org-1", "a", "v"))
	got, err := st.Get(context.Background(), "org-1", "a")
	require.NoError(t, err)
	assert.Equal(t, "v", got)
}

func TestMCPEndpointPathsDifferByHandle(t *testing.T) {
	id := uuid.New()
	assert.NotEqual(t, gwsecret.MCPEndpoint(id, "a"), gwsecret.MCPEndpoint(id, "b"))
	assert.NotContains(t, gwsecret.MCPEndpoint(id, "a/../b"), "..", "free-text handles are hashed")
}

// stagingSecrets records what Prepare was asked to stage and refuses empty values, as an external vault does.
type stagingSecrets struct {
	*gwtest.Secrets
	staged []string
}

func (s *stagingSecrets) Prepare(ctx context.Context, _, path, value string) (context.Context, error) {
	if value == "" {
		return ctx, errors.New("secret value cannot be empty")
	}
	s.staged = append(s.staged, path)
	return ctx, nil
}

// An empty value is treated as no credential before sealing, so staging must not reject it first.
func TestPrepareStagesNothingForAnEmptyValue(t *testing.T) {
	secrets := &stagingSecrets{Secrets: gwtest.NewSecrets()}
	empty, value := "", "sk-upstream"
	for _, auth := range []*gwmodels.UpstreamAuth{nil, {}, {Value: &empty}} {
		_, err := gwsecret.Prepare(context.Background(), secrets, "org-1", "p", auth)
		require.NoError(t, err)
	}
	assert.Empty(t, secrets.staged)

	_, err := gwsecret.Prepare(context.Background(), secrets, "org-1", "p", &gwmodels.UpstreamAuth{Value: &value})
	require.NoError(t, err)
	assert.Equal(t, []string{"p"}, secrets.staged)
}

func TestTransactionStorePreparesThroughTheActiveStore(t *testing.T) {
	st := gwsecret.NewStore()
	secrets := &stagingSecrets{Secrets: gwtest.NewSecrets()}
	st.Use(secrets)
	_, err := st.In(nil).Prepare(context.Background(), "org-1", "p", "v")
	require.NoError(t, err)
	assert.Equal(t, []string{"p"}, secrets.staged)
}

// TestStoreCreatesOnlyThroughACreatingStore: Create keeps the value already stored, needs an active store, and
// is not implemented by a store that cannot create atomically.
func TestStoreCreatesOnlyThroughACreatingStore(t *testing.T) {
	ctx := context.Background()
	st := gwsecret.NewStore()
	_, err := st.Create(ctx, "org-1", "k", "first")
	require.Error(t, err, "no store before activation")
	_, err = st.Prepare(ctx, "org-1", "k", "first")
	require.Error(t, err, "no store before activation")

	st.Use(gwtest.NewSecrets())
	got, err := st.Create(ctx, "org-1", "k", "first")
	require.NoError(t, err)
	assert.Equal(t, "first", got)
	got, err = st.Create(ctx, "org-1", "k", "second")
	require.NoError(t, err)
	assert.Equal(t, "first", got, "an existing value is kept")

	st.Use(createlessSecrets{gwtest.NewSecrets()})
	_, err = st.Create(ctx, "org-1", "k", "v")
	assert.ErrorIs(t, err, modkit.ErrNotImplemented)
}

// createlessSecrets exposes only modkit.Secrets, hiding the wrapped store's Create.
type createlessSecrets struct{ modkit.Secrets }
