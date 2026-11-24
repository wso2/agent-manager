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
	"net/http"
	"path"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// ownedComponentJSON is a component named name that belongs to project.
func ownedComponentJSON(name, project string) string {
	return `{"metadata":{"name":"` + name + `"},"spec":{"componentType":{"name":"proxy/agent-api"},"owner":{"projectName":"` + project + `"}}}`
}

// componentServer is a stub OpenChoreo holding one component, owned by project, and
// recording every request as "<METHOD> <last path segment>". A create answers 201 with the
// component named createdAs, as an OpenChoreo build that renames on conflict does.
type componentServer struct {
	name, project, createdAs string

	mu       sync.Mutex
	requests []string
}

func (s *componentServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.requests = append(s.requests, r.Method+" "+path.Base(r.URL.Path))
	s.mu.Unlock()

	switch r.Method {
	case http.MethodGet:
		if path.Base(r.URL.Path) != s.name {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found","code":"NOT_FOUND"}`))
			return
		}
		_, _ = w.Write([]byte(ownedComponentJSON(s.name, s.project)))
	case http.MethodPost:
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(ownedComponentJSON(s.createdAs, s.project)))
	case http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *componentServer) sent(method string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, r := range s.requests {
		if len(r) > len(method) && r[:len(method)+1] == method+" " {
			out = append(out, r[len(method)+1:])
		}
	}
	return out
}

func TestGetComponent_IsScopedToTheProject(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t, &componentServer{name: "support-agent", project: "project-a"})

	t.Run("its own project finds it", func(t *testing.T) {
		got, err := c.GetComponent(ctx, "ou", "project-a", "support-agent")
		require.NoError(t, err)
		assert.Equal(t, "project-a", got.ProjectName)
	})

	t.Run("project names compare case-insensitively", func(t *testing.T) {
		_, err := c.GetComponent(ctx, "ou", "PROJECT-A", "support-agent")
		require.NoError(t, err)
	})

	t.Run("another project's agent of that name is not found", func(t *testing.T) {
		_, err := c.GetComponent(ctx, "ou", "project-b", "support-agent")
		require.ErrorIs(t, err, utils.ErrNotFound)
	})

	t.Run("an empty project finds it in any project", func(t *testing.T) {
		got, err := c.GetComponent(ctx, "ou", "", "support-agent")
		require.NoError(t, err)
		assert.Equal(t, "project-a", got.ProjectName)
	})
}

// The create rollback and an agent delete name a project; deleting by name alone took
// another project's agent of the same name.
func TestDeleteComponent_LeavesAnotherProjectsComponent(t *testing.T) {
	srv := &componentServer{name: "support-agent", project: "project-a"}
	c := newTestClient(t, srv)

	err := c.DeleteComponent(context.Background(), "ou", "project-b", "support-agent")

	require.ErrorIs(t, err, utils.ErrNotFound, "the delete flow treats not found as already gone")
	assert.Empty(t, srv.sent(http.MethodDelete), "no DELETE reaches OpenChoreo")
}

func TestDeleteComponent_DeletesItsOwnProjectsComponent(t *testing.T) {
	srv := &componentServer{name: "support-agent", project: "project-a"}
	c := newTestClient(t, srv)

	require.NoError(t, c.DeleteComponent(context.Background(), "ou", "project-a", "support-agent"))
	assert.Equal(t, []string{"support-agent"}, srv.sent(http.MethodDelete))
}

func TestDeleteComponent_MissingComponentIsNotFound(t *testing.T) {
	srv := &componentServer{name: "other", project: "project-a"}
	c := newTestClient(t, srv)

	err := c.DeleteComponent(context.Background(), "ou", "project-a", "support-agent")

	require.ErrorIs(t, err, utils.ErrNotFound)
	assert.Empty(t, srv.sent(http.MethodDelete))
}

// Name availability is org-wide: OpenChoreo names components per namespace.
func TestComponentExists_CountsEveryProject(t *testing.T) {
	c := newTestClient(t, &componentServer{name: "support-agent", project: "project-a"})

	exists, err := c.ComponentExists(context.Background(), "ou", "project-b", "support-agent")

	require.NoError(t, err)
	assert.True(t, exists)
}

// A name is per namespace, so one used in another project is taken. Some OpenChoreo builds
// would create the component under a suffixed name instead of refusing, so nothing is sent.
func TestCreateComponent_RefusesANameTakenInAnotherProject(t *testing.T) {
	srv := &componentServer{name: "support-agent", project: "project-a", createdAs: "support-agent-x1y2"}
	c := newTestClient(t, srv)

	err := c.CreateComponent(context.Background(), "ou", "project-b",
		CreateComponentRequest{Name: "support-agent", ProvisioningType: ProvisioningExternal})

	require.ErrorIs(t, err, utils.ErrConflict)
	assert.Contains(t, err.Error(), `project "project-a"`, "the error names the project that holds the name")
	assert.Empty(t, srv.sent(http.MethodPost), "no create is sent")
	assert.Empty(t, srv.sent(http.MethodDelete))
}

// A holder still being deleted keeps the name until its cleanup ends; the caller can retry.
func TestCreateComponent_NameHeldByATerminatingComponent(t *testing.T) {
	c := newTestClient(t, conflictServer(t, terminatingMeta("support-agent")))

	err := c.CreateComponent(context.Background(), "ou", "project-b",
		CreateComponentRequest{Name: "support-agent", ProvisioningType: ProvisioningExternal})

	require.ErrorIs(t, err, utils.ErrResourceBeingDeleted)
}

