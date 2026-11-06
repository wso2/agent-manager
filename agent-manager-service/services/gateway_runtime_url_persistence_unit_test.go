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
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const testRuntimeURL = "http://api-platform-acme-dev-gw-gateway-gateway-runtime.acme-dev:22893"

// The stored value is what sandboxed agent pods are handed alongside the gateway API key,
// so these tests pin that the write paths persist exactly what validation approved.

func TestRegisterGateway_PersistsTrimmedRuntimeURL(t *testing.T) {
	var created *models.Gateway
	repo := &repomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(string, string) (*models.Gateway, error) {
			return nil, utils.ErrGatewayNotFound
		},
		TransactionFunc: func(fn func(tx *gorm.DB) error) error { return fn(nil) },
		CreateTxFunc: func(_ *gorm.DB, gw *models.Gateway) error {
			created = gw
			return nil
		},
	}
	svc := NewPlatformGatewayService(repo, nil)

	resp, err := svc.RegisterGateway("org", "gw1", "GW", "", "https://ext.example.com",
		"  "+testRuntimeURL+"  ", false, "BOTH", nil, nil, models.GatewayHostingManaged)

	require.NoError(t, err)
	require.NotNil(t, created)
	require.Equal(t, testRuntimeURL, created.RuntimeURL)
	require.Equal(t, testRuntimeURL, resp.RuntimeURL)
}

func TestRegisterGateway_InvalidRuntimeURLRejectedBeforeAnyWrite(t *testing.T) {
	// Every mock func left nil: reaching the repository at all panics the test.
	repo := &repomocks.GatewayRepositoryMock{}
	svc := NewPlatformGatewayService(repo, nil)

	_, err := svc.RegisterGateway("org", "gw1", "GW", "", "https://ext.example.com",
		"http://exfil.dev:53", false, "BOTH", nil, nil, models.GatewayHostingManaged)

	require.ErrorIs(t, err, utils.ErrBadRequest)
	require.Empty(t, repo.GetByNameAndOrgIDCalls())
	require.Empty(t, repo.CreateTxCalls())
}

// updateRuntimeURLRepo returns a mock holding one gateway with the given stored address,
// its ID, and a pointer to whatever gateway the service hands back to UpdateGateway.
func updateRuntimeURLRepo(
	t *testing.T, stored string,
) (repo *repomocks.GatewayRepositoryMock, gatewayID string, updated **models.Gateway) {
	t.Helper()
	gw := &models.Gateway{UUID: uuid.New(), OUID: "org", RuntimeURL: stored}
	updated = new(*models.Gateway)
	repo = &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(string) (*models.Gateway, error) { return gw, nil },
		UpdateGatewayFunc: func(g *models.Gateway) error {
			*updated = g
			return nil
		},
	}
	return repo, gw.UUID.String(), updated
}

func TestUpdateGateway_RuntimeURLOmittedLeavesStoredValue(t *testing.T) {
	repo, id, updated := updateRuntimeURLRepo(t, testRuntimeURL)
	svc := NewPlatformGatewayService(repo, nil)

	resp, err := svc.UpdateGateway(id, "org", nil, nil, nil, nil, nil)

	require.NoError(t, err)
	require.Equal(t, testRuntimeURL, (*updated).RuntimeURL)
	require.Equal(t, testRuntimeURL, resp.RuntimeURL)
}

func TestUpdateGateway_ValidRuntimeURLIsStoredTrimmed(t *testing.T) {
	repo, id, updated := updateRuntimeURLRepo(t, "")
	svc := NewPlatformGatewayService(repo, nil)
	next := "  http://runtime.acme-dev.svc:22893  "

	resp, err := svc.UpdateGateway(id, "org", nil, nil, nil, nil, &next)

	require.NoError(t, err)
	require.Equal(t, "http://runtime.acme-dev.svc:22893", (*updated).RuntimeURL)
	require.Equal(t, "http://runtime.acme-dev.svc:22893", resp.RuntimeURL)
}

// A whitespace-only value is legal and clears the address; this is the only test that
// exercises the TrimSpace guard on the update path.
func TestUpdateGateway_WhitespaceOnlyRuntimeURLClearsStoredValue(t *testing.T) {
	repo, id, updated := updateRuntimeURLRepo(t, testRuntimeURL)
	svc := NewPlatformGatewayService(repo, nil)
	blank := "   "

	resp, err := svc.UpdateGateway(id, "org", nil, nil, nil, nil, &blank)

	require.NoError(t, err)
	require.Empty(t, (*updated).RuntimeURL)
	require.Empty(t, resp.RuntimeURL)
}

func TestUpdateGateway_InvalidRuntimeURLRejectedBeforeWrite(t *testing.T) {
	repo, id, updated := updateRuntimeURLRepo(t, testRuntimeURL)
	svc := NewPlatformGatewayService(repo, nil)
	bad := "http://exfil.dev:53"

	_, err := svc.UpdateGateway(id, "org", nil, nil, nil, nil, &bad)

	require.ErrorIs(t, err, utils.ErrBadRequest)
	require.Nil(t, *updated)
	require.Empty(t, repo.UpdateGatewayCalls())
}
