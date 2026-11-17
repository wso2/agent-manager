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
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/requests"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	// shadowScanTokenRequestTimeout bounds the identity provider call,
	// including its retries, as for the build token.
	shadowScanTokenRequestTimeout = 10 * time.Second
	// shadowScanTokenMinValidity is the least lifetime a scan token may have
	// left when it is issued. The identity provider issues one-hour tokens, so
	// asking for the full hour would refuse every token.
	shadowScanTokenMinValidity = 50 * time.Minute
)

// ShadowRunnerScan is a pending source code scan and the parts of the scan
// command that runs it. The token is handed out once and never stored.
type ShadowRunnerScan struct {
	ScanID    uuid.UUID
	Token     string
	ExpiresAt time.Time
	AMPURL    string
	Image     string
}

// ShadowScanTokenIssuer obtains the token a source code scan command
// publishes its results with: an access token for the organization carrying
// the amp:shadow-agent:publish scope and nothing else.
type ShadowScanTokenIssuer interface {
	IssueScanToken(ctx context.Context, ouID string) (token string, expiresAt time.Time, err error)
}

// ShadowScanRunner is what this deployment gives a source code scan command.
// A zero value refuses every scan command as not set up.
type ShadowScanRunner struct {
	// Issuer is nil when the identity provider client (IDP_TOKEN_URL,
	// IDP_CLIENT_ID, IDP_CLIENT_SECRET) is not set up.
	Issuer ShadowScanTokenIssuer
	// ClientID is the client the scan tokens are issued to, for the audit
	// trail.
	ClientID string
	// AMPURL is SERVER_PUBLIC_URL without a trailing slash; empty when unset.
	AMPURL string
	// Image is the scan command's container image.
	Image string
	// unavailable says what to set when Issuer or AMPURL is missing. Empty on
	// a zero value, which is refused as not set up.
	unavailable string
}

// NewShadowScanRunner builds the scan command settings from the config. A
// missing SERVER_PUBLIC_URL or identity provider client does not stop the
// service: creating a scan command is then refused with a message saying
// what to set.
func NewShadowScanRunner(cfg config.Config, logger *slog.Logger) ShadowScanRunner {
	runner := ShadowScanRunner{
		Issuer:      nil,
		ClientID:    strings.TrimSpace(cfg.IDP.ClientID),
		AMPURL:      strings.TrimRight(strings.TrimSpace(cfg.ServerPublicURL), "/"),
		Image:       cfg.ShadowRunner.Image,
		unavailable: "",
	}
	missing := []string{}
	_, urlErr := config.OrganizationTokenURL(cfg.IDP.TokenURL, "ou")
	if urlErr != nil || runner.ClientID == "" || cfg.IDP.ClientSecret == "" {
		missing = append(missing, "a usable IDP_TOKEN_URL, IDP_CLIENT_ID and IDP_CLIENT_SECRET")
	} else {
		runner.Issuer = &shadowScanTokenIssuer{
			minter: newOrgClientTokenMinter(cfg.IDP.TokenURL, cfg.IDP.ClientID, cfg.IDP.ClientSecret,
				cfg.ShadowRunner.Resource, requests.NewRetryableHTTPClient(&http.Client{})),
		}
	}
	if runner.AMPURL == "" {
		missing = append(missing, "SERVER_PUBLIC_URL")
	}
	if len(missing) > 0 {
		runner.unavailable = "they need " + strings.Join(missing, " and ") + " to be set"
		logger.Info("Source code scan commands are off", "reason", runner.unavailable)
	}
	return runner
}

type shadowScanTokenIssuer struct {
	minter *orgClientTokenMinter
}

func (i *shadowScanTokenIssuer) IssueScanToken(ctx context.Context, ouID string) (string, time.Time, error) {
	ctx, cancel := context.WithTimeout(ctx, shadowScanTokenRequestTimeout)
	defer cancel()
	return i.minter.mint(ctx, ouID, rbac.ShadowAgentPublish.Scope(), shadowScanTokenMinValidity)
}

