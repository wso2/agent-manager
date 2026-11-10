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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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
	// recordedWorkload is the run's openchoreo.dev/workload annotation ("" for none).
	recordedWorkload string

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
		annotations := map[string]string{"keep": "me"}
		if s.recordedWorkload != "" {
			annotations[workflowRunWorkloadAnnotationKey] = s.recordedWorkload
		}
		w.WriteHeader(http.StatusOK)
		require.NoError(s.t, json.NewEncoder(w).Encode(map[string]any{
			"metadata": map[string]any{
				"name":        "my-agent-1700",
				"annotations": annotations,
				"labels":      map[string]string{"openchoreo.dev/project": "proj1", "openchoreo.dev/component": "my-agent"},
			},
			"spec": map[string]any{"workflow": map[string]any{"name": "amp-docker"}},
		}))
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

// publishedOnce publishes samplePublishRequest to a fresh stub and returns what
// that first publish recorded on the run and the binding it created.
func publishedOnce(t *testing.T) (string, *gen.ReleaseBinding) {
	t.Helper()
	stub := &buildPublishStub{t: t}
	require.NoError(t, newTestClient(t, stub).PublishBuildWorkload(context.Background(), "org-1", samplePublishRequest()))
	require.NotNil(t, stub.runUpdate)
	require.NotNil(t, stub.createdBinding)
	return (*stub.runUpdate.Metadata.Annotations)[workflowRunWorkloadAnnotationKey], stub.createdBinding
}

func TestPublishBuildWorkload_RepeatOfTheSameWorkloadWritesNothingAgain(t *testing.T) {
	withFastBindingRetry(t)
	recorded, binding := publishedOnce(t)
	stub := &buildPublishStub{t: t, workloadExists: true, releaseExists: true, recordedWorkload: recorded, binding: binding}
	c := newTestClient(t, stub)

	require.NoError(t, c.PublishBuildWorkload(context.Background(), "org-1", samplePublishRequest()))

	assert.Empty(t, stub.workloadBodies, "the Workload the run already records is not written again")
	assert.Nil(t, stub.runUpdate, "the run's record is not written again")
	assert.Equal(t, "my-agent-1700", stub.generatedRelease, "the release is still made sure of")
	assert.Zero(t, stub.bindingWriteCount, "a binding already on this release is not written again")
}

func TestPublishBuildWorkload_RepeatFinishesWhatAnEarlierAttemptLeftUndone(t *testing.T) {
	withFastBindingRetry(t)
	recorded, _ := publishedOnce(t)
	// The earlier attempt recorded the Workload but stopped before binding it.
	stub := &buildPublishStub{t: t, workloadExists: true, recordedWorkload: recorded}
	c := newTestClient(t, stub)

	require.NoError(t, c.PublishBuildWorkload(context.Background(), "org-1", samplePublishRequest()))

	assert.Empty(t, stub.workloadBodies)
	assert.Equal(t, "my-agent-1700", stub.generatedRelease)
	require.NotNil(t, stub.createdBinding, "the binding the earlier attempt did not reach is created")
	assert.Equal(t, "my-agent-1700", *stub.createdBinding.Spec.ReleaseName)
}

func TestPublishBuildWorkload_RefusesADifferentWorkloadForAnAlreadyPublishedRun(t *testing.T) {
	recorded, _ := publishedOnce(t)
	stub := &buildPublishStub{t: t, workloadExists: true, recordedWorkload: recorded}
	c := newTestClient(t, stub)
	req := samplePublishRequest()
	req.Image = "registry.example.com/my-agent:other"

	err := c.PublishBuildWorkload(context.Background(), "org-1", req)
	require.ErrorIs(t, err, ErrBuildWorkloadAlreadyPublished)
	assert.Equal(t, []string{"GET /api/v1/namespaces/default/workflowruns/my-agent-1700"}, stub.calls,
		"nothing is written for a different workload")
}

func TestPublishBuildWorkload_UnreadableRecordedWorkloadWritesNothing(t *testing.T) {
	stub := &buildPublishStub{t: t, recordedWorkload: "{not json"}
	c := newTestClient(t, stub)

	err := c.PublishBuildWorkload(context.Background(), "org-1", samplePublishRequest())
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrBuildWorkloadAlreadyPublished)
	assert.Len(t, stub.calls, 1, "nothing is written when the run's record cannot be read")
}

