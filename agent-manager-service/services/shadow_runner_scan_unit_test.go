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
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const shadowScanTestToken = "eyJ.scan-token.SECRET"

// stubScanTokenIssuer is a ShadowScanTokenIssuer with a func field.
type stubScanTokenIssuer struct {
	IssueScanTokenFunc func(ctx context.Context, ouID string) (string, time.Time, error)
}

func (s *stubScanTokenIssuer) IssueScanToken(ctx context.Context, ouID string) (string, time.Time, error) {
	return s.IssueScanTokenFunc(ctx, ouID)
}

func shadowRunner(issuer ShadowScanTokenIssuer) ShadowScanRunner {
	return ShadowScanRunner{
		Enabled: true, Issuer: issuer, ClientID: "amp-api-client", AMPURL: "https://amp.example.com",
		Image: "ghcr.io/wso2/amp-shadow-runner:latest",
	}
}

func newRunnerShadowSvc(repo *repomocks.ShadowAgentRepositoryMock, runner ShadowScanRunner) ShadowAgentService {
	return NewShadowAgentServiceWithScanRunner(repo, ShadowConnectors{}, fakeAgentLookup{}, shadowTestKey, discardLogger(), runner)
}

// shadowRepoWith returns a repository holding one connection of provider.
func shadowRepoWith(conn models.ShadowConnection) *repomocks.ShadowAgentRepositoryMock {
	return &repomocks.ShadowAgentRepositoryMock{
		GetConnectionFunc: func(_ context.Context, ouID string, id uuid.UUID) (*models.ShadowConnection, error) {
			if ouID != shadowTestOU || id != conn.ID {
				return nil, gorm.ErrRecordNotFound
			}
			c := conn
			return &c, nil
		},
	}
}

func TestShadowCreateRunnerScan_CreatesPendingScanAndReturnsParts(t *testing.T) {
	conn := models.ShadowConnection{ID: uuid.New(), OUID: shadowTestOU, Name: "GitHub", Provider: models.ShadowProviderGitHub}
	repo := shadowRepoWith(conn)
	var created *models.ShadowScan
	repo.CreateScanFunc = func(_ context.Context, s *models.ShadowScan) error {
		created = s
		return nil
	}
	exp := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
	issuer := &stubScanTokenIssuer{IssueScanTokenFunc: func(_ context.Context, ouID string) (string, time.Time, error) {
		assert.Nil(t, created, "the token is obtained before the scan is created")
		assert.Equal(t, shadowTestOU, ouID)
		return shadowScanTestToken, exp, nil
	}}
	rec := &recordingAuditor{}
	ctx := audit.WithRecorder(context.Background(), rec)

	got, err := newRunnerShadowSvc(repo, shadowRunner(issuer)).CreateRunnerScan(ctx, shadowTestOU, conn.ID,
		models.ShadowScanOptions{AIReview: true, LLMJudge: true})
	require.NoError(t, err)

	require.NotNil(t, created)
	assert.Equal(t, models.ShadowScanPending, created.Status)
	assert.Equal(t, shadowTestOU, created.OUID)
	assert.Equal(t, []string{conn.ID.String()}, created.ConnectionIDs)
	assert.Equal(t, models.ShadowScanOptions{AIReview: true, LLMJudge: true}, created.Options)
	assert.Equal(t, created.ID, got.ScanID)
	assert.Equal(t, shadowScanTestToken, got.Token)
	assert.Equal(t, exp, got.ExpiresAt)
	assert.Equal(t, "https://amp.example.com", got.AMPURL)
	assert.Equal(t, "ghcr.io/wso2/amp-shadow-runner:latest", got.Image)

	require.Len(t, rec.events, 2, "intent and outcome")
	assert.Equal(t, audit.ActionShadowScanTokenIssue, rec.events[0].Action)
	assert.Equal(t, audit.OutcomeUnknown, rec.events[0].Outcome)
	assert.Equal(t, audit.OutcomeSuccess, rec.events[1].Outcome)
	for _, e := range rec.events {
		assert.NotContains(t, fmt.Sprint(e), shadowScanTestToken, "the token never reaches the audit trail")
	}
}

