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
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

func TestNormalizeGatewayRole(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      string
		wantErr   bool
		wantErrIs error
	}{
		{name: "canonical ingress", input: "INGRESS", want: gwmodels.GatewayRoleIngress},
		{name: "canonical egress", input: "EGRESS", want: gwmodels.GatewayRoleEgress},
		{name: "canonical both", input: "BOTH", want: gwmodels.GatewayRoleBoth},
		{name: "lowercase accepted", input: "both", want: gwmodels.GatewayRoleBoth},
		{name: "alias REGULAR maps to both", input: "REGULAR", want: gwmodels.GatewayRoleBoth},
		{name: "alias AI maps to egress", input: "AI", want: gwmodels.GatewayRoleEgress},
		{name: "event is rejected", input: "EVENT", wantErr: true, wantErrIs: httpx.ErrBadRequest},
		{name: "empty is rejected", input: "", wantErr: true, wantErrIs: httpx.ErrBadRequest},
		{name: "unknown is rejected", input: "sideways", wantErr: true, wantErrIs: httpx.ErrBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeGatewayRole(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrIs != nil {
					require.ErrorIs(t, err, tt.wantErrIs)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// A second ingress-capable gateway in one environment is rejected.
func TestAssignGatewayToEnvironment_SecondIngressRejected(t *testing.T) {
	envID := uuid.New().String()
	gw := &gwmodels.Gateway{UUID: uuid.New(), GatewayFunctionalityType: gwmodels.GatewayRoleBoth}
	repo := &gwrepomocks.GatewayRepositoryMock{
		GetByUUIDFunc:                         func(string) (*gwmodels.Gateway, error) { return gw, nil },
		TransactionFunc:                       func(fn func(tx *gorm.DB) error) error { return fn(nil) },
		AcquireGatewayLockFunc:                func(*gorm.DB, string) error { return nil },
		GetEnvironmentMappingsByGatewayIDFunc: func(string) ([]gwmodels.GatewayEnvironmentMapping, error) { return nil, nil },
		AcquireEnvironmentLockFunc:            func(*gorm.DB, string) error { return nil },
		EnvironmentMappingExistsFunc:          func(string, string) (bool, error) { return false, nil },
		CountIngressCapableInEnvironmentFunc:  func(*gorm.DB, string) (int64, error) { return 1, nil },
	}
	svc := NewPlatformGatewayService(repo, nil)

	err := svc.AssignGatewayToEnvironment(gw.UUID.String(), envID)

	require.ErrorIs(t, err, gwerr.ErrGatewayIngressCapExceeded)
}

// A second EGRESS gateway is allowed. This is the supported both+egress shape and the
// only way an existing environment gains egress separation — asserting it is permitted
// matters as much as asserting the ingress rejection.
func TestAssignGatewayToEnvironment_SecondEgressAllowed(t *testing.T) {
	envID := uuid.New().String()
	gw := &gwmodels.Gateway{UUID: uuid.New(), GatewayFunctionalityType: gwmodels.GatewayRoleEgress}
	created := false
	repo := &gwrepomocks.GatewayRepositoryMock{
		GetByUUIDFunc:                         func(string) (*gwmodels.Gateway, error) { return gw, nil },
		TransactionFunc:                       func(fn func(tx *gorm.DB) error) error { return fn(nil) },
		AcquireGatewayLockFunc:                func(*gorm.DB, string) error { return nil },
		GetEnvironmentMappingsByGatewayIDFunc: func(string) ([]gwmodels.GatewayEnvironmentMapping, error) { return nil, nil },
		AcquireEnvironmentLockFunc:            func(*gorm.DB, string) error { return nil },
		EnvironmentMappingExistsFunc:          func(string, string) (bool, error) { return false, nil },
		CountIngressCapableInEnvironmentFunc: func(*gorm.DB, string) (int64, error) {
			t.Fatal("egress gateways must not be counted against the ingress cap")
			return 0, nil
		},
		CreateEnvironmentMappingTxFunc: func(*gorm.DB, *gwmodels.GatewayEnvironmentMapping) error {
			created = true
			return nil
		},
	}
	svc := NewPlatformGatewayService(repo, nil)

	require.NoError(t, svc.AssignGatewayToEnvironment(gw.UUID.String(), envID))
	require.True(t, created)
}

// Re-assigning an already-mapped gateway stays idempotent and never counts itself.
func TestAssignGatewayToEnvironment_IdempotentWhenAlreadyMapped(t *testing.T) {
	envID := uuid.New().String()
	gw := &gwmodels.Gateway{UUID: uuid.New(), GatewayFunctionalityType: gwmodels.GatewayRoleBoth}
	lockAcquired := false
	existsChecked := false
	repo := &gwrepomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(string) (*gwmodels.Gateway, error) { return gw, nil },
		TransactionFunc: func(fn func(tx *gorm.DB) error) error {
			return fn(nil)
		},
		AcquireGatewayLockFunc: func(*gorm.DB, string) error { return nil },
		AcquireEnvironmentLockFunc: func(*gorm.DB, string) error {
			lockAcquired = true
			return nil
		},
		EnvironmentMappingExistsFunc: func(string, string) (bool, error) {
			existsChecked = true
			// The lock must be held before the existence check runs.
			require.True(t, lockAcquired, "expected environment lock to be acquired before the existence check")
			return true, nil
		},
		CountIngressCapableInEnvironmentFunc: func(*gorm.DB, string) (int64, error) {
			t.Fatal("an already-mapped gateway must not be counted against the ingress cap")
			return 0, nil
		},
		CreateEnvironmentMappingTxFunc: func(*gorm.DB, *gwmodels.GatewayEnvironmentMapping) error {
			t.Fatal("an already-mapped gateway must not be re-inserted")
			return nil
		},
	}
	svc := NewPlatformGatewayService(repo, nil)

	require.NoError(t, svc.AssignGatewayToEnvironment(gw.UUID.String(), envID))
	require.True(t, existsChecked, "expected the existence check to run inside the transaction")
}

// A cap rejection during registration rolls the gateway row back — no orphan gateway.
func TestRegisterGateway_CapRejectionRollsBackGatewayRow(t *testing.T) {
	var createdInTx bool
	repo := &gwrepomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(string, string) (*gwmodels.Gateway, error) {
			return nil, gwerr.ErrGatewayNotFound
		},
		TransactionFunc: func(fn func(tx *gorm.DB) error) error {
			// Real gorm rolls back on error; emulate by discarding the side effect.
			if err := fn(nil); err != nil {
				createdInTx = false
				return err
			}
			return nil
		},
		CreateTxFunc: func(*gorm.DB, *gwmodels.Gateway) error { createdInTx = true; return nil },
		GetByUUIDFunc: func(string) (*gwmodels.Gateway, error) {
			return &gwmodels.Gateway{UUID: uuid.New(), GatewayFunctionalityType: gwmodels.GatewayRoleBoth}, nil
		},
		AcquireGatewayLockFunc:                func(*gorm.DB, string) error { return nil },
		GetEnvironmentMappingsByGatewayIDFunc: func(string) ([]gwmodels.GatewayEnvironmentMapping, error) { return nil, nil },
		AcquireEnvironmentLockFunc:            func(*gorm.DB, string) error { return nil },
		EnvironmentMappingExistsFunc:          func(string, string) (bool, error) { return false, nil },
		CountIngressCapableInEnvironmentFunc:  func(*gorm.DB, string) (int64, error) { return 1, nil },
	}
	svc := NewPlatformGatewayService(repo, nil)

	_, err := svc.RegisterGateway("org", "gw1", "GW", "", "http://x", "", false, "BOTH", nil,
		[]string{uuid.New().String()}, gwmodels.GatewayHostingManaged)

	require.ErrorIs(t, err, gwerr.ErrGatewayIngressCapExceeded)
	require.False(t, createdInTx, "gateway row must not survive a cap rejection")
}
