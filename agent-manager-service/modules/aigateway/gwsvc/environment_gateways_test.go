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
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

func TestEnvironmentService_GetEnvironmentGateways(t *testing.T) {
	const org, envID = "acme", "dev"
	const envUUID = "33333333-3333-3333-3333-333333333333"

	t.Run("maps env not-found to ErrEnvironmentNotFound", func(t *testing.T) {
		oc := &gwtest.Core{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*modkit.Environment, error) {
				return nil, gwerr.ErrEnvironmentNotFound
			},
		}
		svc := newEnvGatewayService(&gwrepomocks.GatewayRepositoryMock{}, oc)

		_, err := svc.GetEnvironmentGateways(context.Background(), org, envID)

		assert.ErrorIs(t, err, gwerr.ErrEnvironmentNotFound)
	})

	t.Run("wraps a mapping-lookup error", func(t *testing.T) {
		boom := errors.New("mapping query failed")
		oc := &gwtest.Core{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*modkit.Environment, error) {
				return &modkit.Environment{ID: envUUID, Name: "dev"}, nil
			},
		}
		repo := &gwrepomocks.GatewayRepositoryMock{
			GetEnvironmentMappingsByEnvironmentIDFunc: func(_ string) ([]gwmodels.GatewayEnvironmentMapping, error) {
				return nil, boom
			},
		}
		svc := newEnvGatewayService(repo, oc)

		_, err := svc.GetEnvironmentGateways(context.Background(), org, envID)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("aggregates gateways and skips missing/errored ones", func(t *testing.T) {
		activeGW := uuid.New()
		errorGW := uuid.New()
		missingGW := uuid.New()

		oc := &gwtest.Core{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*modkit.Environment, error) {
				return &modkit.Environment{ID: envUUID, Name: "dev"}, nil
			},
		}
		repo := &gwrepomocks.GatewayRepositoryMock{
			GetEnvironmentMappingsByEnvironmentIDFunc: func(_ string) ([]gwmodels.GatewayEnvironmentMapping, error) {
				return []gwmodels.GatewayEnvironmentMapping{
					{GatewayUUID: activeGW},
					{GatewayUUID: errorGW},
					{GatewayUUID: missingGW},
				}, nil
			},
			GetByUUIDFunc: func(id string) (*gwmodels.Gateway, error) {
				switch id {
				case activeGW.String():
					return &gwmodels.Gateway{
						UUID:                     activeGW,
						Name:                     "gw-active",
						DisplayName:              "Active GW",
						GatewayFunctionalityType: "egress",
						Vhost:                    "gw.example.com",
						IsCritical:               true,
						IsActive:                 true,
					}, nil
				case errorGW.String():
					return nil, errors.New("transient") // skipped, not fatal
				case missingGW.String():
					//nolint:nilnil // intentionally exercising the (nil, nil) "missing, skip" input the service must handle
					return nil, nil // skipped
				default:
					return nil, errors.New("unexpected gateway id")
				}
			},
		}
		svc := newEnvGatewayService(repo, oc)

		resp, err := svc.GetEnvironmentGateways(context.Background(), org, envID)

		require.NoError(t, err)
		require.Len(t, resp, 1) // only the active gateway survives
		assert.Equal(t, "gw-active", resp[0].Name)
		assert.Equal(t, activeGW.String(), resp[0].UUID)
		assert.Equal(t, org, resp[0].OrganizationName)
		assert.True(t, resp[0].IsCritical)
		assert.Equal(t, string(gwmodels.GatewayStatusActive), resp[0].Status)
	})

	t.Run("maps an inactive gateway to inactive status", func(t *testing.T) {
		gw := uuid.New()
		oc := &gwtest.Core{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*modkit.Environment, error) {
				return &modkit.Environment{ID: envUUID, Name: "dev"}, nil
			},
		}
		repo := &gwrepomocks.GatewayRepositoryMock{
			GetEnvironmentMappingsByEnvironmentIDFunc: func(_ string) ([]gwmodels.GatewayEnvironmentMapping, error) {
				return []gwmodels.GatewayEnvironmentMapping{{GatewayUUID: gw}}, nil
			},
			GetByUUIDFunc: func(_ string) (*gwmodels.Gateway, error) {
				return &gwmodels.Gateway{UUID: gw, Name: "gw-idle", IsActive: false}, nil
			},
		}
		svc := newEnvGatewayService(repo, oc)

		resp, err := svc.GetEnvironmentGateways(context.Background(), org, envID)

		require.NoError(t, err)
		require.Len(t, resp, 1)
		assert.Equal(t, string(gwmodels.GatewayStatusInactive), resp[0].Status)
	})
}

func newEnvGatewayService(repo *gwrepomocks.GatewayRepositoryMock, oc *gwtest.Core) *EnvironmentGatewayService {
	return NewEnvironmentGatewayService(discardLogger(), repo, oc)
}