func TestHasResourceLabels(t *testing.T) {
	c := &openChoreoClient{resourceLabels: map[string]string{"example.com/managed-by": "amp"}}
	labels := &map[string]string{"other": "x"}

	assert.False(t, c.hasResourceLabels(nil))
	assert.False(t, c.hasResourceLabels(labels))
	assert.Equal(t, map[string]string{"other": "x"}, *labels, "checking the labels does not change them")
	assert.True(t, c.hasResourceLabels(&map[string]string{"other": "x", "example.com/managed-by": "amp"}))
	assert.True(t, (&openChoreoClient{}).hasResourceLabels(nil), "no resource labels to carry")
}

func TestBindingAlreadyCurrent(t *testing.T) {
	release := "my-agent-1700"
	binding := func(rel string, configs *map[string]any) gen.ReleaseBinding {
		return gen.ReleaseBinding{Spec: &gen.ReleaseBindingSpec{ReleaseName: &rel, ComponentTypeEnvironmentConfigs: configs}}
	}
	hc := map[string]any{"readiness": map[string]any{"path": "/ready"}}

	assert.True(t, bindingAlreadyCurrent(binding(release, nil), release, nil))
	assert.True(t, bindingAlreadyCurrent(binding(release, &map[string]any{"healthChecks": hc, "runtimeClassName": "kata"}), release, hc))
	assert.False(t, bindingAlreadyCurrent(binding("my-agent-1600", nil), release, nil), "another release")
	assert.False(t, bindingAlreadyCurrent(binding(release, nil), release, hc), "health checks to add")
	assert.False(t, bindingAlreadyCurrent(binding(release, &map[string]any{"healthChecks": hc}), release, nil), "health checks to remove")
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
		want    any
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
			if tc.want == nil {
				// An untyped nil, so a caller's "!= nil" means the build has checks.
				assert.True(t, got == nil, "no health checks must be an untyped nil, got %#v", got)
			}
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
	got := withBuildClient(params, &BuildClientParams{SecretRefName: "cred-amp-build-token-org-1"})
	assert.Equal(t, map[string]interface{}{"secretRef": "cred-amp-build-token-org-1"}, got[workflowParamAMPBuild],
		"only a SecretReference name is passed, never a secret store location")
	_, leaked := params[workflowParamAMPBuild]
	assert.False(t, leaked, "the component's parameter map must not be modified")

	got = withBuildClient(params, &BuildClientParams{SecretRefName: "cred-amp-build-token-org-1", SecretStore: "secretstore-read"})
	assert.Equal(t, map[string]interface{}{"secretRef": "cred-amp-build-token-org-1", "secretStore": "secretstore-read"},
		got[workflowParamAMPBuild], "a configured store is passed by name alongside the SecretReference")
}

// withBuildSecretStore sets config.BuildClient.SecretStore for one test.
func withBuildSecretStore(t *testing.T, store string) {
	t.Helper()
	cfg := config.GetConfig()
	orig := cfg.BuildClient.SecretStore
	t.Cleanup(func() { cfg.BuildClient.SecretStore = orig })
	cfg.BuildClient.SecretStore = store
}

