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
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func TestSelfHostedHandle(t *testing.T) {
	tests := []struct {
		name, env, handle, display string
		want                       string
		wantErr                    bool
	}{
		{name: "slug from display name", env: "dev", display: "Default AI Self-Hosted Gateway", want: "dev-default-ai-self-hosted-gateway"},
		{name: "explicit handle", env: "dev", handle: "edge-1", display: "x", want: "dev-edge-1"},
		{name: "underscores and punctuation", env: "dev", display: "My_Edge  GW!", want: "dev-my-edge-gw"},
		{name: "nothing left of the slug", env: "dev", display: "!!", wantErr: true},
		{name: "too long", env: "dev", display: strings.Repeat("a", 70), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selfHostedHandle(tt.env, tt.handle, tt.display)
			if tt.wantErr {
				require.ErrorIs(t, err, utils.ErrInvalidInput)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestSelfHostedVhost(t *testing.T) {
	got, err := selfHostedVhost([]string{"  https://gw.example.com\n"})
	require.NoError(t, err)
	require.Equal(t, "https://gw.example.com", got, "the stored vhost is the trimmed endpoint")

	for _, endpoints := range [][]string{
		{"pending://awaiting-connection"}, {}, {"gw.example.com"}, {"http://:80"}, {"https://user:pw@gw.example.com"},
	} {
		_, err := selfHostedVhost(endpoints)
		require.ErrorIs(t, err, utils.ErrInvalidInput, "endpoints %v", endpoints)
	}
}

func registerRepo(created **models.Gateway, nameTaken bool) *repomocks.GatewayRepositoryMock {
	existing := &models.Gateway{UUID: uuid.New()}
	repo := mappingRepo(nil, 0)
	repo.GetByNameAndOrgIDFunc = func(string, string) (*models.Gateway, error) {
		if nameTaken {
			return existing, nil
		}
		return nil, utils.ErrGatewayNotFound
	}
	repo.CreateTxFunc = func(_ *gorm.DB, gw *models.Gateway) error {
		*created = gw
		return nil
	}
	return repo
}

func TestRegisterSelfHostedGateway(t *testing.T) {
	env := uuid.NewString()
	input := func(functionalityType string) SelfHostedGatewayInput {
		return SelfHostedGatewayInput{
			DisplayName: "Edge One", Environment: env, FunctionalityType: functionalityType,
			Endpoints: []string{"https://edge.example.com"}, Version: "2026.09.24",
		}
	}

	for _, functionalityType := range []string{"", "ai"} {
		t.Run("accepts "+functionalityType, func(t *testing.T) {
			var created *models.Gateway
			var mapped []string
			repo := registerRepo(&created, false)
			repo.CreateEnvironmentMappingTxFunc = func(_ *gorm.DB, m *models.GatewayEnvironmentMapping) error {
				mapped = append(mapped, m.EnvironmentUUID.String())
				return nil
			}
			svc := NewPlatformGatewayService(repo, nil)

			resp, err := svc.RegisterSelfHostedGateway("org", "dev", input(functionalityType))

			require.NoError(t, err)
			require.NotNil(t, created)
			require.Equal(t, models.GatewayHostingSelfHosted, created.Hosting)
			require.Equal(t, models.GatewayRoleEgress, created.GatewayFunctionalityType)
			require.Equal(t, "https://edge.example.com", created.Vhost)
			require.Equal(t, "dev-edge-one", created.Name)
			require.Equal(t, []string{env}, mapped)
			require.Equal(t, "SELF_HOSTED", string(resp.Hosting))
		})
	}

	t.Run("regular is refused", func(t *testing.T) {
		var created *models.Gateway
		svc := NewPlatformGatewayService(registerRepo(&created, false), nil)

		_, err := svc.RegisterSelfHostedGateway("org", "dev", input("regular"))

		require.ErrorIs(t, err, utils.ErrInvalidInput)
		require.Nil(t, created)
	})

	t.Run("a taken handle is a conflict", func(t *testing.T) {
		var created *models.Gateway
		svc := NewPlatformGatewayService(registerRepo(&created, true), nil)

		_, err := svc.RegisterSelfHostedGateway("org", "dev", input("ai"))

		require.ErrorIs(t, err, utils.ErrGatewayAlreadyExists)
		require.Nil(t, created)
	})
}

func TestListSelfHostedGatewaysFiltersByHostingAndOrg(t *testing.T) {
	var counted, listed repositories.GatewayFilterOptions
	repo := &repomocks.GatewayRepositoryMock{
		CountWithFiltersFunc: func(f repositories.GatewayFilterOptions) (int64, error) {
			counted = f
			return 0, nil
		},
		ListWithFiltersFunc: func(f repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			listed = f
			return nil, nil
		},
	}
	svc := NewPlatformGatewayService(repo, nil)

	_, err := svc.ListSelfHostedGateways("org-1", 10, 5)

	require.NoError(t, err)
	for _, f := range []repositories.GatewayFilterOptions{counted, listed} {
		require.Equal(t, models.GatewayHostingSelfHosted, f.Hosting)
		require.Equal(t, "org-1", f.OrganizationID)
	}
	require.Equal(t, 10, listed.Limit)
	require.Equal(t, 5, listed.Offset)
}

func storedGateway(hosting, ouID string) *models.Gateway {
	return &models.Gateway{UUID: uuid.New(), OUID: ouID, Name: "gw-one", Hosting: hosting}
}

func TestRequireSelfHosted(t *testing.T) {
	tests := []struct {
		name    string
		stored  *models.Gateway
		wantErr bool
	}{
		{name: "self-hosted", stored: storedGateway(models.GatewayHostingSelfHosted, "org")},
		{name: "managed", stored: storedGateway(models.GatewayHostingManaged, "org"), wantErr: true},
		{name: "other org", stored: storedGateway(models.GatewayHostingSelfHosted, "other"), wantErr: true},
		{name: "missing", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &repomocks.GatewayRepositoryMock{
				GetByUUIDFunc: func(string) (*models.Gateway, error) {
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
	gw := storedGateway(models.GatewayHostingSelfHosted, "org")
	repo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(string) (*models.Gateway, error) { return gw, nil },
		// TransactionFunc left nil: any repository mutation panics.
	}
	svc := NewPlatformGatewayService(repo, nil)

	err := svc.RemoveGatewayFromEnvironment(context.Background(), gw.UUID.String(), uuid.NewString())

	require.ErrorIs(t, err, utils.ErrGatewayEnvironmentFixed)
}

// panickingApplier fails the test by panicking on any call.
type panickingApplier struct{}

func (panickingApplier) ApplyIdentityProvider(context.Context, string, string, models.GatewayIdentityProvider) error {
	panic("applier called for a self-hosted gateway")
}

func (panickingApplier) DeleteIdentityProvider(context.Context, string, string, string) error {
	panic("applier called for a self-hosted gateway")
}

func TestIdentityProvider_SelfHostedIsNotFound(t *testing.T) {
	gw := storedGateway(models.GatewayHostingSelfHosted, "org")
	repo := &repomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(string, string) (*models.Gateway, error) { return gw, nil },
		// Every other func left nil: a repository mutation panics.
	}
	svc := NewPlatformGatewayService(repo, panickingApplier{})

	_, err := svc.UpsertIdentityProvider(context.Background(), gw.UUID.String(), "org", "idp", "https://issuer", "https://issuer/jwks", "", false)
	require.ErrorIs(t, err, utils.ErrGatewayNotFound)

	err = svc.DeleteIdentityProvider(context.Background(), gw.UUID.String(), "org", "idp")
	require.ErrorIs(t, err, utils.ErrGatewayNotFound)
}
