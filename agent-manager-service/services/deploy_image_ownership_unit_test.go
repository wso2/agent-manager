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

// DeployAgent takes a client-supplied imageId and writes it onto the Workload.
// These pin that only an image the agent owns — one of its builds, a version of
// its kind, or what it already runs — gets that far, so a caller cannot deploy
// another tenant's image or an arbitrary registry reference.
package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"gorm.io/gorm"
)

// buildsProducing stubs ListBuilds with one build per image, so a deploy test can
// say which images the agent owns.
func buildsProducing(images ...string) func(context.Context, string, string, string) ([]*models.BuildResponse, error) {
	return func(context.Context, string, string, string) ([]*models.BuildResponse, error) {
		builds := make([]*models.BuildResponse, 0, len(images))
		for _, img := range images {
			builds = append(builds, &models.BuildResponse{ImageId: img})
		}
		return builds, nil
	}
}

const (
	ownBuildImage    = "registry.example.com/acme/my-agent:build-1"
	deployedImage    = "registry.example.com/acme/my-agent:build-0"
	kindVersionImage = "registry.example.com/acme/kind-src:v2"
)

// ownershipFixture is a deploy that stops at the first write after the check —
// EnsureProjectReleaseBinding — so each case only has to say whether it got there.
type ownershipFixture struct {
	oc             *clientmocks.OpenChoreoClientMock
	svc            *agentManagerService
	bindingReached *bool
}

func newOwnershipFixture(kindName string) ownershipFixture {
	reached := false
	stop := errors.New("stop after ownership check")
	oc := &clientmocks.OpenChoreoClientMock{
		// The deploy/promote pre-flight reads the component's reconcile conditions;
		// an unblocked component keeps this test on the path it actually covers.
		GetComponentReconcileBlockFunc: func(_ context.Context, _, _ string) (*client.ComponentReconcileBlock, error) {
			return nil, nil //nolint:nilnil // documented contract: a nil block means "not blocked"
		},
		GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: name}, nil
		},
		GetComponentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
			return &models.AgentResponse{
				KindName:     kindName,
				Provisioning: models.Provisioning{Type: string(utils.InternalAgent)},
			}, nil
		},
		GetProjectDeploymentPipelineFunc: func(context.Context, string, string) (*models.DeploymentPipelineResponse, error) {
			return threeEnvPipeline(), nil
		},
		GetEnvironmentFunc: nonProductionEnvStub(),
		ListBuildsFunc:     buildsProducing(ownBuildImage),
		GetDeploymentsFunc: func(context.Context, string, string, string, string) ([]*models.DeploymentResponse, error) {
			return []*models.DeploymentResponse{{Environment: "dev", ImageId: deployedImage}}, nil
		},
		EnsureProjectReleaseBindingFunc: func(context.Context, string, string, string) error {
			reached = true
			return stop
		},
		DeployFunc: func(context.Context, string, string, string, client.DeployRequest) error {
			return nil
		},
	}
	kindRepo := &repomocks.AgentKindRepositoryMock{
		GetKindFunc: func(_ context.Context, _, name string) (*models.AgentKind, error) {
			k := &models.AgentKind{ID: uuid.New(), Name: name}
			k.Versions = []models.AgentKindVersion{
				{Version: "1.0.0", ImageId: "registry.example.com/acme/kind-src:v1", Kind: k},
				{Version: "2.0.0", ImageId: kindVersionImage, Kind: k},
			}
			return k, nil
		},
	}
	svc := &agentManagerService{ocClient: oc, agentKindService: newKindService(kindRepo, oc), logger: discardLogger()}
	return ownershipFixture{oc: oc, svc: svc, bindingReached: &reached}
}

func (f ownershipFixture) deploy(t *testing.T, imageID string) error {
	_, err := f.svc.DeployAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.DeployAgentRequest{ImageId: imageID})
	return err
}

func TestDeployAgent_AcceptsImagesTheAgentOwns(t *testing.T) {
	tests := []struct {
		name     string
		kindName string
		image    string
	}{
		{"an image one of its builds produced", "", ownBuildImage},
		{"the image it already runs", "", deployedImage},
		{"a version of its kind other than the one it was created from", "my-kind", kindVersionImage},
		// A kind agent's image need not be a kind version: the check must fall
		// through to its builds and deployments rather than stop at the kind.
		{"a kind agent redeploying the image it already runs", "my-kind", deployedImage},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newOwnershipFixture(tc.kindName)
			err := f.deploy(t, tc.image)
			require.Error(t, err)
			assert.NotErrorIs(t, err, utils.ErrInvalidInput)
			assert.True(t, *f.bindingReached, "an owned image must get past the ownership check")
		})
	}
}

