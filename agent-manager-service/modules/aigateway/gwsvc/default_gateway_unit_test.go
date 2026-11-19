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
	"encoding/hex"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gatewaytoken"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwconfig"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
)

const seedOUID = "01900000-0000-7000-8000-000000000001"

func seedConfig(t *testing.T) (gwconfig.DefaultGateway, string) {
	t.Helper()
	token, err := gatewaytoken.New()
	require.NoError(t, err)
	return gwconfig.DefaultGateway{
		Name: "default", DisplayName: "Default AI Gateway", Vhost: "http://gw.localhost:19080",
		FunctionalityType: "BOTH", OrgOUID: seedOUID, Token: token,
	}, token
}

func seedPlatform(t *testing.T) *gwtest.Core {
	t.Helper()
	return &gwtest.Core{
		GetEnvironmentFunc: func(_ context.Context, ouID, envName string) (*modkit.Environment, error) {
			assert.Equal(t, seedOUID, ouID)
			assert.Equal(t, gwmodels.DefaultEnvironmentName, envName)
			return &modkit.Environment{ID: uuid.NewString(), Name: envName}, nil
		},
	}
}

func TestDefaultGatewaySeederDisabledDoesNothing(t *testing.T) {
	// Every mock func is nil: any repository call panics.
	repo := &gwrepomocks.GatewayRepositoryMock{}
	s := NewDefaultGatewaySeeder(gwconfig.DefaultGateway{}, NewPlatformGatewayService(repo), repo,
		&gwtest.Core{}, discardLogger())
	require.NoError(t, s.Seed(context.Background()))
}

func TestDefaultGatewaySeederStoresTokenForExistingGateway(t *testing.T) {
	cfg, token := seedConfig(t)
	gw := &gwmodels.Gateway{UUID: uuid.New(), OUID: seedOUID, Name: "default", Vhost: cfg.Vhost}
	var stored *gwmodels.GatewayToken
	repo := &gwrepomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(name, ouID string) (*gwmodels.Gateway, error) { return gw, nil },
		GetActiveTokenByPrefixFunc: func(ctx context.Context, prefix string) (*gwmodels.GatewayToken, error) {
			return nil, gorm.ErrRecordNotFound
		},
		CreateTokenFunc: func(tok *gwmodels.GatewayToken) error { stored = tok; return nil },
	}
	s := NewDefaultGatewaySeeder(cfg, NewPlatformGatewayService(repo), repo, seedPlatform(t), discardLogger())
	require.NoError(t, s.Seed(context.Background()))

	require.NotNil(t, stored)
	prefix, _ := gatewaytoken.Prefix(token)
	assert.Equal(t, prefix, stored.TokenPrefix)
	assert.Equal(t, gw.UUID, stored.GatewayUUID)
	assert.Equal(t, "active", stored.Status)
	assert.True(t, verifyToken(token, stored.TokenHash, stored.Salt), "the stored hash must verify the token")
	assert.NotContains(t, stored.TokenHash, token)
}

func TestDefaultGatewaySeederKeepsMatchingToken(t *testing.T) {
	cfg, token := seedConfig(t)
	gw := &gwmodels.Gateway{UUID: uuid.New(), OUID: seedOUID, Name: "default", Vhost: cfg.Vhost}
	salt, _ := generateSalt()
	existing := &gwmodels.GatewayToken{GatewayUUID: gw.UUID, TokenHash: hashToken(token, salt), Salt: hexOf(salt)}
	repo := &gwrepomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(name, ouID string) (*gwmodels.Gateway, error) { return gw, nil },
		GetActiveTokenByPrefixFunc: func(ctx context.Context, prefix string) (*gwmodels.GatewayToken, error) {
			return existing, nil
		},
		// CreateTokenFunc left nil: storing the token again would panic.
	}
	s := NewDefaultGatewaySeeder(cfg, NewPlatformGatewayService(repo), repo, seedPlatform(t), discardLogger())
	require.NoError(t, s.Seed(context.Background()))
}

func TestDefaultGatewaySeederRefusesTokenOfAnotherGateway(t *testing.T) {
	cfg, token := seedConfig(t)
	gw := &gwmodels.Gateway{UUID: uuid.New(), OUID: seedOUID, Name: "default", Vhost: cfg.Vhost}
	salt, _ := generateSalt()
	other := &gwmodels.GatewayToken{GatewayUUID: uuid.New(), TokenHash: hashToken(token, salt), Salt: hexOf(salt)}
	repo := &gwrepomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(name, ouID string) (*gwmodels.Gateway, error) { return gw, nil },
		GetActiveTokenByPrefixFunc: func(ctx context.Context, prefix string) (*gwmodels.GatewayToken, error) {
			return other, nil
		},
	}
	s := NewDefaultGatewaySeeder(cfg, NewPlatformGatewayService(repo), repo, seedPlatform(t), discardLogger())
	require.ErrorContains(t, s.Seed(context.Background()), "belongs to another token")
}

func TestDefaultGatewaySeederRegistersMissingGateway(t *testing.T) {
	cfg, _ := seedConfig(t)
	registered := &gwmodels.Gateway{UUID: uuid.New(), OUID: seedOUID, Name: "default", Vhost: cfg.Vhost}
	lookups := 0
	transactions := 0
	repo := &gwrepomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(name, ouID string) (*gwmodels.Gateway, error) {
			lookups++
			// The seeder's lookup and RegisterGateway's uniqueness check find nothing; the read-back finds it.
			if lookups <= 2 {
				return nil, gwerr.ErrGatewayNotFound
			}
			return registered, nil
		},
		TransactionFunc: func(fn func(tx *gorm.DB) error) error { transactions++; return nil },
		GetActiveTokenByPrefixFunc: func(ctx context.Context, prefix string) (*gwmodels.GatewayToken, error) {
			return nil, gorm.ErrRecordNotFound
		},
		CreateTokenFunc: func(tok *gwmodels.GatewayToken) error {
			assert.Equal(t, registered.UUID, tok.GatewayUUID)
			return nil
		},
	}
	s := NewDefaultGatewaySeeder(cfg, NewPlatformGatewayService(repo), repo, seedPlatform(t), discardLogger())
	require.NoError(t, s.Seed(context.Background()))
	assert.Equal(t, 1, transactions, "the gateway is registered once")
	assert.Len(t, repo.CreateTokenCalls(), 1)
}

func TestDefaultGatewaySeederRejectsMalformedToken(t *testing.T) {
	cfg, _ := seedConfig(t)
	cfg.Token = "not-a-token"
	repo := &gwrepomocks.GatewayRepositoryMock{}
	s := NewDefaultGatewaySeeder(cfg, NewPlatformGatewayService(repo), repo, &gwtest.Core{}, discardLogger())
	require.ErrorContains(t, s.Seed(context.Background()), "default gateway token")
}

func hexOf(b []byte) string { return hex.EncodeToString(b) }