// NewShadowAgentServiceWithScanRunner creates a ShadowAgentService that can
// create source code scan commands. NewShadowAgentService creates one that
// refuses them as not set up.
func NewShadowAgentServiceWithScanRunner(
	repo repositories.ShadowAgentRepository, connectors ShadowConnectors, encryptionKey []byte, logger *slog.Logger,
	runner ShadowScanRunner,
) ShadowAgentService {
	return &shadowAgentService{
		repo: repo, connectors: connectors, encryptionKey: encryptionKey, logger: logger, now: time.Now,
		runner: runner,
	}
}

func (s *shadowAgentService) CreateRunnerScan(
	ctx context.Context, ouID string, id uuid.UUID, opts models.ShadowScanOptions,
) (*ShadowRunnerScan, error) {
	if opts.LLMJudge && !opts.AIReview {
		return nil, fmt.Errorf("%w: the LLM judge needs AI review", utils.ErrInvalidInput)
	}
	c, err := s.loadConnection(ctx, ouID, id)
	if err != nil {
		return nil, err
	}
	if spec, ok := sa.Provider(c.Provider); !ok || !spec.SourceHost {
		return nil, fmt.Errorf("%w: only source code repository connections (GitHub, Azure DevOps) have a scan command",
			utils.ErrInvalidInput)
	}
	if s.runner.Issuer == nil || s.runner.AMPURL == "" {
		err := utils.ErrShadowScanCommandUnavailable
		if s.runner.unavailable != "" {
			err = fmt.Errorf("%w: %s", utils.ErrShadowScanCommandUnavailable, s.runner.unavailable)
		}
		s.logger.Warn("Refused a source code scan command", "ouID", ouID, "reason", err.Error())
		return nil, err
	}

	// Handing out a live token for the organization: record the intent first
	// and refuse to proceed if the trail is unavailable.
	scanID := uuid.New()
	attempt, err := audit.Begin(ctx, audit.ActionShadowScanTokenIssue,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceShadowConnection, c.ID.String(), c.Name),
		audit.Detail("provider", c.Provider),
		audit.Detail("scanId", scanID.String()),
		audit.Detail("clientId", s.runner.ClientID),
		audit.Detail("aiReview", opts.AIReview),
		audit.Detail("llmJudge", opts.LLMJudge),
	)
	if err != nil {
		return nil, err
	}
	out, err := s.startRunnerScan(ctx, ouID, c.ID, scanID, opts)
	attempt.Complete(ctx, err)
	return out, err
}

// startRunnerScan obtains the token, then creates the pending scan. A token
// that could not be issued leaves no scan behind, and a token whose scan could
// not be created is dropped unseen, so a token is never handed out without a
// scan to publish to.
func (s *shadowAgentService) startRunnerScan(
	ctx context.Context, ouID string, connectionID, scanID uuid.UUID, opts models.ShadowScanOptions,
) (*ShadowRunnerScan, error) {
	token, expiresAt, err := s.runner.Issuer.IssueScanToken(ctx, ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to issue the scan token: %w", err)
	}
	scan := &models.ShadowScan{
		ID:            scanID,
		OUID:          ouID,
		Status:        models.ShadowScanPending,
		ConnectionIDs: []string{connectionID.String()},
		Options:       opts,
		Results:       []models.ShadowScanConnectionResult{},
		RequestedBy:   audit.BuildEvent(ctx, audit.ActionShadowScanTokenIssue).ActorID,
	}
	if err := s.repo.CreateScan(ctx, scan); err != nil {
		return nil, fmt.Errorf("failed to create the scan: %w", err)
	}
	return &ShadowRunnerScan{
		ScanID:    scanID,
		Token:     token,
		ExpiresAt: expiresAt,
		AMPURL:    s.runner.AMPURL,
		Image:     s.runner.Image,
	}, nil
}