func TestAttachBuildClient_WithAConfiguredSecretStore(t *testing.T) {
	cfg := config.GetConfig()
	orig := cfg.BuildClient.Workflows
	t.Cleanup(func() { cfg.BuildClient.Workflows = orig })
	cfg.BuildClient.Workflows = []string{"amp-store", "amp-no-store"}

	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/namespaces/default/workflows/amp-store":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"metadata":{"name":"amp-store"},"spec":{"parameters":{"openAPIV3Schema":{"type":"object",
				"properties":{"ampBuild":{"type":"object","properties":{"secretRef":{"type":"string"},"secretStore":{"type":"string"}}}}}}}}`))
		case "/api/v1/namespaces/default/workflows/amp-no-store":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"metadata":{"name":"amp-no-store"},"spec":{"parameters":{"openAPIV3Schema":{"type":"object",
				"properties":{"ampBuild":{"type":"object","properties":{"secretRef":{"type":"string"}}}}}}}}`))
		default:
			t.Errorf("unexpected call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	ctx := context.Background()

	t.Run("no store configured: ampBuild alone is enough", func(t *testing.T) {
		withBuildSecretStore(t, "")
		got, err := c.attachBuildClient(ctx, "default", gen.WorkflowRunConfigKindWorkflow, "amp-no-store", true)
		require.NoError(t, err)
		assert.True(t, got)
	})
	t.Run("store configured: a workflow declaring ampBuild.secretStore takes the token", func(t *testing.T) {
		withBuildSecretStore(t, "secretstore-read")
		got, err := c.attachBuildClient(ctx, "default", gen.WorkflowRunConfigKindWorkflow, "amp-store", true)
		require.NoError(t, err)
		assert.True(t, got)
	})
	t.Run("store configured: a workflow without ampBuild.secretStore keeps the previous path", func(t *testing.T) {
		withBuildSecretStore(t, "secretstore-read")
		got, err := c.attachBuildClient(ctx, "default", gen.WorkflowRunConfigKindWorkflow, "amp-no-store", true)
		require.NoError(t, err, "not an error: the build runs on its previous path")
		assert.False(t, got, "it would read the token through a store that cannot read it")
	})
	t.Run("store configured: a listed ClusterWorkflow takes the token without being read", func(t *testing.T) {
		withBuildSecretStore(t, "secretstore-read")
		got, err := c.attachBuildClient(ctx, "default", gen.WorkflowRunConfigKindClusterWorkflow, "amp-no-store", true)
		require.NoError(t, err)
		assert.True(t, got, "a listed ClusterWorkflow is taken to declare ampBuild.secretStore")
	})
}

