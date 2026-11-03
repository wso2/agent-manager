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
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func mappedTo(env uuid.UUID, gw *models.Gateway) []models.GatewayEnvironmentMapping {
	return []models.GatewayEnvironmentMapping{{GatewayUUID: gw.UUID, EnvironmentUUID: env}}
}

func TestAssignGatewaySecondEnvironmentRefused(t *testing.T) {
	for _, role := range []string{models.GatewayRoleBoth, models.GatewayRoleEgress} {
		t.Run(string(role), func(t *testing.T) {
			gw := &models.Gateway{UUID: uuid.New(), GatewayFunctionalityType: role}
			repo := mappingRepo(gw, 0)
			repo.GetEnvironmentMappingsByGatewayIDFunc = func(string) ([]models.GatewayEnvironmentMapping, error) {
				return mappedTo(uuid.New(), gw), nil
			}
			repo.CreateEnvironmentMappingTxFunc = func(*gorm.DB, *models.GatewayEnvironmentMapping) error {
				t.Fatal("a gateway already mapped to another environment must not get a second mapping")
				return nil
			}
			svc := NewPlatformGatewayService(repo, nil)

			err := svc.AssignGatewayToEnvironment(gw.UUID.String(), uuid.NewString())

			require.ErrorIs(t, err, utils.ErrGatewayEnvironmentFixed)
		})
	}
}

func TestAssignGatewaySameEnvironmentIdempotent(t *testing.T) {
	env := uuid.New()
	gw := &models.Gateway{UUID: uuid.New(), GatewayFunctionalityType: models.GatewayRoleBoth}
	repo := mappingRepo(gw, 0)
	repo.EnvironmentMappingExistsFunc = func(string, string) (bool, error) { return true, nil }
	repo.GetEnvironmentMappingsByGatewayIDFunc = func(string) ([]models.GatewayEnvironmentMapping, error) {
		return mappedTo(env, gw), nil
	}
	svc := NewPlatformGatewayService(repo, nil)

	require.NoError(t, svc.AssignGatewayToEnvironment(gw.UUID.String(), env.String()))
}

func TestAssignGatewayLocksTheGatewayBeforeReadingItsMappings(t *testing.T) {
	gw := &models.Gateway{UUID: uuid.New(), GatewayFunctionalityType: models.GatewayRoleEgress}
	repo := mappingRepo(gw, 0)
	locked := false
	repo.AcquireGatewayLockFunc = func(_ *gorm.DB, gatewayID string) error {
		require.Equal(t, gw.UUID.String(), gatewayID)
		locked = true
		return nil
	}
	repo.GetEnvironmentMappingsByGatewayIDFunc = func(string) ([]models.GatewayEnvironmentMapping, error) {
		require.True(t, locked, "mappings read before the gateway lock was held")
		return nil, nil
	}
	svc := NewPlatformGatewayService(repo, nil)

	require.NoError(t, svc.AssignGatewayToEnvironment(gw.UUID.String(), uuid.NewString()))
	require.True(t, locked)
}

func TestRegisterGatewayWithTwoEnvironmentsRefused(t *testing.T) {
	gw := &models.Gateway{UUID: uuid.New(), GatewayFunctionalityType: models.GatewayRoleEgress}
	repo := mappingRepo(gw, 0)
	repo.CreateTxFunc = func(*gorm.DB, *models.Gateway) error {
		t.Fatal("no gateway row may be written for a two-environment registration")
		return nil
	}
	svc := NewPlatformGatewayService(repo, nil)

	_, err := svc.RegisterGateway("ou-1", "gw1", "GW", "", "https://ext.example.com", "", false, "EGRESS", nil,
		[]string{uuid.NewString(), uuid.NewString()})

	require.ErrorIs(t, err, utils.ErrGatewayEnvironmentFixed)
}
