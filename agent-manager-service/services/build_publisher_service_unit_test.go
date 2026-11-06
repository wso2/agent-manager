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
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/workloadpolicy"
)

const bpOrg = "6f1e2c3d-0000-4000-8000-000000000001"

type bpBackgroundKey struct{}

func bpEnabledConfig() config.Config {
	return config.Config{BuildClient: config.BuildClientConfig{
		Enabled:   true,
		Workflows: []string{"amp-docker", "amp-ballerina-buildpack", "amp-google-cloud-buildpacks"},
	}}
}

func bpRequest(t *testing.T, workload string, overrides string) *models.PublishBuildWorkloadRequest {
	t.Helper()
	req := &models.PublishBuildWorkloadRequest{Workload: json.RawMessage(workload)}
	if overrides != "" {
		req.WorkloadOverrides = json.RawMessage(overrides)
	}
	return req
}

const bpWorkload = `{"metadata":{"name":"ignored-name","namespace":"ignored-ns"},
	"spec":{"owner":{"projectName":"other-proj","componentName":"other-agent"},
	"container":{"image":"registry.example.com/my-agent:abc"},
	"endpoints":{"main":{"type":"HTTP","port":8000,"basePath":"/chat","visibility":["external"],"schema":{"type":"REST","content":"openapi: 3.0.0"}}}}}`

const bpOverrides = `{"container":{"env":[{"key":"MODE","value":"v1"}],
	"files":[{"key":"CFG","mountPath":"/etc/cfg","valueFrom":{"secretKeyRef":{"name":"s","key":"k"}}}]}}`

// bpHappyOC returns an OpenChoreo mock for a running amp-docker build of a
// source-built agent; the publish call records what it received.
func bpHappyOC(published *client.PublishBuildWorkloadRequest, ocCtxSeen *context.Context) *clientmocks.OpenChoreoClientMock {
	return &clientmocks.OpenChoreoClientMock{
		GetBuildRunFunc: func(ctx context.Context, ouID, runName string) (*client.BuildRunInfo, error) {
			if ocCtxSeen != nil {
				*ocCtxSeen = ctx
			}
			return &client.BuildRunInfo{
				Name: runName, ProjectName: "proj1", ComponentName: "my-agent",
				WorkflowName: "amp-docker", Status: client.WorkflowStatusRunning,
				HealthChecks: bpRunHealthChecks(),
			}, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{
				UUID: "agent-uuid", Name: "my-agent",
				Provisioning: models.Provisioning{Type: string(utils.InternalAgent)},
			}, nil
		},
		PublishBuildWorkloadFunc: func(_ context.Context, _ string, req client.PublishBuildWorkloadRequest) error {
			if published != nil {
				*published = req
			}
			return nil
		},
	}
}

// bpRunHealthChecks is the healthChecks parameter bpHappyOC's run carries.
func bpRunHealthChecks() map[string]any {
	return map[string]any{"readiness": map[string]any{"path": "/ready"}}
}

func TestBuildPublisherService_PublishesForTheVerifiedBuild(t *testing.T) {
	var published client.PublishBuildWorkloadRequest
	var ocCtx context.Context
	oc := bpHappyOC(&published, &ocCtx)
	svc := NewBuildPublisherService(oc, bpEnabledConfig(), discardLogger())
	bg := context.WithValue(context.Background(), bpBackgroundKey{}, "background")
	svc.(BuildPublisherBackgroundContextSetter).SetBackgroundContext(bg)

	// The request context carries the build's token, which must not reach OpenChoreo.
	reqCtx := jwtassertion.ContextWithJWT(auditableCtx(t), "build-client-token")
	err := svc.PublishWorkload(reqCtx, bpOrg, "my-agent-1700", bpRequest(t, bpWorkload, bpOverrides))
	require.NoError(t, err)

	// Identity comes from the verified run, never from the document.
	assert.Equal(t, "my-agent-1700", published.RunName)
	assert.Equal(t, "proj1", published.ProjectName)
	assert.Equal(t, "my-agent", published.ComponentName)
	assert.Equal(t, "registry.example.com/my-agent:abc", published.Image)
	require.Contains(t, published.Endpoints, "main")
	assert.Equal(t, 8000, published.Endpoints["main"].Port)
	require.NotNil(t, published.Endpoints["main"].Visibility)
	assert.Len(t, *published.Endpoints["main"].Visibility, 1)
	require.NotNil(t, published.Endpoints["main"].Schema)
	require.NotNil(t, published.WorkloadOverrides)
	require.NotNil(t, published.WorkloadOverrides.Container.Env)
	assert.Equal(t, "MODE", (*published.WorkloadOverrides.Container.Env)[0].Key)
	// The health checks come from the verified run, not from the document.
	assert.Equal(t, bpRunHealthChecks(), published.HealthChecks)

	// OpenChoreo calls run on the background context, for the caller's organization only.
	require.NotNil(t, ocCtx)
	assert.Empty(t, jwtassertion.GetJWTFromContext(ocCtx), "the build's token must not be forwarded")
	assert.Equal(t, "background", ocCtx.Value(bpBackgroundKey{}))
	org, ok := orgctx.GetResolvedOrg(ocCtx)
	require.True(t, ok)
	assert.Equal(t, bpOrg, org.OUID)
}