func TestShadowCreateRunnerScan_RefusesWithoutAuditTrail(t *testing.T) {
	conn := models.ShadowConnection{ID: uuid.New(), OUID: shadowTestOU, Provider: models.ShadowProviderGitHub}
	// CreateScanFunc and the issuer are nil: reaching either panics.
	svc := newRunnerShadowSvc(shadowRepoWith(conn), shadowRunner(&stubScanTokenIssuer{}))
	_, err := svc.CreateRunnerScan(context.Background(), shadowTestOU, conn.ID, models.ShadowScanOptions{})
	require.ErrorIs(t, err, audit.ErrRecorderUnavailable)
}

func TestShadowCreateRunnerScan_MintFailureLeavesNoScan(t *testing.T) {
	conn := models.ShadowConnection{ID: uuid.New(), OUID: shadowTestOU, Provider: models.ShadowProviderGitHub}
	// CreateScanFunc is nil: creating a scan after a failed mint panics.
	repo := shadowRepoWith(conn)
	idpDown := fmt.Errorf("%w: token request failed", utils.ErrServiceUnavailable)
	issuer := &stubScanTokenIssuer{IssueScanTokenFunc: func(context.Context, string) (string, time.Time, error) {
		return "", time.Time{}, idpDown
	}}
	rec := &recordingAuditor{}
	ctx := audit.WithRecorder(context.Background(), rec)

	got, err := newRunnerShadowSvc(repo, shadowRunner(issuer)).CreateRunnerScan(ctx, shadowTestOU, conn.ID,
		models.ShadowScanOptions{})
	require.Error(t, err)
	assert.Nil(t, got)
	assert.ErrorIs(t, err, utils.ErrServiceUnavailable)
	assert.Empty(t, repo.CreateScanCalls(), "a failed mint leaves no pending scan")
	require.Len(t, rec.events, 2)
	assert.Equal(t, audit.OutcomeFailure, rec.events[1].Outcome)
}

func TestShadowCreateRunnerScan_CreateScanFailureDropsTheToken(t *testing.T) {
	conn := models.ShadowConnection{ID: uuid.New(), OUID: shadowTestOU, Provider: models.ShadowProviderGitHub}
	repo := shadowRepoWith(conn)
	repo.CreateScanFunc = func(context.Context, *models.ShadowScan) error { return errors.New("db down") }
	issuer := &stubScanTokenIssuer{IssueScanTokenFunc: func(context.Context, string) (string, time.Time, error) {
		return shadowScanTestToken, time.Now().Add(time.Hour), nil
	}}
	rec := &recordingAuditor{}
	ctx := audit.WithRecorder(context.Background(), rec)

	got, err := newRunnerShadowSvc(repo, shadowRunner(issuer)).CreateRunnerScan(ctx, shadowTestOU, conn.ID,
		models.ShadowScanOptions{})
	require.Error(t, err)
	assert.Nil(t, got, "no token without a scan")
	assert.NotContains(t, err.Error(), shadowScanTestToken)
	require.Len(t, rec.events, 2)
	assert.Equal(t, audit.OutcomeFailure, rec.events[1].Outcome)
	for _, e := range rec.events {
		assert.NotContains(t, fmt.Sprint(e), shadowScanTestToken)
	}
}

func TestShadowCreateRunnerScan_RejectsInvalidRequests(t *testing.T) {
	cloud := models.ShadowConnection{ID: uuid.New(), OUID: shadowTestOU, Provider: models.ShadowProviderOpenAI}
	source := models.ShadowConnection{ID: uuid.New(), OUID: shadowTestOU, Provider: models.ShadowProviderGitHub}

	t.Run("judge without AI review", func(t *testing.T) {
		svc := newRunnerShadowSvc(shadowRepoWith(source), shadowRunner(&stubScanTokenIssuer{}))
		_, err := svc.CreateRunnerScan(auditableCtx(t), shadowTestOU, source.ID, models.ShadowScanOptions{LLMJudge: true})
		assert.ErrorIs(t, err, utils.ErrInvalidInput)
	})
	t.Run("cloud platform", func(t *testing.T) {
		svc := newRunnerShadowSvc(shadowRepoWith(cloud), shadowRunner(&stubScanTokenIssuer{}))
		_, err := svc.CreateRunnerScan(auditableCtx(t), shadowTestOU, cloud.ID, models.ShadowScanOptions{})
		assert.ErrorIs(t, err, utils.ErrInvalidInput)
	})
	t.Run("connection not found", func(t *testing.T) {
		svc := newRunnerShadowSvc(shadowRepoWith(source), shadowRunner(&stubScanTokenIssuer{}))
		_, err := svc.CreateRunnerScan(auditableCtx(t), shadowTestOU, uuid.New(), models.ShadowScanOptions{})
		assert.ErrorIs(t, err, utils.ErrShadowConnectionNotFound)
	})
	t.Run("repository failure is not not-found", func(t *testing.T) {
		repo := &repomocks.ShadowAgentRepositoryMock{
			GetConnectionFunc: func(context.Context, string, uuid.UUID) (*models.ShadowConnection, error) {
				return nil, errors.New("db down")
			},
		}
		_, err := newRunnerShadowSvc(repo, shadowRunner(&stubScanTokenIssuer{})).
			CreateRunnerScan(auditableCtx(t), shadowTestOU, source.ID, models.ShadowScanOptions{})
		require.Error(t, err)
		assert.NotErrorIs(t, err, utils.ErrShadowConnectionNotFound)
	})
}