// When the name is taken between the check and the create, an OpenChoreo build that renames on
// conflict answers 201 with a suffixed component. The caller must not go on as if it owned the
// requested name, and the suffixed component, this create's own, is removed.
func TestCreateComponent_RefusesARenamedCreate(t *testing.T) {
	srv := &componentServer{name: "unrelated", project: "project-a", createdAs: "support-agent-x1y2"}
	c := newTestClient(t, srv)

	err := c.CreateComponent(context.Background(), "ou", "project-b",
		CreateComponentRequest{Name: "support-agent", ProvisioningType: ProvisioningExternal})

	require.ErrorIs(t, err, utils.ErrConflict)
	assert.Equal(t, []string{"support-agent-x1y2"}, srv.sent(http.MethodDelete),
		"only the renamed component this create made is deleted")
}

func TestCreateComponent_AcceptsTheRequestedName(t *testing.T) {
	srv := &componentServer{name: "other", project: "project-b", createdAs: "support-agent"}
	c := newTestClient(t, srv)

	err := c.CreateComponent(context.Background(), "ou", "project-b",
		CreateComponentRequest{Name: "support-agent", ProvisioningType: ProvisioningExternal})

	require.NoError(t, err)
	assert.Empty(t, srv.sent(http.MethodDelete))
}

// A delete removes the secrets these names point at. The sources are listed by component name
// alone, so a same-named agent's bindings, workloads and component in another project must not
// hand over that agent's secrets.
func TestGetWorkloadSecretRefNames_KeepsOnlyItsOwnProjectsRefs(t *testing.T) {
	owned := func(project string, b gen.ReleaseBinding) gen.ReleaseBinding {
		b.Spec.Owner.ProjectName = project
		return b
	}
	bindings := gen.ReleaseBindingList{Items: []gen.ReleaseBinding{
		owned("my-project", bindingWithOverrides([]gen.EnvVar{secretEnvVar("API_KEY", "mine")}, nil)),
		owned("other-project", bindingWithOverrides([]gen.EnvVar{secretEnvVar("API_KEY", "theirs")}, nil)),
	}}
	var workloads gen.WorkloadList
	require.NoError(t, json.Unmarshal([]byte(`{"items":[{"metadata":{"name":"my-agent"},"spec":{
		"owner":{"componentName":"my-agent","projectName":"other-project"},
		"container":{"image":"x","env":[{"key":"K","valueFrom":{"secretKeyRef":{"name":"their-workload-secret"}}}]}}}]}`), &workloads))
	component := `{"metadata":{"name":"my-agent"},"spec":{"owner":{"projectName":"other-project"},
		"workflow":{"name":"amp-docker-build","parameters":{"environmentVariables":[
			{"name":"K","valueFrom":{"secretKeyRef":{"name":"their-build-secret"}}}]}}}}`
	c := newTestClient(t, serveListsAndComponent(t, bindings, workloads, component))

	names, err := c.GetWorkloadSecretRefNames(context.Background(), "acme", "my-project", "my-agent")

	require.NoError(t, err)
	assert.Equal(t, []string{"mine"}, names)
}

// OpenChoreo requires an owner project, so a source that names none is not shown to be this
// project's and contributes nothing.
func TestGetWorkloadSecretRefNames_SkipsSourcesWithoutAnOwner(t *testing.T) {
	ownerless := bindingWithOverrides([]gen.EnvVar{secretEnvVar("API_KEY", "ownerless-binding")}, nil)
	ownerless.Spec.Owner.ProjectName = ""
	var workloads gen.WorkloadList
	require.NoError(t, json.Unmarshal([]byte(`{"items":[{"metadata":{"name":"my-agent"},"spec":{
		"container":{"image":"x","env":[{"key":"K","valueFrom":{"secretKeyRef":{"name":"ownerless-workload"}}}]}}}]}`), &workloads))
	c := newTestClient(t, serveLists(t, gen.ReleaseBindingList{Items: []gen.ReleaseBinding{ownerless}}, workloads))

	names, err := c.GetWorkloadSecretRefNames(context.Background(), "acme", "my-project", "my-agent")

	require.NoError(t, err)
	assert.Empty(t, names)
}

// An error before the create is sent leaves no component behind, so it is marked: a caller
// cleaning up after an interrupted create must not delete a component of that name, which
// could only be another request's.
func TestCreateComponent_FailureBeforeSendingIsMarkedNotSent(t *testing.T) {
	srv := &componentServer{name: "other", project: "project-a", createdAs: "support-agent"}
	c := newTestClient(t, srv)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := c.CreateComponent(ctx, "ou", "project-b",
		CreateComponentRequest{Name: "support-agent", ProvisioningType: ProvisioningExternal})

	require.ErrorIs(t, err, ErrComponentCreateNotSent)
	require.ErrorIs(t, err, context.Canceled, "the cause is kept")
	assert.Empty(t, srv.sent(http.MethodPost))
}

// A conflict found by the name check keeps its message and its sentinel.
func TestCreateComponent_NameTakenIsMarkedNotSent(t *testing.T) {
	c := newTestClient(t, &componentServer{name: "support-agent", project: "project-a"})

	err := c.CreateComponent(context.Background(), "ou", "project-b",
		CreateComponentRequest{Name: "support-agent", ProvisioningType: ProvisioningExternal})

	require.ErrorIs(t, err, ErrComponentCreateNotSent)
	require.ErrorIs(t, err, utils.ErrConflict)
	assert.Equal(t, `conflict: component "support-agent" already exists in project "project-a"`, err.Error())
}
