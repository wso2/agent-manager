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

package gwsvc

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

func TestSelfHostedVhost(t *testing.T) {
	for endpoint, want := range map[string]string{
		"  https://gw.example.com\n": "https://gw.example.com",
		"https://gw.example.com/":    "https://gw.example.com",
		"http://gw.example.com:8443": "http://gw.example.com:8443",
	} {
		got, err := selfHostedVhost([]string{endpoint})
		require.NoError(t, err, endpoint)
		require.Equal(t, want, got, "the stored vhost is the endpoint's scheme and host")
	}

	for _, endpoints := range [][]string{
		{"pending://awaiting-connection"},
		{},
		{"gw.example.com"},
		{"http://:80"},
		{"https://user:pw@gw.example.com"},
		{"https://gw.example.com/base"},
		{"https://gw.example.com?x=1"},
		{"https://gw.example.com/#frag"},
		{"https://gw.example.com?"},
		{"https://gw.example.com#"},
		{"https://gw.example.com:99999"},
	} {
		_, err := selfHostedVhost(endpoints)
		require.ErrorIs(t, err, httpx.ErrInvalidInput, "endpoints %v", endpoints)
	}
}

func registerRepo(created **gwmodels.Gateway, nameTaken bool) *gwrepomocks.GatewayRepositoryMock {
	existing := &gwmodels.Gateway{UUID: uuid.New()}
	repo := mappingRepo(nil, 0)
	repo.GetByNameAndOrgIDFunc = func(string, string) (*gwmodels.Gateway, error) {
		if nameTaken {
			return existing, nil
		}
		return nil, gwerr.ErrGatewayNotFound
	}
	repo.CreateTxFunc = func(_ *gorm.DB, gw *gwmodels.Gateway) error {
		*created = gw
		return nil
	}
	return repo
}

func TestRegisterSelfHostedGateway(t *testing.T) {
	env := uuid.NewString()
	input := func(functionalityType string) SelfHostedGatewayInput {
		return SelfHostedGatewayInput{
			DisplayName: "Edge One", Handle: " edge-one ", Environment: env, FunctionalityType: functionalityType,
			Endpoints: []string{"https://edge.example.com"}, Version: "2026.09.24",
		}
	}

	for _, functionalityType := range []string{"", "ai"} {
		t.Run("accepts "+functionalityType, func(t *testing.T) {
			var created *gwmodels.Gateway
			var mapped []string
			repo := registerRepo(&created, false)
			repo.CreateEnvironmentMappingTxFunc = func(_ *gorm.DB, m *gwmodels.GatewayEnvironmentMapping) error {
				mapped = append(mapped, m.EnvironmentUUID.String())
				return nil
			}
			svc := NewPlatformGatewayService(repo)

			resp, err := svc.RegisterSelfHostedGateway("org", input(functionalityType))

			require.NoError(t, err)
			require.NotNil(t, created)
			require.Equal(t, gwmodels.GatewayHostingSelfHosted, created.Hosting)
			require.Equal(t, gwmodels.GatewayRoleEgress, created.GatewayFunctionalityType)
			require.Equal(t, "https://edge.example.com", created.Vhost)
			require.Equal(t, "edge-one", created.Name, "the handle is stored as given, trimmed, with no environment prefix")
			require.Equal(t, []string{env}, mapped)
			require.Equal(t, "SELF_HOSTED", string(resp.Hosting))
		})
	}

	t.Run("regular is refused", func(t *testing.T) {
		var created *gwmodels.Gateway
		svc := NewPlatformGatewayService(registerRepo(&created, false))

		_, err := svc.RegisterSelfHostedGateway("org", input("regular"))

		require.ErrorIs(t, err, httpx.ErrInvalidInput)
		require.Nil(t, created)
	})

	t.Run("a missing or invalid handle is refused", func(t *testing.T) {
		for _, handle := range []string{"", "  ", "Edge One", "-edge", "edge-", "ab", strings.Repeat("a", 65)} {
			var created *gwmodels.Gateway
			svc := NewPlatformGatewayService(registerRepo(&created, false))
			in := input("ai")
			in.Handle = handle

			_, err := svc.RegisterSelfHostedGateway("org", in)

			require.ErrorIs(t, err, httpx.ErrInvalidInput, "handle %q", handle)
			require.Nil(t, created)
		}
	})

	t.Run("a taken handle is a conflict", func(t *testing.T) {
		var created *gwmodels.Gateway
		svc := NewPlatformGatewayService(registerRepo(&created, true))

		_, err := svc.RegisterSelfHostedGateway("org", input("ai"))

		require.ErrorIs(t, err, gwerr.ErrGatewayAlreadyExists)
		require.Nil(t, created)
	})
}

