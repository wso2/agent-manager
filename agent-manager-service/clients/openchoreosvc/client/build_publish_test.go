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

package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// buildPublishStub is a stateful stand-in for the OpenChoreo API covering the calls
// PublishBuildWorkload makes.
type buildPublishStub struct {
	t  *testing.T
	mu sync.Mutex

	workloadExists bool
	releaseExists  bool                // answer generate-release with 409
	binding        *gen.ReleaseBinding // nil: no binding yet
	// bindingWriteFailures answers this many binding writes with 500 before succeeding.
	bindingWriteFailures int

	calls             []string
	workloadBodies    []gen.Workload
	runUpdate         *gen.WorkflowRun
	generatedRelease  string
	createdBinding    *gen.ReleaseBinding
	updatedBinding    *gen.ReleaseBinding
	bindingWriteCount int
}

func (s *buildPublishStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, r.Method+" "+r.URL.Path)
	body, _ := io.ReadAll(r.Body)
	const ns = "/api/v1/namespaces/default"

	switch {
	case r.Method == http.MethodPost && r.URL.Path == ns+"/workloads":
		var wl gen.Workload
		require.NoError(s.t, json.Unmarshal(body, &wl))
		s.workloadBodies = append(s.workloadBodies, wl)
		if s.workloadExists {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"exists","code":"CONFLICT"}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(body)
	case r.Method == http.MethodPut && r.URL.Path == ns+"/workloads/my-agent-workload":
		var wl gen.Workload
		require.NoError(s.t, json.Unmarshal(body, &wl))
		s.workloadBodies = append(s.workloadBodies, wl)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	case r.Method == http.MethodGet && r.URL.Path == ns+"/workflowruns/my-agent-1700":
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"metadata":{"name":"my-agent-1700","annotations":{"keep":"me"},
			"labels":{"openchoreo.dev/project":"proj1","openchoreo.dev/component":"my-agent"}},
			"spec":{"workflow":{"name":"amp-docker"}}}`))
	case r.Method == http.MethodPut && r.URL.Path == ns+"/workflowruns/my-agent-1700":
		var run gen.WorkflowRun
		require.NoError(s.t, json.Unmarshal(body, &run))
		s.runUpdate = &run
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	case r.Method == http.MethodPost && r.URL.Path == ns+"/components/my-agent/generate-release":
		var req gen.GenerateReleaseRequest
		require.NoError(s.t, json.Unmarshal(body, &req))
		if req.ReleaseName != nil {
			s.generatedRelease = *req.ReleaseName
		}
		if s.releaseExists {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"exists","code":"CONFLICT"}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"metadata":{"name":"my-agent-1700"}}`))
	case r.Method == http.MethodGet && r.URL.Path == ns+"/projects/proj1":
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"metadata":{"name":"proj1"},"spec":{"deploymentPipelineRef":{"name":"pipeline"}}}`))
	case r.Method == http.MethodGet && r.URL.Path == ns+"/deploymentpipelines/pipeline":
		// Listed out of promotion order on purpose: the root is the source that is
		// never a target, not the first source listed.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"metadata":{"name":"pipeline"},"spec":{"promotionPaths":[
			{"sourceEnvironmentRef":{"name":"staging"},"targetEnvironmentRefs":[{"name":"production"}]},
			{"sourceEnvironmentRef":{"name":"development"},"targetEnvironmentRefs":[{"name":"staging"}]}]}}`))
	case r.Method == http.MethodGet && r.URL.Path == ns+"/releasebindings/my-agent-development":
		if s.binding == nil {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found","code":"NOT_FOUND"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		require.NoError(s.t, json.NewEncoder(w).Encode(s.binding))
	case r.Method == http.MethodPost && r.URL.Path == ns+"/releasebindings":
		s.bindingWriteCount++
		if s.bindingWriteFailures > 0 {
			s.bindingWriteFailures--
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"conflict","code":"INTERNAL"}`))
			return
		}
		var rb gen.ReleaseBinding
		require.NoError(s.t, json.Unmarshal(body, &rb))
		s.createdBinding = &rb
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(body)
	case r.Method == http.MethodPut && r.URL.Path == ns+"/releasebindings/my-agent-development":
		s.bindingWriteCount++
		if s.bindingWriteFailures > 0 {
			s.bindingWriteFailures--
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"conflict","code":"INTERNAL"}`))
			return
		}
		var rb gen.ReleaseBinding
		require.NoError(s.t, json.Unmarshal(body, &rb))
		s.updatedBinding = &rb
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	default:
		s.t.Errorf("unexpected call %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotImplemented)
	}
}