func TestAttachBuildClient(t *testing.T) {
	cfg := config.GetConfig()
	orig := cfg.BuildClient.Workflows
	t.Cleanup(func() { cfg.BuildClient.Workflows = orig })
	cfg.BuildClient.Workflows = []string{"amp-docker", "amp-old"}
	origDelay := buildWorkflowLookupRetryDelay
	t.Cleanup(func() { buildWorkflowLookupRetryDelay = origDelay })
	buildWorkflowLookupRetryDelay = time.Millisecond

	flakyCalls := 0
	brokenCalls := 0
	clusterWorkflowReads := 0
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/namespaces/default/workflows/amp-flaky":
			flakyCalls++
			if flakyCalls == 1 {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(`{"error":"bad gateway","code":"INTERNAL"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"metadata":{"name":"amp-flaky"},"spec":{"parameters":{"openAPIV3Schema":{
				"type":"object","properties":{"ampBuild":{"type":"object"}}}}}}`))
		case "/api/v1/namespaces/default/workflows/amp-broken":
			brokenCalls++
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom","code":"INTERNAL"}`))
		case "/api/v1/namespaces/default/workflows/amp-docker":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"metadata":{"name":"amp-docker"},"spec":{"parameters":{"openAPIV3Schema":{
				"type":"object","properties":{"ampBuild":{"type":"object"},"repository":{"type":"object"}}}}}}`))
		case "/api/v1/namespaces/default/workflows/amp-old":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"metadata":{"name":"amp-old"},"spec":{"parameters":{"openAPIV3Schema":{
				"type":"object","properties":{"repository":{"type":"object"}}}}}}`))
		default:
			if strings.HasPrefix(r.URL.Path, "/api/v1/clusterworkflows/") {
				clusterWorkflowReads++
			} else {
				t.Errorf("unexpected call %s %s", r.Method, r.URL.Path)
			}
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found","code":"NOT_FOUND"}`))
		}
	}))
	ctx := context.Background()

	attach := func(kind gen.WorkflowRunConfigKind, name string, enabled bool) (bool, error) {
		return c.attachBuildClient(ctx, "default", kind, name, enabled)
	}

	t.Run("a listed ClusterWorkflow takes the token without being read", func(t *testing.T) {
		got, err := attach(gen.WorkflowRunConfigKindClusterWorkflow, "amp-docker", true)
		require.NoError(t, err)
		assert.True(t, got)
		// amp-old declares no ampBuild as a namespaced Workflow; as a ClusterWorkflow
		// it is not read, so being listed is what decides.
		got, err = attach(gen.WorkflowRunConfigKindClusterWorkflow, "amp-old", true)
		require.NoError(t, err)
		assert.True(t, got)
		got, err = attach("", "amp-docker", true)
		require.NoError(t, err)
		assert.True(t, got, "an unset kind is a ClusterWorkflow")
		assert.Zero(t, clusterWorkflowReads, "a ClusterWorkflow is never read")
	})

	t.Run("a namespaced Workflow is read for ampBuild", func(t *testing.T) {
		got, err := attach(gen.WorkflowRunConfigKindWorkflow, "amp-docker", true)
		require.NoError(t, err)
		assert.True(t, got)

		got, err = attach(gen.WorkflowRunConfigKindWorkflow, "amp-old", true)
		require.NoError(t, err, "a workflow that is read and does not declare ampBuild is not an error")
		assert.False(t, got, "a workflow that does not declare ampBuild keeps its previous path")
	})

	t.Run("an unknown kind does not take the token and is not read", func(t *testing.T) {
		got, err := attach("SomethingElse", "amp-docker", true)
		require.NoError(t, err)
		assert.False(t, got)
	})

	t.Run("an unlisted workflow never takes the token", func(t *testing.T) {
		for _, kind := range []gen.WorkflowRunConfigKind{gen.WorkflowRunConfigKindClusterWorkflow, gen.WorkflowRunConfigKindWorkflow} {
			got, err := attach(kind, "dockerfile-builder", true)
			require.NoError(t, err)
			assert.False(t, got, "a workflow not listed in AMP_BUILD_CLIENT_WORKFLOWS never receives it (%s)", kind)
		}
	})

	t.Run("disabled: no workflow takes the token", func(t *testing.T) {
		for _, kind := range []gen.WorkflowRunConfigKind{gen.WorkflowRunConfigKindClusterWorkflow, gen.WorkflowRunConfigKindWorkflow} {
			got, err := attach(kind, "amp-docker", false)
			require.NoError(t, err)
			assert.False(t, got)
		}
	})

	t.Run("a namespaced Workflow that cannot be read fails the build", func(t *testing.T) {
		cfg.BuildClient.Workflows = []string{"amp-broken", "amp-flaky"}
		got, err := attach(gen.WorkflowRunConfigKindWorkflow, "amp-broken", true)
		require.ErrorIs(t, err, utils.ErrServiceUnavailable,
			"a workflow that cannot be read fails the build rather than silently keeping the previous path")
		assert.False(t, got)
		assert.Equal(t, 2, brokenCalls, "the lookup is retried exactly once")

		got, err = attach(gen.WorkflowRunConfigKindWorkflow, "amp-flaky", true)
		require.NoError(t, err, "a lookup that fails once and then succeeds is not an error")
		assert.True(t, got)
		assert.Equal(t, 2, flakyCalls)
	})

	assert.Zero(t, clusterWorkflowReads, "no case reads a ClusterWorkflow")
}

// triggerBuildStub serves the calls TriggerBuild makes for a component whose
// build workflow is workflowName, of workflowKind (ClusterWorkflow when empty).
// As namespaced Workflows, amp-docker declares ampBuild, amp-store declares
// ampBuild.secretStore too, amp-old declares neither and amp-broken cannot be
// read. A ClusterWorkflow read is answered 404, as an organization-scoped API in
// front of OpenChoreo answers it, and counted.
type triggerBuildStub struct {
	t                    *testing.T
	workflowName         string
	workflowKind         string
	createdRun           *gen.WorkflowRun
	clusterWorkflowReads int
	gitSecretRef         string
}

