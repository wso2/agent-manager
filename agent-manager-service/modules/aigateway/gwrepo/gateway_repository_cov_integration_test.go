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
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

// covGateway registers a gateway through the repository and returns it.
func covGateway(t *testing.T, repo GatewayRepository, ou, role, hosting string) *gwmodels.Gateway {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	gw := &gwmodels.Gateway{
		UUID: id, OUID: ou, Name: "cov-" + id.String()[24:], DisplayName: "Cov", Vhost: "cov.example.com",
		GatewayFunctionalityType: role, Hosting: hosting, IsActive: true,
	}
	require.NoError(t, repo.Create(gw))
	return gw
}

func covGatewayIDs(gateways []*gwmodels.Gateway) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(gateways))
	for _, g := range gateways {
		ids = append(ids, g.UUID)
	}
	return ids
}

// A registered gateway starts inactive and managed, is found by id and name, and its details and liveness update.
func TestCovGatewayRepoRegistersReadsAndUpdatesGateways(t *testing.T) {
	repo := NewGatewayRepo(db.GetDB())
	ou := newOrg(t)
	gw := covGateway(t, repo, ou, gwmodels.GatewayRoleIngress, "")

	got, err := repo.GetByUUID(gw.UUID.String())
	require.NoError(t, err)
	assert.False(t, got.IsActive, "a gateway is inactive until it connects")
	assert.Equal(t, gwmodels.GatewayHostingManaged, got.Hosting)
	assert.Equal(t, ou, got.OUID)

	byName, err := repo.GetByNameAndOrgID(gw.Name, ou)
	require.NoError(t, err)
	assert.Equal(t, gw.UUID, byName.UUID)
	_, err = repo.GetByNameAndOrgID(gw.Name, newOrg(t))
	require.ErrorIs(t, err, gwerr.ErrGatewayNotFound, "names are per organization")
	_, err = repo.GetByUUID(uuid.NewString())
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	dup := *gw
	dup.UUID = uuid.Must(uuid.NewV7())
	require.Error(t, repo.Create(&dup), "a name is unique in an organization")

	second := covGateway(t, repo, ou, gwmodels.GatewayRoleEgress, gwmodels.GatewayHostingSelfHosted)
	inOrg, err := repo.GetByOrganizationID(context.Background(), ou)
	require.NoError(t, err)
	assert.ElementsMatch(t, []uuid.UUID{gw.UUID, second.UUID}, covGatewayIDs(inOrg))
	all, err := repo.List()
	require.NoError(t, err)
	assert.Subset(t, covGatewayIDs(all), []uuid.UUID{gw.UUID, second.UUID})

	got.DisplayName, got.Description, got.IsCritical = "Renamed", "described", true
	got.Properties, got.RuntimeURL = map[string]any{"region": "eu"}, "https://rt.example.com"
	require.NoError(t, repo.UpdateGateway(got))
	updated, err := repo.GetByUUID(gw.UUID.String())
	require.NoError(t, err)
	assert.Equal(t, "Renamed", updated.DisplayName)
	assert.Equal(t, "described", updated.Description)
	assert.True(t, updated.IsCritical)
	assert.Equal(t, "eu", updated.Properties["region"])
	assert.Equal(t, "https://rt.example.com", updated.RuntimeURL)
	missing := *got
	missing.UUID = uuid.New()
	require.ErrorIs(t, repo.UpdateGateway(&missing), gorm.ErrRecordNotFound)

	require.NoError(t, repo.UpdateActiveStatus(gw.UUID.String(), true))
	updated, err = repo.GetByUUID(gw.UUID.String())
	require.NoError(t, err)
	assert.True(t, updated.IsActive)
	require.ErrorIs(t, repo.UpdateActiveStatus(uuid.NewString(), true), gorm.ErrRecordNotFound)

	require.ErrorIs(t, repo.Delete(uuid.NewString(), ou), gwerr.ErrGatewayNotFound)
	require.ErrorIs(t, repo.Delete(gw.UUID.String(), newOrg(t)), gwerr.ErrGatewayNotFound, "another org cannot delete it")
}

