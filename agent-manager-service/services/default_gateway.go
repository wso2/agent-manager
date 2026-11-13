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
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/gatewaytoken"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// DefaultGatewaySeeder registers the gateway named in configuration, with the registration token the
// gateway is configured with, so a bundled gateway connects without a manual registration step. It is
// safe to run on every start: an existing gateway is kept, and the token is added only when missing.
type DefaultGatewaySeeder struct {
	cfg      config.DefaultGatewayConfig
	gateways *PlatformGatewayService
	repo     repositories.GatewayRepository
	platform repositories.PlatformRepository
	logger   *slog.Logger
}

// NewDefaultGatewaySeeder builds the seeder.
func NewDefaultGatewaySeeder(
	cfg config.DefaultGatewayConfig, gateways *PlatformGatewayService, repo repositories.GatewayRepository,
	platform repositories.PlatformRepository, logger *slog.Logger,
) *DefaultGatewaySeeder {
	return &DefaultGatewaySeeder{cfg: cfg, gateways: gateways, repo: repo, platform: platform, logger: logger}
}

// Seed registers the default gateway and its token. It does nothing when no default gateway is configured.
func (s *DefaultGatewaySeeder) Seed(ctx context.Context) error {
	if s.cfg.Name == "" {
		return nil
	}
	token, err := s.token()
	if err != nil {
		return err
	}
	prefix, err := gatewaytoken.Prefix(token)
	if err != nil {
		return fmt.Errorf("default gateway token: %w", err)
	}

	ouID := s.cfg.OrgOUID
	// Creates the organization, under its handle, and its default environment on first use.
	ctx = orgctx.WithResolvedOrg(ctx, orgctx.ResolvedOrg{OUID: ouID, OuHandle: s.cfg.OrgHandle})
	env, err := s.platform.GetEnvironment(ctx, ouID, models.DefaultEnvironmentName)
	if err != nil {
		return fmt.Errorf("default gateway: resolve the default environment: %w", err)
	}

	gateway, err := s.repo.GetByNameAndOrgID(s.cfg.Name, ouID)
	switch {
	case errors.Is(err, utils.ErrGatewayNotFound) || (err == nil && gateway == nil):
		created, regErr := s.gateways.RegisterGateway(ouID, s.cfg.Name, s.cfg.DisplayName, "", s.cfg.Vhost, "",
			false, s.cfg.FunctionalityType, nil, []string{env.UUID}, models.GatewayHostingManaged)
		if regErr != nil {
			return fmt.Errorf("default gateway: register %q: %w", s.cfg.Name, regErr)
		}
		s.logger.Info("Registered the default gateway", "name", s.cfg.Name, "gatewayID", created.ID, "ouID", ouID)
		if gateway, err = s.repo.GetByNameAndOrgID(s.cfg.Name, ouID); err != nil {
			return fmt.Errorf("default gateway: read back %q: %w", s.cfg.Name, err)
		}
	case err != nil:
		return fmt.Errorf("default gateway: look up %q: %w", s.cfg.Name, err)
	case gateway.Vhost != s.cfg.Vhost:
		s.logger.Warn("The default gateway is registered with another vhost; keeping the registered one",
			"name", s.cfg.Name, "registered", gateway.Vhost, "configured", s.cfg.Vhost)
	}

	return s.ensureToken(ctx, gateway, token, prefix)
}

// token is the configured token, or the content of the configured token file.
func (s *DefaultGatewaySeeder) token() (string, error) {
	if s.cfg.Token != "" {
		return s.cfg.Token, nil
	}
	raw, err := os.ReadFile(s.cfg.TokenFile)
	if err != nil {
		return "", fmt.Errorf("default gateway token: %w", err)
	}
	return strings.TrimSpace(string(raw)), nil
}

// ensureToken stores the token's hash for the gateway unless it is already an active token of it.
func (s *DefaultGatewaySeeder) ensureToken(ctx context.Context, gateway *models.Gateway, token, prefix string) error {
	existing, err := s.repo.GetActiveTokenByPrefix(ctx, prefix)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("default gateway: look up its token: %w", err)
	}
	if err == nil && existing != nil {
		if existing.GatewayUUID != gateway.UUID || !verifyToken(token, existing.TokenHash, existing.Salt) {
			return errors.New("default gateway: its token's prefix belongs to another token; configure a new token")
		}
		return nil
	}
	salt, err := generateSalt()
	if err != nil {
		return err
	}
	if err := s.repo.CreateToken(&models.GatewayToken{
		UUID: uuid.New(), GatewayUUID: gateway.UUID, TokenPrefix: prefix, TokenHash: hashToken(token, salt),
		Salt: hex.EncodeToString(salt), Status: "active",
	}); err != nil {
		return fmt.Errorf("default gateway: store its token: %w", err)
	}
	s.gateways.InvalidateGatewayTokensCache(gateway.UUID)
	s.logger.Info("Stored the default gateway's registration token", "name", s.cfg.Name, "gatewayID", gateway.UUID)
	return nil
}