func (s *triggerBuildStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const ns = "/api/v1/namespaces/default"
	kind := s.workflowKind
	if kind == "" {
		kind = string(gen.WorkflowRunConfigKindClusterWorkflow)
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == ns+"/components/my-agent":
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"metadata":{"name":"my-agent"},"spec":{"owner":{"projectName":"proj1"},
			"componentType":{"name":"deployment/agent-api"},
			"workflow":{"kind":"` + kind + `","name":"` + s.workflowName + `","parameters":{"repository":{"url":"u","secretRef":` + strconv.Quote(s.gitSecretRef) + `}}}}}`))
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/clusterworkflows/"):
		s.clusterWorkflowReads++
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found","code":"NOT_FOUND"}`))
	case r.Method == http.MethodGet && r.URL.Path == ns+"/workflows/amp-docker":
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"metadata":{"name":"amp-docker"},"spec":{"parameters":{"openAPIV3Schema":{
			"type":"object","properties":{"ampBuild":{"type":"object"}}}}}}`))
	case r.Method == http.MethodGet && r.URL.Path == ns+"/workflows/amp-store":
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"metadata":{"name":"amp-store"},"spec":{"parameters":{"openAPIV3Schema":{"type":"object",
			"properties":{"ampBuild":{"type":"object","properties":{"secretRef":{"type":"string"},"secretStore":{"type":"string"}}}}}}}}`))
	case r.Method == http.MethodGet && r.URL.Path == ns+"/workflows/amp-old":
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"metadata":{"name":"amp-old"},"spec":{"parameters":{"openAPIV3Schema":{
			"type":"object","properties":{"repository":{"type":"object"}}}}}}`))
	case r.Method == http.MethodGet && r.URL.Path == ns+"/workflows/amp-broken":
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"unavailable","code":"INTERNAL"}`))
	case r.Method == http.MethodPost && r.URL.Path == ns+"/workflowruns":
		body, _ := io.ReadAll(r.Body)
		var run gen.WorkflowRun
		require.NoError(s.t, json.Unmarshal(body, &run))
		s.createdRun = &run
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(body)
	default:
		s.t.Errorf("unexpected call %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotImplemented)
	}
}

func TestTriggerBuildPassesTenantScopedGitSecretStoreKey(t *testing.T) {
	stub := &triggerBuildStub{t: t, workflowName: "amp-docker", gitSecretRef: "private"}
	c := newTestClient(t, stub)
	_, err := c.TriggerBuild(context.Background(), "org-1", "proj1", "my-agent", "", "my-agent-1700", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, stub.createdRun)
	params := *stub.createdRun.Spec.Workflow.Parameters
	assert.Equal(t, map[string]any{"url": "u"}, params["repository"], "the run carries no SecretReference name")
	assert.Equal(t, "org-1/"+GitSecretReferenceName("org-1", "private"), params[workflowParamGitSecretKVPath])
	assert.NotEqual(t, GitSecretReferenceName("org-1", "private"), GitSecretReferenceName("org-2", "private"))
}