func strPtrBP(s string) *string { return &s }

func samplePublishRequest() PublishBuildWorkloadRequest {
	basePath := "/chat"
	value := "v1"
	return PublishBuildWorkloadRequest{
		RunName:       "my-agent-1700",
		ProjectName:   "proj1",
		ComponentName: "my-agent",
		Image:         "registry.example.com/my-agent:abc",
		Endpoints: map[string]gen.WorkloadEndpoint{
			"main": {Port: 8000, Type: gen.WorkloadEndpointTypeHTTP, BasePath: &basePath},
		},
		WorkloadOverrides: &gen.WorkloadOverrides{Container: &gen.ContainerOverride{
			Env: &[]gen.EnvVar{{Key: "MODE", Value: &value}},
		}},
	}
}

func withFastBindingRetry(t *testing.T) {
	t.Helper()
	orig := buildBindingRetryDelay
	buildBindingRetryDelay = 0
	t.Cleanup(func() { buildBindingRetryDelay = orig })
}

func TestPublishBuildWorkload_FirstBuildCreatesBindingWithOverrides(t *testing.T) {
	withFastBindingRetry(t)
	stub := &buildPublishStub{t: t}
	c := newTestClient(t, stub)

	require.NoError(t, c.PublishBuildWorkload(context.Background(), "org-1", samplePublishRequest()))

	// Step 4: the Workload carries only image, endpoints and the owner.
	require.Len(t, stub.workloadBodies, 1)
	wl := stub.workloadBodies[0]
	assert.Equal(t, "my-agent-workload", wl.Metadata.Name)
	require.NotNil(t, wl.Spec)
	require.NotNil(t, wl.Spec.Owner)
	assert.Equal(t, "proj1", wl.Spec.Owner.ProjectName)
	assert.Equal(t, "my-agent", wl.Spec.Owner.ComponentName)
	assert.Equal(t, "registry.example.com/my-agent:abc", wl.Spec.Container.Image)
	assert.Nil(t, wl.Spec.Container.Env, "configuration must not reach the Workload")
	require.NotNil(t, wl.Spec.Endpoints)
	assert.Equal(t, 8000, (*wl.Spec.Endpoints)["main"].Port)

	// Step 5: the run records the Workload and keeps its other annotations.
	require.NotNil(t, stub.runUpdate)
	ann := *stub.runUpdate.Metadata.Annotations
	assert.Equal(t, "me", ann["keep"])
	var recorded gen.Workload
	require.NoError(t, json.Unmarshal([]byte(ann[workflowRunWorkloadAnnotationKey]), &recorded))
	assert.Equal(t, "registry.example.com/my-agent:abc", recorded.Spec.Container.Image)

	// Step 6: the release is named after the run.
	assert.Equal(t, "my-agent-1700", stub.generatedRelease)

	// Step 7: the root environment's binding is created with the build's overrides.
	require.NotNil(t, stub.createdBinding)
	assert.Equal(t, "my-agent-development", stub.createdBinding.Metadata.Name)
	assert.Equal(t, "development", stub.createdBinding.Spec.Environment)
	assert.Equal(t, "my-agent-1700", *stub.createdBinding.Spec.ReleaseName)
	require.NotNil(t, stub.createdBinding.Spec.WorkloadOverrides)
	assert.Equal(t, "MODE", (*stub.createdBinding.Spec.WorkloadOverrides.Container.Env)[0].Key)
	assert.Nil(t, stub.updatedBinding)
}

func TestPublishBuildWorkload_RebuildOnlyAdvancesTheReleasePin(t *testing.T) {
	withFastBindingRetry(t)
	existingValue := "from-backend"
	stub := &buildPublishStub{t: t, workloadExists: true, binding: &gen.ReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "my-agent-development"},
		Spec: &gen.ReleaseBindingSpec{
			Environment: "development",
			ReleaseName: strPtrBP("my-agent-1600"),
			WorkloadOverrides: &gen.WorkloadOverrides{Container: &gen.ContainerOverride{
				Env: &[]gen.EnvVar{{Key: "EDITED", Value: &existingValue}},
			}},
		},
	}}
	c := newTestClient(t, stub)

	require.NoError(t, c.PublishBuildWorkload(context.Background(), "org-1", samplePublishRequest()))

	// The existing Workload is replaced with the same body.
	require.Len(t, stub.workloadBodies, 2)
	assert.Equal(t, stub.workloadBodies[0], stub.workloadBodies[1])

	require.Nil(t, stub.createdBinding)
	require.NotNil(t, stub.updatedBinding)
	assert.Equal(t, "my-agent-1700", *stub.updatedBinding.Spec.ReleaseName)
	env := *stub.updatedBinding.Spec.WorkloadOverrides.Container.Env
	require.Len(t, env, 1)
	assert.Equal(t, "EDITED", env[0].Key, "a rebuild must not roll back the environment's configuration")
}