// bpCountingRecorder counts the audit events written to it.
type bpCountingRecorder struct{ events int }

func (r *bpCountingRecorder) Record(context.Context, audit.Event) { r.events++ }
func (r *bpCountingRecorder) RecordSync(context.Context, audit.Event) error {
	r.events++
	return nil
}
func (r *bpCountingRecorder) Close(context.Context) error { return nil }

// bpUntouchableOC fails the test if any call reaches OpenChoreo.
func bpUntouchableOC(t *testing.T) *clientmocks.OpenChoreoClientMock {
	return &clientmocks.OpenChoreoClientMock{
		GetBuildRunFunc: func(context.Context, string, string) (*client.BuildRunInfo, error) {
			t.Error("a suspended organization's build must not be looked up")
			return nil, fmt.Errorf("unexpected call")
		},
		GetComponentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
			t.Error("a suspended organization's agent must not be looked up")
			return nil, fmt.Errorf("unexpected call")
		},
		PublishBuildWorkloadFunc: func(context.Context, string, client.PublishBuildWorkloadRequest) error {
			t.Error("a suspended organization must not publish")
			return fmt.Errorf("unexpected call")
		},
	}
}

func TestBuildPublisherService_RefusesASuspendedOrganization(t *testing.T) {
	svc := NewBuildPublisherService(bpUntouchableOC(t), bpEnabledConfig(), discardLogger())
	var guarded []string
	// The deployment's workload guard is installed on the background context
	// (app.Run), which the publisher derives its OpenChoreo calls from.
	bg := workloadpolicy.WithGuard(context.Background(), func(_ context.Context, org string) error {
		guarded = append(guarded, org)
		return workloadpolicy.ErrSuspended
	})
	svc.(BuildPublisherBackgroundContextSetter).SetBackgroundContext(bg)
	recorder := &bpCountingRecorder{}

	err := svc.PublishWorkload(audit.WithRecorder(context.Background(), recorder), bpOrg, "my-agent-1700",
		bpRequest(t, bpWorkload, bpOverrides))
	require.ErrorIs(t, err, workloadpolicy.ErrSuspended)
	assert.NotErrorIs(t, err, errBuildPublishUpstream, "a suspension is final, not a retryable failure")
	assert.Equal(t, []string{bpOrg}, guarded, "the guard is asked about the build token's organization")
	assert.Zero(t, recorder.events, "nothing is recorded for a publish that never starts")
}

func TestBuildPublisherService_PassesOnASuspensionFoundWhilePublishing(t *testing.T) {
	// The organization is suspended between the service's check and the client's:
	// the client refuses, and the refusal is answered as a suspension.
	oc := bpHappyOC(nil, nil)
	oc.PublishBuildWorkloadFunc = func(context.Context, string, client.PublishBuildWorkloadRequest) error {
		return workloadpolicy.ErrSuspended
	}
	svc := NewBuildPublisherService(oc, bpEnabledConfig(), discardLogger())

	err := svc.PublishWorkload(auditableCtx(t), bpOrg, "my-agent-1700", bpRequest(t, bpWorkload, bpOverrides))
	require.ErrorIs(t, err, workloadpolicy.ErrSuspended)
	assert.NotErrorIs(t, err, errBuildPublishUpstream)
}

