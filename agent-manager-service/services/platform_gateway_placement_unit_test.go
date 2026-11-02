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
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type placedEnv struct {
	ouID string
	env  uuid.UUID
}

type recordingPlacer chan placedEnv

func (p recordingPlacer) RequestPlaceEnv(ouID string, env uuid.UUID) {
	p <- placedEnv{ouID: ouID, env: env}
}

// placed waits for the placements a mapping triggers after its transaction commits.
func (p recordingPlacer) placed(t *testing.T, n int) []placedEnv {
	t.Helper()
	var got []placedEnv
	for range n {
		select {
		case placed := <-p:
			got = append(got, placed)
		case <-time.After(5 * time.Second):
			t.Fatalf("got %d placements, want %d", len(got), n)
		}
	}
	return got
}

func mappingRepo(gw *models.Gateway, ingressInEnv int64) *repomocks.GatewayRepositoryMock {
	return &repomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc:                 func(string, string) (*models.Gateway, error) { return nil, utils.ErrGatewayNotFound },
		GetByUUIDFunc:                         func(string) (*models.Gateway, error) { return gw, nil },
		TransactionFunc:                       func(fn func(tx *gorm.DB) error) error { return fn(nil) },
		CreateTxFunc:                          func(*gorm.DB, *models.Gateway) error { return nil },
		AcquireGatewayLockFunc:                func(*gorm.DB, string) error { return nil },
		AcquireEnvironmentLockFunc:            func(*gorm.DB, string) error { return nil },
		EnvironmentMappingExistsFunc:          func(string, string) (bool, error) { return false, nil },
		GetEnvironmentMappingsByGatewayIDFunc: func(string) ([]models.GatewayEnvironmentMapping, error) { return nil, nil },
		CountIngressCapableInEnvironmentFunc:  func(*gorm.DB, string) (int64, error) { return ingressInEnv, nil },
		CreateEnvironmentMappingTxFunc:        func(*gorm.DB, *models.GatewayEnvironmentMapping) error { return nil },
	}
}

func TestAssignGatewayToEnvironmentPlacesTheEnvironment(t *testing.T) {
	gw := &models.Gateway{UUID: uuid.New(), OUID: "ou-1", GatewayFunctionalityType: models.GatewayRoleBoth}
	env := uuid.New()
	placer := make(recordingPlacer, 1)
	svc := NewPlatformGatewayService(mappingRepo(gw, 0), nil).WithEnvironmentPlacer(placer)

	require.NoError(t, svc.AssignGatewayToEnvironment(gw.UUID.String(), env.String()))

	assert.Equal(t, []placedEnv{{ouID: "ou-1", env: env}}, placer.placed(t, 1))
}

func TestAssignGatewayToEnvironmentPlacesNothingWhenTheMappingFails(t *testing.T) {
	gw := &models.Gateway{UUID: uuid.New(), OUID: "ou-1", GatewayFunctionalityType: models.GatewayRoleBoth}
	placer := make(recordingPlacer, 1)
	svc := NewPlatformGatewayService(mappingRepo(gw, 1), nil).WithEnvironmentPlacer(placer)

	require.ErrorIs(t, svc.AssignGatewayToEnvironment(gw.UUID.String(), uuid.NewString()), utils.ErrGatewayIngressCapExceeded)

	select {
	case placed := <-placer:
		t.Fatalf("placed %v after a rejected mapping", placed)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestRegisterGatewayPlacesItsEnvironment(t *testing.T) {
	gw := &models.Gateway{UUID: uuid.New(), GatewayFunctionalityType: models.GatewayRoleEgress}
	env := uuid.New()
	placer := make(recordingPlacer, 1)
	svc := NewPlatformGatewayService(mappingRepo(gw, 0), nil).WithEnvironmentPlacer(placer)

	_, err := svc.RegisterGateway("ou-1", "gw1", "GW", "", "https://ext.example.com", "", false, "EGRESS", nil,
		[]string{env.String(), env.String()}, models.GatewayHostingManaged)
	require.NoError(t, err)

	assert.Equal(t, []placedEnv{{ouID: "ou-1", env: env}}, placer.placed(t, 1))
}