// The list filters narrow by organization, role, hosting, liveness and environment, and the count ignores paging.
func TestCovGatewayRepoListFiltersAndPages(t *testing.T) {
	gdb := db.GetDB()
	repo := NewGatewayRepo(gdb)
	ou := newOrg(t)
	ingress := covGateway(t, repo, ou, gwmodels.GatewayRoleIngress, gwmodels.GatewayHostingManaged)
	egress := covGateway(t, repo, ou, gwmodels.GatewayRoleEgress, gwmodels.GatewayHostingSelfHosted)
	both := covGateway(t, repo, ou, gwmodels.GatewayRoleBoth, gwmodels.GatewayHostingManaged)
	require.NoError(t, repo.UpdateActiveStatus(both.UUID.String(), true))
	env := dbtest.Environment(t, gdb, ou)
	require.NoError(t, repo.CreateEnvironmentMapping(&gwmodels.GatewayEnvironmentMapping{GatewayUUID: egress.UUID, EnvironmentUUID: env}))
	covGateway(t, repo, newOrg(t), gwmodels.GatewayRoleIngress, "")

	role, active, envID := gwmodels.GatewayRoleIngress, true, env.String()
	for _, c := range []struct {
		name    string
		filters GatewayFilterOptions
		want    []uuid.UUID
	}{
		{"org", GatewayFilterOptions{OrganizationID: ou}, []uuid.UUID{ingress.UUID, egress.UUID, both.UUID}},
		{"role", GatewayFilterOptions{OrganizationID: ou, FunctionalityType: &role}, []uuid.UUID{ingress.UUID}},
		{"roles", GatewayFilterOptions{OrganizationID: ou, FunctionalityTypeIn: gwmodels.IngressGatewayRoles}, []uuid.UUID{ingress.UUID, both.UUID}},
		{"hosting", GatewayFilterOptions{OrganizationID: ou, Hosting: gwmodels.GatewayHostingSelfHosted}, []uuid.UUID{egress.UUID}},
		{"active", GatewayFilterOptions{OrganizationID: ou, Status: &active}, []uuid.UUID{both.UUID}},
		{"environment", GatewayFilterOptions{OrganizationID: ou, EnvironmentID: &envID}, []uuid.UUID{egress.UUID}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := repo.ListWithFilters(c.filters)
			require.NoError(t, err)
			assert.ElementsMatch(t, c.want, covGatewayIDs(got))
			n, err := repo.CountWithFilters(c.filters)
			require.NoError(t, err)
			assert.EqualValues(t, len(c.want), n)
		})
	}

	first, err := repo.ListWithFilters(GatewayFilterOptions{OrganizationID: ou, Limit: 2})
	require.NoError(t, err)
	rest, err := repo.ListWithFilters(GatewayFilterOptions{OrganizationID: ou, Limit: 2, Offset: 2})
	require.NoError(t, err)
	require.Len(t, first, 2)
	require.Len(t, rest, 1)
	assert.ElementsMatch(t, []uuid.UUID{ingress.UUID, egress.UUID, both.UUID}, append(covGatewayIDs(first), covGatewayIDs(rest)...))
	n, err := repo.CountWithFilters(GatewayFilterOptions{OrganizationID: ou, Limit: 1, Offset: 1})
	require.NoError(t, err)
	assert.EqualValues(t, 3, n, "the count ignores paging")
}