func TestListGatewaysFiltersByHostingAndOrg(t *testing.T) {
	var counted, listed gwrepo.GatewayFilterOptions
	repo := &gwrepomocks.GatewayRepositoryMock{
		CountWithFiltersFunc: func(f gwrepo.GatewayFilterOptions) (int64, error) {
			counted = f
			return 0, nil
		},
		ListWithFiltersFunc: func(f gwrepo.GatewayFilterOptions) ([]*gwmodels.Gateway, error) {
			listed = f
			return nil, nil
		},
	}
	svc := NewPlatformGatewayService(repo)

	_, err := svc.ListGateways(strPtr("org-1"), &GatewayListFilters{Hosting: gwmodels.GatewayHostingSelfHosted}, 10, 5)

	require.NoError(t, err)
	for _, f := range []gwrepo.GatewayFilterOptions{counted, listed} {
		require.Equal(t, gwmodels.GatewayHostingSelfHosted, f.Hosting)
		require.Equal(t, "org-1", f.OrganizationID)
	}
	require.Equal(t, 10, listed.Limit)
	require.Equal(t, 5, listed.Offset)
}

func storedGateway(hosting, ouID string) *gwmodels.Gateway {
	return &gwmodels.Gateway{UUID: uuid.New(), OUID: ouID, Name: "gw-one", Hosting: hosting}
}

func TestRequireSelfHosted(t *testing.T) {
	dbDown := errors.New("connection refused")
	tests := []struct {
		name    string
		stored  *gwmodels.Gateway
		byName  bool
		repoErr error
		wantErr error
	}{
		{name: "self-hosted by UUID", stored: storedGateway(gwmodels.GatewayHostingSelfHosted, "org")},
		{name: "self-hosted by name", stored: storedGateway(gwmodels.GatewayHostingSelfHosted, "org"), byName: true},
		{name: "managed", stored: storedGateway(gwmodels.GatewayHostingManaged, "org"), wantErr: gwerr.ErrGatewayManaged},
		{name: "other org", stored: storedGateway(gwmodels.GatewayHostingSelfHosted, "other"), wantErr: gwerr.ErrGatewayNotFound},
		{name: "missing", wantErr: gwerr.ErrGatewayNotFound},
		{name: "repository failure is not a missing gateway", repoErr: dbDown, wantErr: dbDown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookup := func() (*gwmodels.Gateway, error) {
				if tt.repoErr != nil {
					return nil, tt.repoErr
				}
				if tt.stored == nil {
					return nil, gwerr.ErrGatewayNotFound
				}
				return tt.stored, nil
			}
			repo := &gwrepomocks.GatewayRepositoryMock{
				GetByNameAndOrgIDFunc: func(string, string) (*gwmodels.Gateway, error) { return lookup() },
				GetByUUIDFunc:         func(string) (*gwmodels.Gateway, error) { return lookup() },
			}
			svc := NewPlatformGatewayService(repo)
			ref := " " + uuid.NewString() + " "
			if tt.byName {
				ref = " edge "
			}

			got, err := svc.RequireSelfHosted(ref, "org")

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				if tt.repoErr != nil {
					require.NotErrorIs(t, err, gwerr.ErrGatewayNotFound)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.stored, got)
			if tt.byName {
				require.Empty(t, repo.GetByUUIDCalls(), "a name is not looked up as a UUID")
			} else {
				require.Empty(t, repo.GetByNameAndOrgIDCalls(), "a UUID is looked up only as a UUID")
			}
		})
	}
}

func TestRemoveGatewayFromEnvironment_SelfHostedRefused(t *testing.T) {
	gw := storedGateway(gwmodels.GatewayHostingSelfHosted, "org")
	repo := &gwrepomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(string) (*gwmodels.Gateway, error) { return gw, nil },
		// TransactionFunc left nil: any repository mutation panics.
	}
	svc := NewPlatformGatewayService(repo)

	err := svc.RemoveGatewayFromEnvironment(context.Background(), gw.UUID.String(), uuid.NewString())

	require.ErrorIs(t, err, gwerr.ErrGatewayEnvironmentFixed)
}

// A token is listed with only the start of its prefix: enough to match it to the token
// a gateway uses, never the full lookup key.
func TestListTokens_ShowsShortPrefix(t *testing.T) {
	gw := storedGateway(gwmodels.GatewayHostingSelfHosted, "org")
	prefix := "9b2c41d0-5717-4562-b3fc-2c963f66afa6"
	repo := &gwrepomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(string) (*gwmodels.Gateway, error) { return gw, nil },
		GetActiveTokensByGatewayUUIDFunc: func(string) ([]*gwmodels.GatewayToken, error) {
			return []*gwmodels.GatewayToken{{UUID: uuid.New(), TokenPrefix: prefix, Status: "active"}}, nil
		},
	}
	svc := NewPlatformGatewayService(repo)

	resp, err := svc.ListTokens(gw.UUID.String(), "org")

	require.NoError(t, err)
	require.Len(t, resp.List, 1)
	require.Equal(t, "9b2c41d0", resp.List[0].Prefix)
	require.Equal(t, "", shownTokenPrefix(""))
}

