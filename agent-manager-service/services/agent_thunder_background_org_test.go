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
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// orgRecordingReader is a project pipeline reader that records the organization each read ran
// with, the way the OpenChoreo client sees it, and then fails so nothing further is attempted.
type orgRecordingReader struct {
	mu    sync.Mutex
	reads []recordedPipelineRead
}

type recordedPipelineRead struct {
	organizationID string
	org            orgctx.ResolvedOrg
	hasOrg         bool
}

func (r *orgRecordingReader) ProjectPipeline(ctx context.Context, organizationID, _ string) (string, []ProjectIdentityEnvironment, error) {
	org, ok := orgctx.GetResolvedOrg(ctx)
	r.mu.Lock()
	r.reads = append(r.reads, recordedPipelineRead{organizationID: organizationID, org: org, hasOrg: ok})
	r.mu.Unlock()
	return "", nil, errors.New("pipeline is not read in this test")
}

func newOrgTestService(t *testing.T, repo *repomocks.AgentThunderClientRepositoryMock, reader ProjectPipelineReader) AgentThunderProvisioningService {
	t.Helper()
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
			return "secret-reference", nil
		},
	}
	return newPromotionService(t, repo, store, (&fakeTargetPlatform{}).provider(t), reader)
}

func attemptRepo() *repomocks.AgentThunderClientRepositoryMock {
	return &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc:    func(context.Context, uuid.UUID) (bool, error) { return true, nil },
		UpdateAfterAttemptFunc: func(context.Context, uuid.UUID, repositories.AgentThunderAttemptUpdate) error { return nil },
		ReleaseClaimFunc:       func(context.Context, uuid.UUID, time.Time, string) error { return nil },
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
			return nil, repositories.ErrAgentThunderClientNotFound
		},
	}
}

// The reconciler runs retries from a bare background context: no token and no organization. The
// OpenChoreo client takes the organization for X-Impersonate-Org from the context, and a platform
// that calls OpenChoreo with a machine token not scoped to one organization refuses the pipeline read
// (HTTP 400) without it. So an attempt must name the binding's organization itself.
func TestAttemptProvisionNamesTheBindingsOrganizationForThePipelineRead(t *testing.T) {
	t.Run("a source attempt run by the reconciler", func(t *testing.T) {
		reader := &orgRecordingReader{}
		svc := newOrgTestService(t, attemptRepo(), reader)
		binding := models.AgentThunderClient{
			ID: uuid.New(), OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a", EnvironmentName: "x",
			ProvisioningType: models.AgentProvisioningTypeInternal,
		}

		svc.AttemptProvision(context.Background(), binding) // the reconciler's context: nothing on it

		require.Len(t, reader.reads, 1)
		require.True(t, reader.reads[0].hasOrg, "the pipeline read must carry an organization")
		require.Equal(t, "org-a", reader.reads[0].org.OUID)
		require.Equal(t, "org-a", reader.reads[0].organizationID)
	})

	t.Run("a promoted attempt run by the reconciler", func(t *testing.T) {
		reader := &orgRecordingReader{}
		svc := newOrgTestService(t, attemptRepo(), reader)
		binding := pendingA(models.AgentProvisioningTypeInternal)
		binding.ID = uuid.New()

		svc.AttemptProvision(context.Background(), binding)

		require.NotEmpty(t, reader.reads)
		for _, read := range reader.reads {
			require.True(t, read.hasOrg)
			require.Equal(t, "org-a", read.org.OUID)
		}
	})

	t.Run("each binding gets its own organization", func(t *testing.T) {
		reader := &orgRecordingReader{}
		svc := newOrgTestService(t, attemptRepo(), reader)
		for _, ouID := range []string{"org-a", "org-b"} {
			svc.AttemptProvision(context.Background(), models.AgentThunderClient{
				ID: uuid.New(), OUID: ouID, ProjectName: "project-a", AgentName: "agent-a", EnvironmentName: "x",
				ProvisioningType: models.AgentProvisioningTypeInternal,
			})
		}
		require.Len(t, reader.reads, 2)
		require.Equal(t, "org-a", reader.reads[0].org.OUID)
		require.Equal(t, "org-b", reader.reads[1].org.OUID)
	})

	t.Run("a request that already names the organization keeps its handle", func(t *testing.T) {
		reader := &orgRecordingReader{}
		svc := newOrgTestService(t, attemptRepo(), reader)
		ctx := orgctx.WithResolvedOrg(context.Background(), orgctx.ResolvedOrg{OUID: "org-a", OuHandle: "acme"})

		svc.AttemptProvision(ctx, models.AgentThunderClient{
			ID: uuid.New(), OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a", EnvironmentName: "x",
			ProvisioningType: models.AgentProvisioningTypeInternal,
		})

		require.Len(t, reader.reads, 1)
		require.Equal(t, orgctx.ResolvedOrg{OUID: "org-a", OuHandle: "acme"}, reader.reads[0].org)
	})
}

// The topology pass also runs outside any request, so it must read each project's pipeline in that
// project's organization.
func TestPromotionTopologyPassNamesEachProjectsOrganization(t *testing.T) {
	reader := &orgRecordingReader{}
	repo := attemptRepo()
	repo.ListHomeAgentsFunc = func(_ context.Context, after *repositories.AgentRef, _ int) ([]repositories.AgentRef, error) {
		if after != nil {
			return nil, nil
		}
		return []repositories.AgentRef{
			{OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a", EnvironmentName: "x", ProvisioningType: models.AgentProvisioningTypeInternal},
			{OUID: "org-b", ProjectName: "project-b", AgentName: "agent-b", EnvironmentName: "x", ProvisioningType: models.AgentProvisioningTypeInternal},
			{OUID: "org-a", ProjectName: "project-a", AgentName: "agent-c", EnvironmentName: "x", ProvisioningType: models.AgentProvisioningTypeInternal},
		}, nil
	}
	svc := newOrgTestService(t, repo, reader)

	svc.(ProjectAgentIdentityPromotionReconciler).ReconcileProjectAgentIdentityPromotions(context.Background())

	require.Len(t, reader.reads, 2, "one read per project, shared by its agents")
	for _, read := range reader.reads {
		require.True(t, read.hasOrg)
		require.Equal(t, read.organizationID, read.org.OUID, "the project's own organization")
	}
}

func TestWithOrgContext(t *testing.T) {
	bare := context.Background()
	named := withOrgContext(bare, "org-a")
	org, ok := orgctx.GetResolvedOrg(named)
	require.True(t, ok)
	require.Equal(t, "org-a", org.OUID)

	withHandle := orgctx.WithResolvedOrg(bare, orgctx.ResolvedOrg{OUID: "org-a", OuHandle: "acme"})
	require.Equal(t, withHandle, withOrgContext(withHandle, "org-a"), "the same organization leaves the context as it is")

	switched := withOrgContext(withHandle, "org-b")
	org, _ = orgctx.GetResolvedOrg(switched)
	require.Equal(t, orgctx.ResolvedOrg{OUID: "org-b"}, org, "another organization is named, never mixed with the old handle")
}