func TestDeployAgent_RejectsImagesTheAgentDoesNotOwn(t *testing.T) {
	tests := []struct {
		name     string
		kindName string
		image    string
	}{
		{"another agent's or tenant's build", "", "registry.example.com/other-org/their-agent:build-7"},
		{"an arbitrary public image", "", "docker.io/library/nginx:latest"},
		{"the same repository at a different tag", "", "registry.example.com/acme/my-agent:build-1-evil"},
		{"a kind image when the agent has no kind", "", kindVersionImage},
		{"a foreign image on a kind agent", "my-kind", "docker.io/library/nginx:latest"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newOwnershipFixture(tc.kindName)
			err := f.deploy(t, tc.image)
			require.ErrorIs(t, err, utils.ErrInvalidInput)
			require.NotNil(t, utils.IsValidationError(err), "must surface as a 400, not a 500")
			assert.False(t, *f.bindingReached, "a rejected image must not reach any write")
			assert.Empty(t, f.oc.DeployCalls())
		})
	}
}

// A check that passes whenever OpenChoreo is unreachable is not a check.
func TestDeployAgent_OwnershipLookupFailureRejects(t *testing.T) {
	boom := errors.New("openchoreo unavailable")

	t.Run("builds", func(t *testing.T) {
		f := newOwnershipFixture("")
		f.oc.ListBuildsFunc = func(context.Context, string, string, string) ([]*models.BuildResponse, error) {
			return nil, boom
		}
		err := f.deploy(t, ownBuildImage)
		require.ErrorIs(t, err, boom)
		assert.Nil(t, utils.IsValidationError(err), "a failed lookup is not the caller's fault")
		assert.False(t, *f.bindingReached)
		assert.Empty(t, f.oc.DeployCalls())
	})

	t.Run("kind", func(t *testing.T) {
		f := newOwnershipFixture("my-kind")
		f.svc.agentKindService = newKindService(&repomocks.AgentKindRepositoryMock{
			GetKindFunc: func(context.Context, string, string) (*models.AgentKind, error) {
				return nil, boom
			},
		}, f.oc)
		err := f.deploy(t, kindVersionImage)
		require.ErrorIs(t, err, boom)
		assert.Nil(t, utils.IsValidationError(err), "a failed lookup is not the caller's fault")
		assert.False(t, *f.bindingReached)
	})

	// The builds and deployments can still prove ownership without the kind.
	t.Run("kind, but a build owns the image", func(t *testing.T) {
		f := newOwnershipFixture("my-kind")
		f.svc.agentKindService = newKindService(&repomocks.AgentKindRepositoryMock{
			GetKindFunc: func(context.Context, string, string) (*models.AgentKind, error) {
				return nil, boom
			},
		}, f.oc)
		err := f.deploy(t, ownBuildImage)
		require.Error(t, err)
		assert.NotErrorIs(t, err, boom)
		assert.True(t, *f.bindingReached)
	})

	// A failed build listing is held like a failed kind lookup, so a redeploy of
	// what the agent already runs survives a builds outage.
	t.Run("builds, but a deployment owns the image", func(t *testing.T) {
		f := newOwnershipFixture("")
		f.oc.ListBuildsFunc = func(context.Context, string, string, string) ([]*models.BuildResponse, error) {
			return nil, boom
		}
		err := f.deploy(t, deployedImage)
		require.Error(t, err)
		assert.NotErrorIs(t, err, boom)
		assert.True(t, *f.bindingReached)
	})

	t.Run("deployments", func(t *testing.T) {
		f := newOwnershipFixture("")
		f.oc.GetDeploymentsFunc = func(context.Context, string, string, string, string) ([]*models.DeploymentResponse, error) {
			return nil, boom
		}
		err := f.deploy(t, deployedImage)
		require.ErrorIs(t, err, boom)
		assert.Nil(t, utils.IsValidationError(err), "a failed lookup is not the caller's fault")
		assert.False(t, *f.bindingReached)
		assert.Empty(t, f.oc.DeployCalls())
	})
}