func TestBuildPublisherService_PublishesForAnAdmittedOrganization(t *testing.T) {
	var published client.PublishBuildWorkloadRequest
	svc := NewBuildPublisherService(bpHappyOC(&published, nil), bpEnabledConfig(), discardLogger())
	var guarded []string
	bg := workloadpolicy.WithGuard(context.Background(), func(_ context.Context, org string) error {
		guarded = append(guarded, org)
		return nil
	})
	svc.(BuildPublisherBackgroundContextSetter).SetBackgroundContext(bg)

	require.NoError(t, svc.PublishWorkload(auditableCtx(t), bpOrg, "my-agent-1700", bpRequest(t, bpWorkload, bpOverrides)))
	assert.Equal(t, "my-agent-1700", published.RunName)
	assert.Equal(t, []string{bpOrg}, guarded)
}

func TestBuildPublisherService_RefusesToPublishWithoutAnAuditTrail(t *testing.T) {
	oc := bpHappyOC(nil, nil)
	oc.PublishBuildWorkloadFunc = nil // must not be reached
	svc := NewBuildPublisherService(oc, bpEnabledConfig(), discardLogger())
	err := svc.PublishWorkload(context.Background(), bpOrg, "my-agent-1700", bpRequest(t, bpWorkload, ""))
	require.ErrorIs(t, err, audit.ErrRecorderUnavailable)
}

func TestBuildPublisherService_NullOverridesMeanNone(t *testing.T) {
	var published client.PublishBuildWorkloadRequest
	svc := NewBuildPublisherService(bpHappyOC(&published, nil), bpEnabledConfig(), discardLogger())
	require.NoError(t, svc.PublishWorkload(auditableCtx(t), bpOrg, "my-agent-1700", bpRequest(t, bpWorkload, "null")))
	assert.Nil(t, published.WorkloadOverrides)
}

