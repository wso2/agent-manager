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
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
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
		require.ErrorIs(t, err, utils.ErrInvalidInput, "endpoints %v", endpoints)
	}
}

func registerRepo(created **gwmodels.Gateway, nameTaken bool) *gwrepomocks.GatewayRepositoryMock {
	existing := &gwmodels.Gateway{UUID: uuid.New()}
	repo := mappingRepo(nil, 0)
	repo.GetByNameAndOrgIDFunc = func(string, string) (*gwmodels.Gateway, error) {
		if nameTaken {
			return existing, nil
		}
		return nil, utils.ErrGatewayNotFound
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
			svc := NewPlatformGatewayService(repo, nil)

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
		svc := NewPlatformGatewayService(registerRepo(&created, false), nil)

		_, err := svc.RegisterSelfHostedGateway("org", input("regular"))

		require.ErrorIs(t, err, utils.ErrInvalidInput)
		require.Nil(t, created)
	})

	t.Run("a missing or invalid handle is refused", func(t *testing.T) {
		for _, handle := range []string{"", "  ", "Edge One", "-edge", "edge-", "ab", strings.Repeat("a", 65)} {
			var created *gwmodels.Gateway
			svc := NewPlatformGatewayService(registerRepo(&created, false), nil)
			in := input("ai")
			in.Handle = handle

			_, err := svc.RegisterSelfHostedGateway("org", in)

			require.ErrorIs(t, err, utils.ErrInvalidInput, "handle %q", handle)
			require.Nil(t, created)
		}
	})

	t.Run("a taken handle is a conflict", func(t *testing.T) {
		var created *gwmodels.Gateway
		svc := NewPlatformGatewayService(registerRepo(&created, true), nil)

		_, err := svc.RegisterSelfHostedGateway("org", input("ai"))

		require.ErrorIs(t, err, utils.ErrGatewayAlreadyExists)
		require.Nil(t, created)
	})
}

func TestListSelfHostedGatewaysFiltersByHostingAndOrg(t *testing.T) {
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
	svc := NewPlatformGatewayService(repo, nil)

	_, err := svc.ListSelfHostedGateways("org-1", 10, 5)

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
	tests := []struct {
		name    string
		stored  *gwmodels.Gateway
		wantErr bool
	}{
		{name: "self-hosted", stored: storedGateway(gwmodels.GatewayHostingSelfHosted, "org")},
		{name: "managed", stored: storedGateway(gwmodels.GatewayHostingManaged, "org"), wantErr: true},
		{name: "other org", stored: storedGateway(gwmodels.GatewayHostingSelfHosted, "other"), wantErr: true},
		{name: "missing", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &gwrepomocks.GatewayRepositoryMock{
				GetByUUIDFunc: func(string) (*gwmodels.Gateway, error) {
					if tt.stored == nil {
						return nil, utils.ErrGatewayNotFound
					}
					return tt.stored, nil
				},
			}
			svc := NewPlatformGatewayService(repo, nil)

			got, err := svc.RequireSelfHosted(uuid.NewString(), "org")

			if tt.wantErr {
				require.ErrorIs(t, err, utils.ErrGatewayNotFound)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.stored, got)
		})
	}
}

func TestRemoveGatewayFromEnvironment_SelfHostedRefused(t *testing.T) {
	gw := storedGateway(gwmodels.GatewayHostingSelfHosted, "org")
	repo := &gwrepomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(string) (*gwmodels.Gateway, error) { return gw, nil },
		// TransactionFunc left nil: any repository mutation panics.
	}
	svc := NewPlatformGatewayService(repo, nil)

	err := svc.RemoveGatewayFromEnvironment(context.Background(), gw.UUID.String(), uuid.NewString())

	require.ErrorIs(t, err, utils.ErrGatewayEnvironmentFixed)
}

// panickingApplier fails the test by panicking on any call.
type panickingApplier struct{}

func (panickingApplier) ApplyIdentityProvider(context.Context, string, string, gwmodels.GatewayIdentityProvider) error {
	panic("applier called for a self-hosted gateway")
}

func (panickingApplier) DeleteIdentityProvider(context.Context, string, string, string) error {
	panic("applier called for a self-hosted gateway")
}

func TestIdentityProvider_SelfHostedIsNotFound(t *testing.T) {
	gw := storedGateway(gwmodels.GatewayHostingSelfHosted, "org")
	repo := &gwrepomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(string, string) (*gwmodels.Gateway, error) { return gw, nil },
		// Every other func left nil: a repository mutation panics.
	}
	svc := NewPlatformGatewayService(repo, panickingApplier{})

	_, err := svc.UpsertIdentityProvider(context.Background(), gw.UUID.String(), "org", "idp", "https://issuer", "https://issuer/jwks", "", false)
	require.ErrorIs(t, err, utils.ErrGatewayNotFound)

	err = svc.DeleteIdentityProvider(context.Background(), gw.UUID.String(), "org", "idp")
	require.ErrorIs(t, err, utils.ErrGatewayNotFound)
}