func TestPublishBuildWorkload_RetryAcceptsTheRunsOwnExistingRelease(t *testing.T) {
	withFastBindingRetry(t)
	stub := &buildPublishStub{t: t, workloadExists: true, releaseExists: true}
	c := newTestClient(t, stub)

	require.NoError(t, c.PublishBuildWorkload(context.Background(), "org-1", samplePublishRequest()))
	assert.Equal(t, "my-agent-1700", stub.generatedRelease)
	require.NotNil(t, stub.createdBinding, "the publish must carry on to the binding")
}

func TestPublishBuildWorkload_RetriesTheBindingWriteOn500(t *testing.T) {
	withFastBindingRetry(t)
	stub := &buildPublishStub{t: t, bindingWriteFailures: 2}
	c := newTestClient(t, stub)

	require.NoError(t, c.PublishBuildWorkload(context.Background(), "org-1", samplePublishRequest()))
	assert.Equal(t, 3, stub.bindingWriteCount)
	require.NotNil(t, stub.createdBinding)
}

func TestPublishBuildWorkload_GivesUpAfterMaxBindingAttempts(t *testing.T) {
	withFastBindingRetry(t)
	stub := &buildPublishStub{t: t, bindingWriteFailures: buildBindingMaxAttempts}
	c := newTestClient(t, stub)

	err := c.PublishBuildWorkload(context.Background(), "org-1", samplePublishRequest())
	require.Error(t, err)
	assert.Equal(t, buildBindingMaxAttempts, stub.bindingWriteCount)
}

func sampleHealthChecks() map[string]any {
	return map[string]any{
		"readiness": map[string]any{"type": "HTTP", "path": "/ready", "port": float64(8000)},
		"liveness":  map[string]any{"enabled": false},
	}
}

func TestPublishBuildWorkload_FirstBuildCreatesBindingWithHealthChecks(t *testing.T) {
	withFastBindingRetry(t)
	stub := &buildPublishStub{t: t}
	c := newTestClient(t, stub)
	req := samplePublishRequest()
	req.HealthChecks = sampleHealthChecks()

	require.NoError(t, c.PublishBuildWorkload(context.Background(), "org-1", req))

	require.NotNil(t, stub.createdBinding)
	require.NotNil(t, stub.createdBinding.Spec.ComponentTypeEnvironmentConfigs)
	assert.Equal(t, sampleHealthChecks(), (*stub.createdBinding.Spec.ComponentTypeEnvironmentConfigs)["healthChecks"])
}

func TestPublishBuildWorkload_FirstBuildWithoutHealthChecksSetsNone(t *testing.T) {
	withFastBindingRetry(t)
	stub := &buildPublishStub{t: t}
	c := newTestClient(t, stub)

	require.NoError(t, c.PublishBuildWorkload(context.Background(), "org-1", samplePublishRequest()))

	require.NotNil(t, stub.createdBinding)
	assert.Nil(t, stub.createdBinding.Spec.ComponentTypeEnvironmentConfigs)
}

func TestPublishBuildWorkload_RebuildReplacesHealthChecksAndKeepsOtherConfigs(t *testing.T) {
	withFastBindingRetry(t)
	stub := &buildPublishStub{t: t, workloadExists: true, binding: &gen.ReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "my-agent-development"},
		Spec: &gen.ReleaseBindingSpec{
			Environment: "development",
			ReleaseName: strPtrBP("my-agent-1600"),
			ComponentTypeEnvironmentConfigs: &map[string]any{
				"runtimeClassName": "kata",
				"healthChecks":     map[string]any{"startup": map[string]any{"path": "/old"}},
			},
		},
	}}
	c := newTestClient(t, stub)
	req := samplePublishRequest()
	req.HealthChecks = sampleHealthChecks()

	require.NoError(t, c.PublishBuildWorkload(context.Background(), "org-1", req))

	require.NotNil(t, stub.updatedBinding)
	configs := *stub.updatedBinding.Spec.ComponentTypeEnvironmentConfigs
	assert.Equal(t, sampleHealthChecks(), configs["healthChecks"], "the rebuild's health checks replace the earlier build's")
	assert.Equal(t, "kata", configs["runtimeClassName"], "other component-type configs are kept")
}

