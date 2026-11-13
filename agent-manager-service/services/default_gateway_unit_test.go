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
	"encoding/hex"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/gatewaytoken"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const seedOUID = "01900000-0000-7000-8000-000000000001"

func seedConfig(t *testing.T) (config.DefaultGatewayConfig, string) {
	t.Helper()
	token, err := gatewaytoken.New()
	require.NoError(t, err)
	return config.DefaultGatewayConfig{
		Name: "default", DisplayName: "Default AI Gateway", Vhost: "http://gw.localhost:19080",
		FunctionalityType: "BOTH", OrgOUID: seedOUID, OrgHandle: "default", Token: token,
	}, token
}

func seedPlatform(t *testing.T) *repomocks.PlatformRepositoryMock {
	t.Helper()
	return &repomocks.PlatformRepositoryMock{
		GetEnvironmentFunc: func(ctx context.Context, ouID, envName string) (*models.EnvironmentResponse, error) {
			resolved, ok := orgctx.GetResolvedOrg(ctx)
			assert.True(t, ok, "the organization must be created under its handle")
			assert.Equal(t, "default", resolved.OuHandle)
			assert.Equal(t, models.DefaultEnvironmentName, envName)
			return &models.EnvironmentResponse{UUID: uuid.NewString(), Name: envName}, nil
		},
	}
}

func TestDefaultGatewaySeederDisabledDoesNothing(t *testing.T) {
	// Every mock func is nil: any repository call panics.
	repo := &repomocks.GatewayRepositoryMock{}
	s := NewDefaultGatewaySeeder(config.DefaultGatewayConfig{}, NewPlatformGatewayService(repo, nil), repo,
		&repomocks.PlatformRepositoryMock{}, discardLogger())
	require.NoError(t, s.Seed(context.Background()))
}

func TestDefaultGatewaySeederStoresTokenForExistingGateway(t *testing.T) {
	cfg, token := seedConfig(t)
	gw := &models.Gateway{UUID: uuid.New(), OUID: seedOUID, Name: "default", Vhost: cfg.Vhost}
	var stored *models.GatewayToken
	repo := &repomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(name, ouID string) (*models.Gateway, error) { return gw, nil },
		GetActiveTokenByPrefixFunc: func(ctx context.Context, prefix string) (*models.GatewayToken, error) {
			return nil, gorm.ErrRecordNotFound
		},
		CreateTokenFunc: func(tok *models.GatewayToken) error { stored = tok; return nil },
	}
	s := NewDefaultGatewaySeeder(cfg, NewPlatformGatewayService(repo, nil), repo, seedPlatform(t), discardLogger())
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
	gw := &models.Gateway{UUID: uuid.New(), OUID: seedOUID, Name: "default", Vhost: cfg.Vhost}
	salt, _ := generateSalt()
	existing := &models.GatewayToken{GatewayUUID: gw.UUID, TokenHash: hashToken(token, salt), Salt: hexOf(salt)}
	repo := &repomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(name, ouID string) (*models.Gateway, error) { return gw, nil },
		GetActiveTokenByPrefixFunc: func(ctx context.Context, prefix string) (*models.GatewayToken, error) {
			return existing, nil
		},
		// CreateTokenFunc left nil: storing the token again would panic.
	}
	s := NewDefaultGatewaySeeder(cfg, NewPlatformGatewayService(repo, nil), repo, seedPlatform(t), discardLogger())
	require.NoError(t, s.Seed(context.Background()))
}

func TestDefaultGatewaySeederRefusesTokenOfAnotherGateway(t *testing.T) {
	cfg, token := seedConfig(t)
	gw := &models.Gateway{UUID: uuid.New(), OUID: seedOUID, Name: "default", Vhost: cfg.Vhost}
	salt, _ := generateSalt()
	other := &models.GatewayToken{GatewayUUID: uuid.New(), TokenHash: hashToken(token, salt), Salt: hexOf(salt)}
	repo := &repomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(name, ouID string) (*models.Gateway, error) { return gw, nil },
		GetActiveTokenByPrefixFunc: func(ctx context.Context, prefix string) (*models.GatewayToken, error) {
			return other, nil
		},
	}
	s := NewDefaultGatewaySeeder(cfg, NewPlatformGatewayService(repo, nil), repo, seedPlatform(t), discardLogger())
	require.ErrorContains(t, s.Seed(context.Background()), "belongs to another token")
}

func TestDefaultGatewaySeederRegistersMissingGateway(t *testing.T) {
	cfg, _ := seedConfig(t)
	registered := &models.Gateway{UUID: uuid.New(), OUID: seedOUID, Name: "default", Vhost: cfg.Vhost}
	lookups := 0
	transactions := 0
	repo := &repomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(name, ouID string) (*models.Gateway, error) {
			lookups++
			// The seeder's lookup and RegisterGateway's uniqueness check find nothing; the read-back finds it.
			if lookups <= 2 {
				return nil, utils.ErrGatewayNotFound
			}
			return registered, nil
		},
		TransactionFunc: func(fn func(tx *gorm.DB) error) error { transactions++; return nil },
		GetActiveTokenByPrefixFunc: func(ctx context.Context, prefix string) (*models.GatewayToken, error) {
			return nil, gorm.ErrRecordNotFound
		},
		CreateTokenFunc: func(tok *models.GatewayToken) error {
			assert.Equal(t, registered.UUID, tok.GatewayUUID)
			return nil
		},
	}
	s := NewDefaultGatewaySeeder(cfg, NewPlatformGatewayService(repo, nil), repo, seedPlatform(t), discardLogger())
	require.NoError(t, s.Seed(context.Background()))
	assert.Equal(t, 1, transactions, "the gateway is registered once")
	assert.Len(t, repo.CreateTokenCalls(), 1)
}

func TestDefaultGatewaySeederRejectsMalformedToken(t *testing.T) {
	cfg, _ := seedConfig(t)
	cfg.Token = "not-a-token"
	repo := &repomocks.GatewayRepositoryMock{}
	s := NewDefaultGatewaySeeder(cfg, NewPlatformGatewayService(repo, nil), repo, &repomocks.PlatformRepositoryMock{}, discardLogger())
	require.ErrorContains(t, s.Seed(context.Background()), "default gateway token")
}

func hexOf(b []byte) string { return hex.EncodeToString(b) }