// A token is found by id and prefix while active; revoking it takes it out of the active set.
func TestCovGatewayRepoTokensLifecycle(t *testing.T) {
	repo := NewGatewayRepo(db.GetDB())
	ou := newOrg(t)
	gw := covGateway(t, repo, ou, gwmodels.GatewayRoleBoth, "")
	prefix := uuid.NewString()
	token := &gwmodels.GatewayToken{
		UUID: uuid.New(), GatewayUUID: gw.UUID, TokenPrefix: prefix, TokenHash: "h", Salt: "s", Status: "active",
	}
	require.NoError(t, repo.CreateToken(token))
	assert.Equal(t, ou, token.OrgID.String(), "the token takes its gateway's organization")
	require.Error(t, repo.CreateToken(&gwmodels.GatewayToken{UUID: uuid.New(), GatewayUUID: uuid.New(), Status: "active"}),
		"a token needs a gateway")

	byID, err := repo.GetTokenByUUID(token.UUID.String())
	require.NoError(t, err)
	assert.Equal(t, gw.UUID, byID.GatewayUUID)
	_, err = repo.GetTokenByUUID(uuid.NewString())
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	byPrefix, err := repo.GetActiveTokenByPrefix(context.Background(), prefix)
	require.NoError(t, err)
	assert.Equal(t, token.UUID, byPrefix.UUID)
	active, err := repo.GetActiveTokensByGatewayUUID(gw.UUID.String())
	require.NoError(t, err)
	require.Len(t, active, 1)
	n, err := repo.CountActiveTokens(gw.UUID.String())
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	require.NoError(t, repo.RevokeToken(token.UUID.String()))
	require.ErrorIs(t, repo.RevokeToken(uuid.NewString()), gorm.ErrRecordNotFound)
	_, err = repo.GetActiveTokenByPrefix(context.Background(), prefix)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound, "a revoked token no longer authenticates")
	revoked, err := repo.GetTokenByUUID(token.UUID.String())
	require.NoError(t, err)
	assert.Equal(t, "revoked", revoked.Status)
	assert.NotNil(t, revoked.RevokedAt)
	n, err = repo.CountActiveTokens(gw.UUID.String())
	require.NoError(t, err)
	assert.Zero(t, n)
}