func TestPublishBuildWorkload_RebuildWithoutHealthChecksRemovesTheEarlierOnes(t *testing.T) {
	withFastBindingRetry(t)
	stub := &buildPublishStub{t: t, workloadExists: true, binding: &gen.ReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "my-agent-development"},
		Spec: &gen.ReleaseBindingSpec{
			Environment: "development",
			ReleaseName: strPtrBP("my-agent-1600"),
			ComponentTypeEnvironmentConfigs: &map[string]any{
				"runtimeClassName": "kata",
				"healthChecks":     map[string]any{"startup": map[string]any{"path": "/old"}},
			},
		},
	}}
	c := newTestClient(t, stub)

	require.NoError(t, c.PublishBuildWorkload(context.Background(), "org-1", samplePublishRequest()))

	require.NotNil(t, stub.updatedBinding)
	configs := *stub.updatedBinding.Spec.ComponentTypeEnvironmentConfigs
	_, kept := configs["healthChecks"]
	assert.False(t, kept, "a build without health checks puts the environment back on the default probes")
	assert.Equal(t, "kata", configs["runtimeClassName"])
}

func TestPublishBuildWorkload_MalformedHealthChecksWriteNothing(t *testing.T) {
	stub := &buildPublishStub{t: t}
	c := newTestClient(t, stub)
	req := samplePublishRequest()
	req.HealthChecks = map[string]any{"readiness": "yes"}

	err := c.PublishBuildWorkload(context.Background(), "org-1", req)
	require.ErrorIs(t, err, utils.ErrBadRequest)
	assert.Empty(t, stub.calls, "nothing is written when the health checks are malformed")
}