func TestShadowCreateRunnerScan_NotSetUp(t *testing.T) {
	source := models.ShadowConnection{ID: uuid.New(), OUID: shadowTestOU, Provider: models.ShadowProviderGitHub}
	idp := config.IDPConfig{TokenURL: "https://idp.example.com/oauth2/token", ClientID: "amp-api-client", ClientSecret: "s"}
	on := config.ShadowRunnerConfig{Enabled: true}
	tests := []struct {
		name    string
		cfg     config.Config
		want    []string
		notWant []string
	}{
		{
			name:    "no SERVER_PUBLIC_URL",
			cfg:     config.Config{IDP: idp, ShadowRunner: on},
			want:    []string{"SERVER_PUBLIC_URL"},
			notWant: []string{"IDP_CLIENT_ID", "AMP_SHADOW_RUNNER_ENABLED"},
		},
		{
			name:    "no identity provider client",
			cfg:     config.Config{ServerPublicURL: "https://amp.example.com", ShadowRunner: on},
			want:    []string{"IDP_CLIENT_ID"},
			notWant: []string{"SERVER_PUBLIC_URL", "AMP_SHADOW_RUNNER_ENABLED"},
		},
		{
			name: "neither",
			cfg:  config.Config{ShadowRunner: on},
			want: []string{"IDP_CLIENT_ID", "SERVER_PUBLIC_URL"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner := NewShadowScanRunner(tc.cfg, discardLogger())
			_, err := newRunnerShadowSvc(shadowRepoWith(source), runner).
				CreateRunnerScan(auditableCtx(t), shadowTestOU, source.ID, models.ShadowScanOptions{})
			require.ErrorIs(t, err, utils.ErrShadowScanCommandUnavailable)
			for _, w := range tc.want {
				assert.Contains(t, err.Error(), w)
			}
			for _, w := range tc.notWant {
				assert.NotContains(t, err.Error(), w)
			}
		})
	}

	// NewShadowAgentService refuses scan commands as not set up, without
	// naming a setting it cannot know is missing.
	_, err := NewShadowAgentService(shadowRepoWith(source), ShadowConnectors{}, fakeAgentLookup{}, shadowTestKey, discardLogger()).
		CreateRunnerScan(auditableCtx(t), shadowTestOU, source.ID, models.ShadowScanOptions{})
	require.ErrorIs(t, err, utils.ErrShadowScanCommandUnavailable)
	assert.Equal(t, utils.ErrShadowScanCommandUnavailable.Error(), err.Error())
}

// While AMP_SHADOW_RUNNER_ENABLED is off, a fully set up server refuses scan
// commands as turned off and never calls the identity provider.
func TestShadowCreateRunnerScan_TurnedOff(t *testing.T) {
	source := models.ShadowConnection{ID: uuid.New(), OUID: shadowTestOU, Provider: models.ShadowProviderGitHub}
	cfg := config.Config{
		ServerPublicURL: "https://amp.example.com",
		IDP:             config.IDPConfig{TokenURL: "https://idp.example.com/oauth2/token", ClientID: "c", ClientSecret: "s"},
	}
	runner := NewShadowScanRunner(cfg, discardLogger())
	assert.False(t, runner.Enabled)
	assert.Nil(t, runner.Issuer, "no identity provider client is built while turned off")

	// Even with an issuer set, a turned-off runner never reaches it (its func
	// is nil, so a call panics), nor creates a scan (CreateScanFunc is nil).
	runner.Issuer = &stubScanTokenIssuer{}
	_, err := newRunnerShadowSvc(shadowRepoWith(source), runner).
		CreateRunnerScan(auditableCtx(t), shadowTestOU, source.ID, models.ShadowScanOptions{})
	require.ErrorIs(t, err, utils.ErrShadowScanCommandUnavailable)
	assert.Contains(t, err.Error(), "not turned on")
	assert.Contains(t, err.Error(), "AMP_SHADOW_RUNNER_ENABLED")
}