// Environment mappings are read per gateway, per gateways and per environment, and removed singly or per environment.
func TestCovGatewayRepoEnvironmentMappings(t *testing.T) {
	gdb := db.GetDB()
	repo := NewGatewayRepo(gdb)
	ou := newOrg(t)
	a := covGateway(t, repo, ou, gwmodels.GatewayRoleIngress, "")
	b := covGateway(t, repo, ou, gwmodels.GatewayRoleEgress, "")
	env1, env2 := envOf(t, ou, "dev"), envOf(t, ou, "prod")
	for _, m := range []struct{ gw, env uuid.UUID }{{a.UUID, env1}, {a.UUID, env2}, {b.UUID, env1}} {
		mapping := &gwmodels.GatewayEnvironmentMapping{GatewayUUID: m.gw, EnvironmentUUID: m.env}
		require.NoError(t, repo.CreateEnvironmentMapping(mapping))
		assert.Equal(t, ou, mapping.OrgID.String())
	}
	require.Error(t, repo.CreateEnvironmentMapping(&gwmodels.GatewayEnvironmentMapping{GatewayUUID: a.UUID, EnvironmentUUID: env1}),
		"a gateway maps to an environment once")

	exists, err := repo.EnvironmentMappingExists(a.UUID.String(), env2.String())
	require.NoError(t, err)
	assert.True(t, exists)
	exists, err = repo.EnvironmentMappingExists(b.UUID.String(), env2.String())
	require.NoError(t, err)
	assert.False(t, exists)

	ofA, err := repo.GetEnvironmentMappingsByGatewayID(a.UUID.String())
	require.NoError(t, err)
	assert.Len(t, ofA, 2)
	grouped, err := repo.GetEnvironmentMappingsByGatewayIDs([]string{a.UUID.String(), b.UUID.String(), uuid.NewString()})
	require.NoError(t, err)
	assert.Len(t, grouped[a.UUID.String()], 2)
	assert.Len(t, grouped[b.UUID.String()], 1)
	assert.Len(t, grouped, 2)
	empty, err := repo.GetEnvironmentMappingsByGatewayIDs(nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
	inEnv1, err := repo.GetEnvironmentMappingsByEnvironmentID(env1.String())
	require.NoError(t, err)
	assert.Len(t, inEnv1, 2)

	require.NoError(t, repo.DeleteEnvironmentMapping(a.UUID.String(), env2.String()))
	require.ErrorIs(t, repo.DeleteEnvironmentMapping(a.UUID.String(), env2.String()), gorm.ErrRecordNotFound)
	removed, err := repo.DeleteEnvironmentMappingsByEnvironmentID(env1.String())
	require.NoError(t, err)
	assert.EqualValues(t, 2, removed)
	ofA, err = repo.GetEnvironmentMappingsByGatewayID(a.UUID.String())
	require.NoError(t, err)
	assert.Empty(t, ofA)
}

// Identity providers upsert by gateway and name, list per gateway, environment (deduplicated) and organization, and
// the reserved and system ones are protected.
func TestCovGatewayRepoIdentityProviders(t *testing.T) {
	gdb := db.GetDB()
	repo := NewGatewayRepo(gdb)
	ou := newOrg(t)
	a := covGateway(t, repo, ou, gwmodels.GatewayRoleIngress, "")
	b := covGateway(t, repo, ou, gwmodels.GatewayRoleEgress, "")
	env := envOf(t, ou, "dev")
	for _, g := range []uuid.UUID{a.UUID, b.UUID} {
		require.NoError(t, repo.CreateEnvironmentMapping(&gwmodels.GatewayEnvironmentMapping{GatewayUUID: g, EnvironmentUUID: env}))
	}
	idp := func(gw uuid.UUID, name, issuer, typ string) *gwmodels.GatewayIdentityProvider {
		return &gwmodels.GatewayIdentityProvider{
			UUID: uuid.New(), GatewayUUID: gw, Name: name, Issuer: issuer, JWKSUri: issuer + "/jwks", Type: typ,
		}
	}

	require.ErrorIs(t, repo.UpsertIdentityProvider(idp(a.UUID, gwmodels.ReservedIdentityProviderName, "https://x", gwmodels.IdentityProviderTypeCustom)),
		httpx.ErrInvalidInput)
	require.Error(t, repo.UpsertIdentityProvider(idp(uuid.New(), "orphan", "https://x", gwmodels.IdentityProviderTypeCustom)),
		"a provider needs a gateway")
	require.NoError(t, repo.UpsertIdentityProvider(idp(a.UUID, "corp", "https://old", gwmodels.IdentityProviderTypeCustom)))
	require.NoError(t, repo.UpsertIdentityProvider(idp(a.UUID, "corp", "https://new", gwmodels.IdentityProviderTypeCustom)))
	require.NoError(t, repo.UpsertIdentityProvider(idp(b.UUID, "corp", "https://new", gwmodels.IdentityProviderTypeCustom)))
	require.NoError(t, repo.UpsertIdentityProvider(idp(a.UUID, "platform", "https://p", gwmodels.IdentityProviderTypeSystem)))

	ofA, err := repo.ListIdentityProvidersByGateway(a.UUID.String())
	require.NoError(t, err)
	require.Len(t, ofA, 2)
	assert.Equal(t, "corp", ofA[0].Name, "ordered by name")
	assert.Equal(t, "https://new", ofA[0].Issuer, "the upsert updated the row in place")
	inEnv, err := repo.ListIdentityProvidersByEnvironment(env.String())
	require.NoError(t, err)
	assert.Len(t, inEnv, 2, "one per name across the environment's gateways")
	inOrg, err := repo.ListIdentityProvidersByOrg(ou)
	require.NoError(t, err)
	assert.Len(t, inOrg, 3)
	for _, p := range inOrg {
		assert.Equal(t, env.String(), p.EnvironmentUUID)
		assert.NotEmpty(t, p.GatewayName)
	}

	require.ErrorIs(t, repo.DeleteIdentityProvider(a.UUID.String(), gwmodels.ReservedIdentityProviderName), httpx.ErrInvalidInput)
	require.ErrorIs(t, repo.DeleteIdentityProvider(a.UUID.String(), "ThunderKeyManager"), httpx.ErrInvalidInput)
	// A provider's recorded type protects it, not its name: an env-Thunder-style name
	// recorded as system stays, one recorded as custom can be removed.
	require.NoError(t, repo.UpsertIdentityProvider(idp(b.UUID, "amp-thunder-org-env", "https://t", gwmodels.IdentityProviderTypeSystem)))
	require.ErrorIs(t, repo.DeleteIdentityProvider(b.UUID.String(), "amp-thunder-org-env"), httpx.ErrInvalidInput)
	require.NoError(t, repo.UpsertIdentityProvider(idp(b.UUID, "amp-thunder-lookalike", "https://l", gwmodels.IdentityProviderTypeCustom)))
	require.NoError(t, repo.DeleteIdentityProvider(b.UUID.String(), "amp-thunder-lookalike"))
	require.ErrorIs(t, repo.DeleteIdentityProvider(a.UUID.String(), "platform"), httpx.ErrInvalidInput, "a system provider stays")
	require.ErrorIs(t, repo.DeleteIdentityProvider(a.UUID.String(), "missing"), gorm.ErrRecordNotFound)
	require.NoError(t, repo.DeleteIdentityProvider(a.UUID.String(), "corp"))
	ofA, err = repo.ListIdentityProvidersByGateway(a.UUID.String())
	require.NoError(t, err)
	require.Len(t, ofA, 1)
	assert.Equal(t, "platform", ofA[0].Name)
}

// Registration in a transaction locks, inserts and counts the environment's ingress gateways, and rolls back as one.
func TestCovGatewayRepoTransactionalRegistration(t *testing.T) {
	gdb := db.GetDB()
	repo := NewGatewayRepo(gdb)
	ou := newOrg(t)
	env := envOf(t, ou, "dev")
	id := uuid.Must(uuid.NewV7())
	gw := &gwmodels.Gateway{UUID: id, OUID: ou, Name: "tx-" + id.String()[24:], Vhost: "v", GatewayFunctionalityType: gwmodels.GatewayRoleBoth}

	require.NoError(t, repo.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, repo.AcquireEnvironmentLock(tx, env.String()))
		n, err := repo.CountIngressCapableInEnvironment(tx, env.String())
		require.NoError(t, err)
		assert.Zero(t, n)
		require.NoError(t, repo.CreateTx(tx, gw))
		require.NoError(t, repo.AcquireGatewayLock(tx, gw.UUID.String()))
		require.NoError(t, repo.WithTx(tx).CreateEnvironmentMapping(&gwmodels.GatewayEnvironmentMapping{GatewayUUID: gw.UUID, EnvironmentUUID: env}))
		n, err = repo.CountIngressCapableInEnvironment(tx, env.String())
		require.NoError(t, err)
		assert.EqualValues(t, 1, n)
		return nil
	}))
	got, err := repo.GetByUUID(id.String())
	require.NoError(t, err)
	assert.Equal(t, gwmodels.GatewayHostingManaged, got.Hosting)
	assert.NotNil(t, got.Properties)

	egress := covGateway(t, repo, ou, gwmodels.GatewayRoleEgress, "")
	require.NoError(t, repo.CreateEnvironmentMapping(&gwmodels.GatewayEnvironmentMapping{GatewayUUID: egress.UUID, EnvironmentUUID: env}))
	n, err := repo.CountIngressCapableInEnvironment(gdb, env.String())
	require.NoError(t, err)
	assert.EqualValues(t, 1, n, "an egress gateway takes no ingress slot")

	rolledBack := uuid.Must(uuid.NewV7())
	errStop := errors.New("stop")
	err = repo.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, repo.CreateTx(tx, &gwmodels.Gateway{
			UUID: rolledBack, OUID: ou, Name: "rb-" + rolledBack.String()[24:], Vhost: "v", GatewayFunctionalityType: gwmodels.GatewayRoleIngress,
		}))
		return errStop
	})
	require.ErrorIs(t, err, errStop)
	_, err = repo.GetByUUID(rolledBack.String())
	require.ErrorIs(t, err, gorm.ErrRecordNotFound, "the failed transaction left no gateway")
	require.Error(t, repo.CreateEnvironmentMappingTx(gdb, &gwmodels.GatewayEnvironmentMapping{GatewayUUID: uuid.New(), EnvironmentUUID: env}))
}