func TestBuildHealthChecks(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   any
		want    map[string]any
		wantErr bool
	}{
		{name: "unset", value: nil},
		{name: "empty object", value: map[string]any{}},
		{name: "checks", value: sampleHealthChecks(), want: sampleHealthChecks()},
		{name: "null probe", value: map[string]any{"startup": nil}, want: map[string]any{"startup": nil}},
		{name: "not an object", value: "probes", wantErr: true},
		{name: "list", value: []any{map[string]any{}}, wantErr: true},
		{name: "probe not an object", value: map[string]any{"liveness": true}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildHealthChecks(tc.value)
			if tc.wantErr {
				require.ErrorIs(t, err, utils.ErrBadRequest)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGetBuildRun(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/namespaces/default/workflowruns/my-agent-1700":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"metadata":{"name":"my-agent-1700",
				"labels":{"openchoreo.dev/project":"proj1","openchoreo.dev/component":"my-agent"}},
				"spec":{"workflow":{"name":"amp-docker","parameters":{"healthChecks":{"readiness":{"path":"/ready"}}}}},
				"status":{"conditions":[{"type":"WorkflowRunning","status":"True","reason":"Running","message":"","lastTransitionTime":"2026-11-04T00:00:00Z"}]}}`))
		default:
			// A bare 404 (e.g. from a gateway) with no JSON error body.
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	info, err := c.GetBuildRun(context.Background(), "org-1", "my-agent-1700")
	require.NoError(t, err)
	assert.Equal(t, "my-agent-1700", info.Name)
	assert.Equal(t, "proj1", info.ProjectName)
	assert.Equal(t, "my-agent", info.ComponentName)
	assert.Equal(t, "amp-docker", info.WorkflowName)
	assert.Equal(t, WorkflowStatusRunning, info.Status)
	assert.Equal(t, map[string]any{"readiness": map[string]any{"path": "/ready"}}, info.HealthChecks,
		"the run's health checks are returned as the run carries them")

	// A 404 without an error body is still utils.ErrNotFound.
	_, err = c.GetBuildRun(context.Background(), "org-1", "missing")
	require.ErrorIs(t, err, utils.ErrNotFound)
}

func TestPipelineRootEnvironment(t *testing.T) {
	paths := []models.PromotionPath{
		{SourceEnvironmentRef: "staging", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "production"}}},
		{SourceEnvironmentRef: "", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "x"}}},
		{SourceEnvironmentRef: "development", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "staging"}}},
	}
	assert.Equal(t, "development", pipelineRootEnvironment(paths))
	assert.Empty(t, pipelineRootEnvironment(nil))
}

func TestWithBuildClient(t *testing.T) {
	params := map[string]interface{}{"repository": map[string]interface{}{"url": "u"}}
	got := withBuildClient(params, &BuildClientParams{SecretRefName: "amp-build-org-1-secrets"})
	assert.Equal(t, map[string]interface{}{"secretRef": "amp-build-org-1-secrets"}, got[workflowParamAMPBuild],
		"only a SecretReference name is passed, never a secret store location")
	_, leaked := params[workflowParamAMPBuild]
	assert.False(t, leaked, "the component's parameter map must not be modified")
}

func TestAttachBuildClient(t *testing.T) {
	cfg := config.GetConfig()
	orig := cfg.BuildClient.Workflows
	t.Cleanup(func() { cfg.BuildClient.Workflows = orig })
	cfg.BuildClient.Workflows = []string{"amp-docker", "amp-old"}

	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/clusterworkflows/amp-docker", "/api/v1/namespaces/default/workflows/amp-docker":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"metadata":{"name":"amp-docker"},"spec":{"parameters":{"openAPIV3Schema":{
				"type":"object","properties":{"ampBuild":{"type":"object"},"repository":{"type":"object"}}}}}}`))
		case "/api/v1/clusterworkflows/amp-old":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"metadata":{"name":"amp-old"},"spec":{"parameters":{"openAPIV3Schema":{
				"type":"object","properties":{"repository":{"type":"object"}}}}}}`))
		default:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom","code":"INTERNAL"}`))
		}
	}))
	bc := &BuildClientParams{SecretRefName: "ref"}
	ctx := context.Background()

	assert.True(t, c.attachBuildClient(ctx, "default", gen.WorkflowRunConfigKindClusterWorkflow, "amp-docker", bc))
	assert.True(t, c.attachBuildClient(ctx, "default", gen.WorkflowRunConfigKindWorkflow, "amp-docker", bc))
	assert.False(t, c.attachBuildClient(ctx, "default", gen.WorkflowRunConfigKindClusterWorkflow, "amp-old", bc),
		"a workflow that does not declare ampBuild keeps its previous path")
	assert.False(t, c.attachBuildClient(ctx, "default", gen.WorkflowRunConfigKindClusterWorkflow, "dockerfile-builder", bc),
		"a workflow not listed in AMP_BUILD_CLIENT_WORKFLOWS never receives it")
	assert.False(t, c.attachBuildClient(ctx, "default", gen.WorkflowRunConfigKindClusterWorkflow, "amp-docker", nil))

	cfg.BuildClient.Workflows = []string{"amp-broken"}
	assert.False(t, c.attachBuildClient(ctx, "default", gen.WorkflowRunConfigKindClusterWorkflow, "amp-broken", bc),
		"a workflow that cannot be read keeps its previous path rather than failing the build")
}

type staticAuthProvider struct{ token string }

func (p staticAuthProvider) GetToken(context.Context) (string, error) { return p.token, nil }
func (staticAuthProvider) InvalidateToken()                           {}

// The build publisher's OpenChoreo calls carry this service's credential and the
// caller's organization as X-Impersonate-Org (set from the context's resolved org).
func TestBuildPublisherCallsCarryTheResolvedOrg(t *testing.T) {
	var gotAuth, gotOrg string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotOrg = r.Header.Get("Authorization"), r.Header.Get(HeaderImpersonateOrg)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"metadata":{"name":"run-1"},"spec":{"workflow":{"name":"amp-docker"}}}`))
	}))
	t.Cleanup(srv.Close)
	oc, err := NewOpenChoreoClient(&Config{BaseURL: srv.URL, AuthProvider: staticAuthProvider{token: "service-token"}, DefaultNamespace: "default"})
	require.NoError(t, err)

	ctx := orgctx.WithResolvedOrg(context.Background(), orgctx.ResolvedOrg{OUID: "org-1"})
	_, err = oc.GetBuildRun(ctx, "org-1", "run-1")
	require.NoError(t, err)
	assert.Equal(t, "Bearer service-token", gotAuth)
	assert.Equal(t, "org-1", gotOrg)
}