// The mode a gateway reports is what an identity provider change does to it: a
// self-hosted gateway's owner applies it by hand; nothing applies it to a managed one.
func TestIdentityProviderMode(t *testing.T) {
	managed := storedGateway(gwmodels.GatewayHostingManaged, "org")
	selfHosted := storedGateway(gwmodels.GatewayHostingSelfHosted, "org")

	require.Equal(t, IdentityProviderModeUnsupported, toGatewayResponse(managed).IdentityProviderMode)
	require.Equal(t, IdentityProviderModeManual, toGatewayResponse(selfHosted).IdentityProviderMode)
}

// A self-hosted gateway's identity providers are recorded in the mirror: its owner
// keeps the key managers in the gateway's config.toml.
func TestIdentityProvider_SelfHostedRecordsMirror(t *testing.T) {
	gw := storedGateway(gwmodels.GatewayHostingSelfHosted, "org")
	var upserted *gwmodels.GatewayIdentityProvider
	var deleted string
	repo := &gwrepomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(string) (*gwmodels.Gateway, error) { return gw, nil },
		UpsertIdentityProviderFunc: func(p *gwmodels.GatewayIdentityProvider) error {
			upserted = p
			return nil
		},
		DeleteIdentityProviderFunc: func(gatewayID, name string) error {
			deleted = gatewayID + "/" + name
			return nil
		},
	}
	svc := NewPlatformGatewayService(repo)

	provider, err := svc.UpsertIdentityProvider(context.Background(), gw.UUID.String(), "org", "idp",
		"https://issuer", "https://issuer/jwks", "", false)
	require.NoError(t, err)
	require.Equal(t, gw.UUID, provider.GatewayUUID)
	require.Same(t, provider, upserted)

	require.NoError(t, svc.DeleteIdentityProvider(context.Background(), gw.UUID.String(), "org", "idp"))
	require.Equal(t, gw.UUID.String()+"/idp", deleted)
}

// Another organization's gateway stays not found, whatever its hosting.
func TestIdentityProvider_OtherOrgIsNotFound(t *testing.T) {
	gw := storedGateway(gwmodels.GatewayHostingSelfHosted, "other")
	repo := &gwrepomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(string) (*gwmodels.Gateway, error) { return gw, nil },
		// Every other func left nil: a repository mutation panics.
	}
	svc := NewPlatformGatewayService(repo)

	_, err := svc.UpsertIdentityProvider(context.Background(), gw.UUID.String(), "org", "idp",
		"https://issuer", "https://issuer/jwks", "", false)
	require.ErrorIs(t, err, gwerr.ErrGatewayNotFound)

	err = svc.DeleteIdentityProvider(context.Background(), gw.UUID.String(), "org", "idp")
	require.ErrorIs(t, err, gwerr.ErrGatewayNotFound)
}

// Nothing applies a custom identity provider to a managed gateway (UNSUPPORTED), so
// adding one is refused before anything is recorded; a system provider is still
// recorded, and a custom one can still be removed.
func TestIdentityProvider_ManagedRefusesCustom(t *testing.T) {
	gw := storedGateway(gwmodels.GatewayHostingManaged, "org")
	var upserted []string
	var deleted string
	repo := &gwrepomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(string, string) (*gwmodels.Gateway, error) { return gw, nil },
		UpsertIdentityProviderFunc: func(p *gwmodels.GatewayIdentityProvider) error {
			upserted = append(upserted, p.Name)
			return nil
		},
		DeleteIdentityProviderFunc: func(gatewayID, name string) error {
			deleted = gatewayID + "/" + name
			return nil
		},
	}
	svc := NewPlatformGatewayService(repo)

	_, err := svc.UpsertIdentityProvider(context.Background(), gw.UUID.String(), "org", "idp",
		"https://issuer", "https://issuer/jwks", "", false)
	require.ErrorIs(t, err, gwerr.ErrGatewayIdentityProviderUnsupported)
	require.Empty(t, upserted, "a refused provider is not recorded")

	system := "ThunderKeyManager"
	require.True(t, gwmodels.IsSystemIdentityProvider(system))
	provider, err := svc.UpsertIdentityProvider(context.Background(), gw.UUID.String(), "org", system,
		"https://thunder", "https://thunder/jwks", "", false)
	require.NoError(t, err)
	require.Equal(t, gwmodels.IdentityProviderTypeSystem, provider.Type)
	require.Equal(t, []string{system}, upserted)

	require.NoError(t, svc.DeleteIdentityProvider(context.Background(), gw.UUID.String(), "org", "idp"))
	require.Equal(t, gw.UUID.String()+"/idp", deleted)
}