func TestBuildPublisherService_RefusesBeforeCallingOpenChoreo(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.Config
		ouID    string
		build   string
		req     *models.PublishBuildWorkloadRequest
		wantErr error
	}{
		{
			name: "feature disabled", cfg: config.Config{}, ouID: bpOrg, build: "b-1",
			req: bpRequest(t, bpWorkload, ""), wantErr: utils.ErrNotFound,
		},
		{
			name: "no organization", cfg: bpEnabledConfig(), ouID: "", build: "b-1",
			req: bpRequest(t, bpWorkload, ""), wantErr: utils.ErrForbidden,
		},
		{
			name: "invalid build name", cfg: bpEnabledConfig(), ouID: bpOrg, build: "Bad_Name",
			req: bpRequest(t, bpWorkload, ""), wantErr: utils.ErrBadRequest,
		},
		{
			name: "missing workload", cfg: bpEnabledConfig(), ouID: bpOrg, build: "b-1",
			req: bpRequest(t, ``, ""), wantErr: utils.ErrBadRequest,
		},
		{
			name: "missing image", cfg: bpEnabledConfig(), ouID: bpOrg, build: "b-1",
			req: bpRequest(t, `{"metadata":{"name":"x"},"spec":{"container":{"image":""}}}`, ""), wantErr: utils.ErrBadRequest,
		},
		{
			name: "env on the workload", cfg: bpEnabledConfig(), ouID: bpOrg, build: "b-1",
			req:     bpRequest(t, `{"metadata":{"name":"x"},"spec":{"container":{"image":"i","env":[{"key":"A","value":"b"}]}}}`, ""),
			wantErr: utils.ErrBadRequest,
		},
		{
			name: "command on the workload", cfg: bpEnabledConfig(), ouID: bpOrg, build: "b-1",
			req:     bpRequest(t, `{"metadata":{"name":"x"},"spec":{"container":{"image":"i","command":["sh"]}}}`, ""),
			wantErr: utils.ErrBadRequest,
		},
		{
			name: "unknown field", cfg: bpEnabledConfig(), ouID: bpOrg, build: "b-1",
			req:     bpRequest(t, `{"metadata":{"name":"x"},"spec":{"container":{"image":"i"}},"extra":1}`, ""),
			wantErr: utils.ErrBadRequest,
		},
		{
			name: "endpoint port out of range", cfg: bpEnabledConfig(), ouID: bpOrg, build: "b-1",
			req:     bpRequest(t, `{"metadata":{"name":"x"},"spec":{"container":{"image":"i"},"endpoints":{"e":{"type":"HTTP","port":0}}}}`, ""),
			wantErr: utils.ErrBadRequest,
		},
		{
			name: "malformed overrides", cfg: bpEnabledConfig(), ouID: bpOrg, build: "b-1",
			req: bpRequest(t, bpWorkload, `{"container":{"command":["x"]}}`), wantErr: utils.ErrBadRequest,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Every OpenChoreo func is nil: reaching OpenChoreo panics the test.
			svc := NewBuildPublisherService(&clientmocks.OpenChoreoClientMock{}, tc.cfg, discardLogger())
			err := svc.PublishWorkload(auditableCtx(t), tc.ouID, tc.build, tc.req)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestBuildPublisherService_VerifiesTheBuild(t *testing.T) {
	running := func(info client.BuildRunInfo) func(context.Context, string, string) (*client.BuildRunInfo, error) {
		return func(_ context.Context, _, runName string) (*client.BuildRunInfo, error) {
			info.Name = runName
			return &info, nil
		}
	}
	tests := []struct {
		name         string
		getRun       func(context.Context, string, string) (*client.BuildRunInfo, error)
		getComponent func(context.Context, string, string, string) (*models.AgentResponse, error)
		wantErr      error
		notErr       error
	}{
		{
			name: "build not in the organization",
			getRun: func(context.Context, string, string) (*client.BuildRunInfo, error) {
				return nil, fmt.Errorf("%w: workflow run", utils.ErrNotFound)
			},
			wantErr: utils.ErrNotFound,
		},
		{
			name: "upstream refusal is not the caller's 403",
			getRun: func(context.Context, string, string) (*client.BuildRunInfo, error) {
				return nil, fmt.Errorf("%w: service credential refused", utils.ErrForbidden)
			},
			wantErr: errBuildPublishUpstream,
			notErr:  utils.ErrForbidden,
		},
		{
			name:    "run without agent labels",
			getRun:  running(client.BuildRunInfo{WorkflowName: "amp-docker", Status: client.WorkflowStatusRunning}),
			wantErr: utils.ErrForbidden,
		},
		{
			name: "workflow that does not take the build token",
			getRun: running(client.BuildRunInfo{
				ProjectName: "p", ComponentName: "c",
				WorkflowName: "dockerfile-builder", Status: client.WorkflowStatusRunning,
			}),
			wantErr: utils.ErrForbidden,
		},
		{
			name: "finished build",
			getRun: running(client.BuildRunInfo{
				ProjectName: "p", ComponentName: "c",
				WorkflowName: "amp-docker", Status: client.WorkflowStatusCompleted,
			}),
			wantErr: utils.ErrConflict,
		},
		{
			name: "failed build",
			getRun: running(client.BuildRunInfo{
				ProjectName: "p", ComponentName: "c",
				WorkflowName: "amp-docker", Status: client.WorkflowStatusFailed,
			}),
			wantErr: utils.ErrConflict,
		},
		{
			name: "component that is not an agent",
			getRun: running(client.BuildRunInfo{
				ProjectName: "p", ComponentName: "c",
				WorkflowName: "amp-docker", Status: client.WorkflowStatusRunning,
			}),
			getComponent: func(context.Context, string, string, string) (*models.AgentResponse, error) {
				return nil, fmt.Errorf("%w: component", utils.ErrNotFound)
			},
			wantErr: utils.ErrForbidden,
		},
		{
			name: "kind-sourced agent",
			getRun: running(client.BuildRunInfo{
				ProjectName: "p", ComponentName: "c",
				WorkflowName: "amp-docker", Status: client.WorkflowStatusRunning,
			}),
			getComponent: func(context.Context, string, string, string) (*models.AgentResponse, error) {
				return &models.AgentResponse{KindName: "k", Provisioning: models.Provisioning{Type: string(utils.InternalAgent)}}, nil
			},
			wantErr: utils.ErrForbidden,
		},
		{
			name: "external agent",
			getRun: running(client.BuildRunInfo{
				ProjectName: "p", ComponentName: "c",
				WorkflowName: "amp-docker", Status: client.WorkflowStatusRunning,
			}),
			getComponent: func(context.Context, string, string, string) (*models.AgentResponse, error) {
				return &models.AgentResponse{Provisioning: models.Provisioning{Type: string(utils.ExternalAgent)}}, nil
			},
			wantErr: utils.ErrForbidden,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// PublishBuildWorkloadFunc is nil: a refused build must never be published.
			oc := &clientmocks.OpenChoreoClientMock{GetBuildRunFunc: tc.getRun, GetComponentFunc: tc.getComponent}
			svc := NewBuildPublisherService(oc, bpEnabledConfig(), discardLogger())
			err := svc.PublishWorkload(auditableCtx(t), bpOrg, "my-agent-1700", bpRequest(t, bpWorkload, ""))
			require.ErrorIs(t, err, tc.wantErr)
			if tc.notErr != nil {
				assert.NotErrorIs(t, err, tc.notErr)
			}
		})
	}
}

func TestBuildPublisherService_PublishFailures(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantErr error
		notErr  error
	}{
		{name: "OpenChoreo rejects the workload", err: fmt.Errorf("%w: invalid endpoint", utils.ErrBadRequest), wantErr: utils.ErrBadRequest},
		{
			name:    "the build already published a different workload",
			err:     fmt.Errorf("%w: workflow run", client.ErrBuildWorkloadAlreadyPublished),
			wantErr: ErrBuildWorkloadAlreadyPublished, notErr: errBuildPublishUpstream,
		},
		{
			name:    "missing project is not the caller's 404",
			err:     fmt.Errorf("%w: project", utils.ErrNotFound),
			wantErr: errBuildPublishUpstream, notErr: utils.ErrNotFound,
		},
		{
			name:    "refused service credential is not the caller's 403",
			err:     fmt.Errorf("%w: forbidden", utils.ErrForbidden),
			wantErr: errBuildPublishUpstream, notErr: utils.ErrForbidden,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			oc := bpHappyOC(nil, nil)
			oc.PublishBuildWorkloadFunc = func(context.Context, string, client.PublishBuildWorkloadRequest) error { return tc.err }
			svc := NewBuildPublisherService(oc, bpEnabledConfig(), discardLogger())
			err := svc.PublishWorkload(auditableCtx(t), bpOrg, "my-agent-1700", bpRequest(t, bpWorkload, ""))
			require.ErrorIs(t, err, tc.wantErr)
			if tc.notErr != nil {
				assert.NotErrorIs(t, err, tc.notErr)
			}
		})
	}
}