// A kind row that no longer exists has no versions to offer. It must not turn
// every deploy of the agent into a 404 — the agent's own builds and deployments
// still decide.
func TestDeployAgent_MissingKindFallsThroughToBuildsAndDeployments(t *testing.T) {
	f := newOwnershipFixture("deleted-kind")
	f.svc.agentKindService = newKindService(&repomocks.AgentKindRepositoryMock{
		GetKindFunc: func(context.Context, string, string) (*models.AgentKind, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}, f.oc)

	t.Run("its current image is still accepted", func(t *testing.T) {
		*f.bindingReached = false
		err := f.deploy(t, deployedImage)
		require.Error(t, err)
		assert.NotErrorIs(t, err, utils.ErrAgentKindNotFound)
		assert.True(t, *f.bindingReached)
	})

	t.Run("a foreign image is still rejected as invalid input", func(t *testing.T) {
		*f.bindingReached = false
		err := f.deploy(t, "docker.io/library/nginx:latest")
		require.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.False(t, *f.bindingReached)
	})
}

// A build or deployment record with no image must never authorise an empty imageId.
func TestDeployAgent_EmptyImageNeverMatchesAnImagelessRecord(t *testing.T) {
	f := newOwnershipFixture("")
	f.oc.ListBuildsFunc = func(context.Context, string, string, string) ([]*models.BuildResponse, error) {
		return []*models.BuildResponse{nil, {ImageId: ""}}, nil
	}
	f.oc.GetDeploymentsFunc = func(context.Context, string, string, string, string) ([]*models.DeploymentResponse, error) {
		return []*models.DeploymentResponse{nil, {ImageId: ""}}, nil
	}

	err := f.deploy(t, "")

	require.ErrorIs(t, err, utils.ErrInvalidInput)
	assert.False(t, *f.bindingReached)
}

// A rejected image must reach the audit trail with the image on it: the deploy's
// own record is only opened after the check, so a warn log would be all there was.
func TestDeployAgent_RejectedImageIsAudited(t *testing.T) {
	f := newOwnershipFixture("")
	ctx, sink, flush := capturingAuditCtx(t)
	foreign := "registry.example.com/other-org/their-agent:build-7"

	_, err := f.svc.DeployAgent(tierGrantedOn(ctx), "acme", "proj1", "my-agent", &spec.DeployAgentRequest{ImageId: foreign})
	require.ErrorIs(t, err, utils.ErrInvalidInput)
	flush()

	event, found := findEvent(sink.captured(), audit.ActionAuthzDeny)
	require.True(t, found, "the rejected image was not recorded")
	assert.Equal(t, audit.OutcomeDeny, event.Outcome)
	assert.Equal(t, "acme", event.OUID)
	assert.Equal(t, "proj1", event.ProjectName)
	assert.Equal(t, "my-agent", event.ResourceName)
	assert.Equal(t, "image-not-owned-by-agent", event.Details["reason"])
	assert.Equal(t, foreign, event.Details["imageId"])
}

// An owned image is not a denial, so it must not leave one in the trail.
func TestDeployAgent_OwnedImageIsNotAuditedAsDenied(t *testing.T) {
	f := newOwnershipFixture("")
	ctx, sink, flush := capturingAuditCtx(t)

	_, err := f.svc.DeployAgent(tierGrantedOn(ctx), "acme", "proj1", "my-agent", &spec.DeployAgentRequest{ImageId: ownBuildImage})
	require.Error(t, err)
	flush()

	_, found := findEvent(sink.captured(), audit.ActionAuthzDeny)
	assert.False(t, found)
}

// A kind-sourced agent never builds, so its rejection must point at the kind's
// versions rather than send the caller looking for builds.
func TestDeployAgent_RejectionHintNamesTheKind(t *testing.T) {
	t.Run("kind agent", func(t *testing.T) {
		err := newOwnershipFixture("my-kind").deploy(t, "docker.io/library/nginx:latest")
		verr := utils.IsValidationError(err)
		require.NotNil(t, verr)
		assert.Contains(t, verr.Message, `published version of agent kind "my-kind"`)
	})

	t.Run("agent with no kind", func(t *testing.T) {
		err := newOwnershipFixture("").deploy(t, "docker.io/library/nginx:latest")
		verr := utils.IsValidationError(err)
		require.NotNil(t, verr)
		assert.NotContains(t, verr.Message, "agent kind")
		assert.Contains(t, verr.Message, "one of the agent's builds")
	})
}