func TestNewShadowScanRunner(t *testing.T) {
	cfg := config.Config{ServerPublicURL: "https://amp.example.com/"}
	cfg.ShadowRunner.Enabled = true
	cfg.ShadowRunner.Image = "img"

	runner := NewShadowScanRunner(cfg, discardLogger())
	assert.True(t, runner.Enabled)
	assert.Nil(t, runner.Issuer, "no identity provider client, no issuer")
	assert.Equal(t, "https://amp.example.com", runner.AMPURL)
	assert.Equal(t, "img", runner.Image)

	cfg.IDP.TokenURL = "https://idp.example.com/oauth2/token"
	cfg.IDP.ClientID = "amp-api-client"
	cfg.IDP.ClientSecret = "secret"
	cfg.IDP.OrgTokenEndpointEnabled = true
	runner = NewShadowScanRunner(cfg, discardLogger())
	require.NotNil(t, runner.Issuer)
	assert.True(t, runner.Issuer.(*shadowScanTokenIssuer).minter.orgTokenEndpoint)
	assert.Equal(t, "amp-api-client", runner.ClientID)
	assert.Empty(t, runner.unavailable)
}

// shadowScanIssuer is the real issuer against a fake identity provider.
func shadowScanIssuer(t *testing.T, idp *btIdP, resource string) ShadowScanTokenIssuer {
	t.Helper()
	srv := idp.server(t)
	return &shadowScanTokenIssuer{
		minter: newOrgClientTokenMinter(srv.URL+"/oauth2/token", btClientID, btClientSecret, resource, true, srv.Client()),
	}
}

func shadowScanToken(t *testing.T, expiresIn time.Duration) string {
	t.Helper()
	claims := btClaims(btOrg, btClientID, expiresIn)
	claims["scope"] = ocShadowPublishScope
	return btToken(t, claims)
}

func TestShadowScanTokenIssuer_IssuesAPublishTokenForTheOrganization(t *testing.T) {
	token := shadowScanToken(t, 55*time.Minute)
	idp := &btIdP{status: http.StatusOK, body: `{"access_token":"` + token + `"}`}
	issuer := shadowScanIssuer(t, idp, "")

	got, expiresAt, err := issuer.IssueScanToken(context.Background(), btOrg)
	require.NoError(t, err)
	assert.Equal(t, token, got)
	assert.WithinDuration(t, time.Now().Add(55*time.Minute), expiresAt, time.Minute)
	assert.Equal(t, "/ou/"+btOrg+"/oauth2/token", idp.path, "the organization's token endpoint")
	assert.Equal(t, []string{ocShadowPublishScope}, idp.form["scope"], "the publish scope only")
	assert.NotContains(t, idp.form, "resource", "no resource is requested")
}

func TestShadowScanTokenIssuer_RefusesAShortLivedToken(t *testing.T) {
	token := shadowScanToken(t, 40*time.Minute)
	idp := &btIdP{status: http.StatusOK, body: `{"access_token":"` + token + `"}`}

	got, _, err := shadowScanIssuer(t, idp, "").IssueScanToken(context.Background(), btOrg)
	require.Error(t, err)
	assert.Empty(t, got)
	assert.Contains(t, err.Error(), "less than")
	assert.NotContains(t, err.Error(), token)
}

func TestShadowScanTokenIssuer_RequestsTheConfiguredResource(t *testing.T) {
	idp := &btIdP{status: http.StatusOK, body: `{"access_token":"` + shadowScanToken(t, time.Hour) + `"}`}

	_, _, err := shadowScanIssuer(t, idp, "urn:wso2:cloud").IssueScanToken(context.Background(), btOrg)
	require.NoError(t, err)
	assert.Equal(t, []string{"urn:wso2:cloud"}, idp.form["resource"])
}