func TestTriggerBuild_ProvisionsTheBuildClientOnlyForAWorkflowThatTakesIt(t *testing.T) {
	cfg := config.GetConfig()
	orig := cfg.BuildClient.Workflows
	t.Cleanup(func() { cfg.BuildClient.Workflows = orig })
	cfg.BuildClient.Workflows = []string{"amp-docker", "amp-old", "amp-broken"}
	origDelay := buildWorkflowLookupRetryDelay
	t.Cleanup(func() { buildWorkflowLookupRetryDelay = origDelay })
	buildWorkflowLookupRetryDelay = time.Millisecond

	failing := func(context.Context) (*BuildClientParams, error) {
		return nil, errors.New("thunder down")
	}

	t.Run("a listed ClusterWorkflow gets the client, and is never read", func(t *testing.T) {
		// Regression: the ClusterWorkflow read was answered 404 by an
		// organization-scoped API in front of OpenChoreo, and every build of it was
		// refused with a 503. The stub answers that read 404 the same way.
		stub := &triggerBuildStub{t: t, workflowName: "amp-docker"}
		c := newTestClient(t, stub)
		calls := 0
		source := func(context.Context) (*BuildClientParams, error) {
			calls++
			return &BuildClientParams{SecretRefName: "cred-amp-build-token-org-1"}, nil
		}

		_, err := c.TriggerBuild(context.Background(), "org-1", "proj1", "my-agent", "", "my-agent-1700", nil, source)
		require.NoError(t, err)
		assert.Zero(t, stub.clusterWorkflowReads, "a ClusterWorkflow is not read before a build")
		assert.Equal(t, 1, calls)
		require.NotNil(t, stub.createdRun)
		params := *stub.createdRun.Spec.Workflow.Parameters
		assert.Equal(t, map[string]any{"secretRef": "cred-amp-build-token-org-1"}, params[workflowParamAMPBuild])
	})

	t.Run("a namespaced Workflow that takes the client gets it", func(t *testing.T) {
		stub := &triggerBuildStub{t: t, workflowName: "amp-docker", workflowKind: string(gen.WorkflowRunConfigKindWorkflow)}
		c := newTestClient(t, stub)
		source := func(context.Context) (*BuildClientParams, error) {
			return &BuildClientParams{SecretRefName: "cred-amp-build-token-org-1"}, nil
		}

		_, err := c.TriggerBuild(context.Background(), "org-1", "proj1", "my-agent", "", "my-agent-1700", nil, source)
		require.NoError(t, err)
		require.NotNil(t, stub.createdRun)
		assert.Equal(t, map[string]any{"secretRef": "cred-amp-build-token-org-1"},
			(*stub.createdRun.Spec.Workflow.Parameters)[workflowParamAMPBuild])
	})

	t.Run("a workflow that does not take the token is not held up by issuing", func(t *testing.T) {
		stub := &triggerBuildStub{t: t, workflowName: "amp-old", workflowKind: string(gen.WorkflowRunConfigKindWorkflow)}
		c := newTestClient(t, stub)
		calls := 0
		source := func(ctx context.Context) (*BuildClientParams, error) {
			calls++
			return failing(ctx)
		}

		_, err := c.TriggerBuild(context.Background(), "org-1", "proj1", "my-agent", "", "my-agent-1700", nil, source)
		require.NoError(t, err, "an identity provider outage must not fail a build that would never carry the token")
		assert.Zero(t, calls, "the build token must not be issued for this workflow")
		require.NotNil(t, stub.createdRun)
		_, attached := (*stub.createdRun.Spec.Workflow.Parameters)[workflowParamAMPBuild]
		assert.False(t, attached)
	})

	t.Run("an issuing failure fails a build that takes the token", func(t *testing.T) {
		stub := &triggerBuildStub{t: t, workflowName: "amp-docker"}
		c := newTestClient(t, stub)

		_, err := c.TriggerBuild(context.Background(), "org-1", "proj1", "my-agent", "", "my-agent-1700", nil, failing)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "thunder down")
		assert.Nil(t, stub.createdRun, "no run is started that could not publish its Workload")
	})

	t.Run("an upstream not-found from issuing is not the build's not-found", func(t *testing.T) {
		stub := &triggerBuildStub{t: t, workflowName: "amp-docker"}
		c := newTestClient(t, stub)
		notFound := func(context.Context) (*BuildClientParams, error) {
			return nil, fmt.Errorf("%w: secret reference", utils.ErrNotFound)
		}

		_, err := c.TriggerBuild(context.Background(), "org-1", "proj1", "my-agent", "", "my-agent-1700", nil, notFound)
		require.Error(t, err)
		assert.NotErrorIs(t, err, utils.ErrNotFound)
	})

	t.Run("a workflow that cannot be read fails the build before a run is started", func(t *testing.T) {
		stub := &triggerBuildStub{t: t, workflowName: "amp-broken", workflowKind: string(gen.WorkflowRunConfigKindWorkflow)}
		c := newTestClient(t, stub)
		calls := 0
		source := func(context.Context) (*BuildClientParams, error) {
			calls++
			return &BuildClientParams{SecretRefName: "cred-amp-build-token-org-1"}, nil
		}

		_, err := c.TriggerBuild(context.Background(), "org-1", "proj1", "my-agent", "", "my-agent-1700", nil, source)
		require.ErrorIs(t, err, utils.ErrServiceUnavailable, "the caller gets a retryable 503 now, not a failed build later")
		assert.Zero(t, calls, "no token is issued when it is unknown whether the run takes it")
		assert.Nil(t, stub.createdRun, "no run is started on the previous path")
	})

	t.Run("a temporary issuing outage is retryable and names no upstream detail", func(t *testing.T) {
		stub := &triggerBuildStub{t: t, workflowName: "amp-docker"}
		c := newTestClient(t, stub)
		outage := func(context.Context) (*BuildClientParams, error) {
			return nil, fmt.Errorf("build token could not be issued: %w: token request failed: idp.internal:8443 refused",
				utils.ErrServiceUnavailable)
		}

		_, err := c.TriggerBuild(context.Background(), "org-1", "proj1", "my-agent", "", "my-agent-1700", nil, outage)
		require.ErrorIs(t, err, utils.ErrServiceUnavailable, "answered 503, so the caller knows a retry may work")
		assert.Contains(t, err.Error(), "could not prepare the build's credentials")
		assert.NotContains(t, err.Error(), "idp.internal", "the upstream detail goes to the log, not the response")
		assert.Nil(t, stub.createdRun)
	})

	t.Run("a non-temporary issuing failure is not marked retryable", func(t *testing.T) {
		stub := &triggerBuildStub{t: t, workflowName: "amp-docker"}
		c := newTestClient(t, stub)

		_, err := c.TriggerBuild(context.Background(), "org-1", "proj1", "my-agent", "", "my-agent-1700", nil, failing)
		require.Error(t, err)
		assert.NotErrorIs(t, err, utils.ErrServiceUnavailable)
		assert.Nil(t, stub.createdRun)
	})

	t.Run("a configured store rides on the run with the token", func(t *testing.T) {
		withBuildSecretStore(t, "secretstore-read")
		cfg.BuildClient.Workflows = []string{"amp-docker", "amp-old", "amp-broken", "amp-store"}
		source := func(context.Context) (*BuildClientParams, error) {
			return &BuildClientParams{SecretRefName: "cred-amp-build-token-org-1"}, nil
		}

		for _, kind := range []gen.WorkflowRunConfigKind{gen.WorkflowRunConfigKindClusterWorkflow, gen.WorkflowRunConfigKindWorkflow} {
			stub := &triggerBuildStub{t: t, workflowName: "amp-store", workflowKind: string(kind)}
			c := newTestClient(t, stub)

			_, err := c.TriggerBuild(context.Background(), "org-1", "proj1", "my-agent", "", "my-agent-1700", nil, source)
			require.NoError(t, err)
			require.NotNil(t, stub.createdRun)
			assert.Equal(t, map[string]any{"secretRef": "cred-amp-build-token-org-1", "secretStore": "secretstore-read"},
				(*stub.createdRun.Spec.Workflow.Parameters)[workflowParamAMPBuild], "kind %s", kind)
			assert.Zero(t, stub.clusterWorkflowReads)
		}
	})

	t.Run("a configured store and a workflow without ampBuild.secretStore: no token, previous path", func(t *testing.T) {
		withBuildSecretStore(t, "secretstore-read")
		stub := &triggerBuildStub{t: t, workflowName: "amp-docker", workflowKind: string(gen.WorkflowRunConfigKindWorkflow)}
		c := newTestClient(t, stub)
		calls := 0
		source := func(context.Context) (*BuildClientParams, error) {
			calls++
			return &BuildClientParams{SecretRefName: "cred-amp-build-token-org-1"}, nil
		}

		_, err := c.TriggerBuild(context.Background(), "org-1", "proj1", "my-agent", "", "my-agent-1700", nil, source)
		require.NoError(t, err)
		assert.Zero(t, calls, "no token is issued for a run that could not read it")
		require.NotNil(t, stub.createdRun)
		_, attached := (*stub.createdRun.Spec.Workflow.Parameters)[workflowParamAMPBuild]
		assert.False(t, attached)
	})

	t.Run("disabled: no token and nothing issued", func(t *testing.T) {
		stub := &triggerBuildStub{t: t, workflowName: "amp-docker"}
		c := newTestClient(t, stub)

		_, err := c.TriggerBuild(context.Background(), "org-1", "proj1", "my-agent", "", "my-agent-1700", nil, nil)
		require.NoError(t, err)
		_, attached := (*stub.createdRun.Spec.Workflow.Parameters)[workflowParamAMPBuild]
		assert.False(t, attached)
	})
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