func TestBuildPublisherService_OpenChoreoContextHasADeadlineBeforeTheGateways(t *testing.T) {
	svc := NewBuildPublisherService(&clientmocks.OpenChoreoClientMock{}, bpEnabledConfig(), discardLogger()).(*buildPublisherService)
	ocCtx, done := svc.openChoreoContext(context.Background(), bpOrg)
	defer done()

	deadline, ok := ocCtx.Deadline()
	require.True(t, ok, "a publish must not run for as long as OpenChoreo takes")
	assert.WithinDuration(t, time.Now().Add(buildPublishTimeout), deadline, time.Second)
	assert.Less(t, buildPublishTimeout, 30*time.Second, "it must end before the gateway's 30s request cap")
}

func TestBuildPublisherService_StopsAPublishThatOutlivesItsDeadline(t *testing.T) {
	orig := buildPublishTimeout
	buildPublishTimeout = 20 * time.Millisecond
	t.Cleanup(func() { buildPublishTimeout = orig })

	oc := bpHappyOC(nil, nil)
	var cause error
	oc.PublishBuildWorkloadFunc = func(ctx context.Context, _ string, _ client.PublishBuildWorkloadRequest) error {
		<-ctx.Done() // OpenChoreo never answers
		cause = context.Cause(ctx)
		return ctx.Err()
	}
	svc := NewBuildPublisherService(oc, bpEnabledConfig(), discardLogger())

	err := svc.PublishWorkload(auditableCtx(t), bpOrg, "my-agent-1700", bpRequest(t, bpWorkload, ""))
	require.ErrorIs(t, err, errBuildPublishUpstream, "a timed-out publish is a 5xx, which the build step retries")
	assert.ErrorIs(t, cause, errBuildPublishTimedOut)
}

func TestBuildPublisherService_OpenChoreoContextEndsWithTheRequest(t *testing.T) {
	svc := NewBuildPublisherService(&clientmocks.OpenChoreoClientMock{}, bpEnabledConfig(), discardLogger()).(*buildPublisherService)
	reqCtx, cancelReq := context.WithCancel(context.Background())
	ocCtx, done := svc.openChoreoContext(reqCtx, bpOrg)
	defer done()

	require.NoError(t, ocCtx.Err())
	cancelReq()
	<-ocCtx.Done()
	assert.ErrorIs(t, context.Cause(ocCtx), context.Canceled)
}
