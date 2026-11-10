//
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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// deletionTime is the deletionTimestamp stamped on the "already deleted" fixtures below.
// Its value is irrelevant — Kubernetes sets the field the moment a delete is accepted, and
// the object lingers for as long as its finalizers take, which may be well past this instant.
var deletionTime = time.Date(2026, time.September, 22, 10, 0, 0, 0, time.UTC)

func terminatingMeta(name string) gen.ObjectMeta {
	return gen.ObjectMeta{Name: name, DeletionTimestamp: &deletionTime}
}

func liveMeta(name string) gen.ObjectMeta {
	return gen.ObjectMeta{Name: name}
}

// agentComponent builds a Component carrying an agent component type. It goes through JSON
// because ComponentSpec.ComponentType is an anonymous struct that cannot be written as a
// composite literal from outside the generated package.
func agentComponent(t *testing.T, meta gen.ObjectMeta) gen.Component {
	t.Helper()
	metaJSON, err := json.Marshal(meta)
	require.NoError(t, err)

	var comp gen.Component
	require.NoError(t, json.Unmarshal([]byte(
		`{"metadata":`+string(metaJSON)+`,"spec":{"componentType":{"name":"proxy/agent-api"}}}`,
	), &comp))
	return comp
}

// A component that has been deleted keeps appearing in OpenChoreo's LIST until its cleanup
// finalizer runs, which is what made a deleted agent linger in the console for minutes.
func TestListComponents_SkipsTerminating(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(gen.ComponentList{Items: []gen.Component{
			agentComponent(t, liveMeta("keeper")),
			agentComponent(t, terminatingMeta("deleted-agent")),
		}}))
	}))

	agents, err := c.ListComponents(context.Background(), "ou-acme", "proj")

	require.NoError(t, err)
	require.Len(t, agents, 1)
	assert.Equal(t, "keeper", agents[0].Name)
}

// CountProjectComponents decides whether DeleteProject is allowed. Counting a component that
// is itself already terminating refused the very common "delete the agent, then delete the
// project that held it" sequence.
func TestCountProjectComponents_SkipsTerminating(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(gen.ComponentList{Items: []gen.Component{
			{Metadata: terminatingMeta("deleted-agent")},
			{Metadata: terminatingMeta("deleted-other-product-component")},
		}}))
	}))

	count, err := c.CountProjectComponents(context.Background(), "ou-acme", "proj")

	require.NoError(t, err)
	assert.Zero(t, count, "a project whose only components are terminating is empty and must be deletable")
}

// Same guard, for DeleteKind: an instance on its way out must not count as an instance.
func TestListComponentsByKind_SkipsTerminating(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(gen.ComponentList{Items: []gen.Component{
			agentComponent(t, terminatingMeta("deleted-instance")),
		}}))
	}))

	instances, err := c.ListComponentsByKind(context.Background(), "ou-acme", "proj", "my-kind")

	require.NoError(t, err)
	assert.Empty(t, instances)
}

// Projects carry the openchoreo.dev/project-cleanup finalizer, so they outlive their delete
// by longer than most.
func TestListProjects_SkipsTerminating(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(gen.ProjectList{Items: []gen.Project{
			{Metadata: terminatingMeta("deleted-project")},
			{Metadata: liveMeta("keeper")},
		}}))
	}))

	projects, err := c.ListProjects(context.Background(), "ou-acme")

	require.NoError(t, err)
	require.Len(t, projects, 1)
	assert.Equal(t, "keeper", projects[0].Name)
}

func TestListEnvironments_SkipsTerminating(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(gen.EnvironmentList{Items: []gen.Environment{
			{Metadata: liveMeta("prod")},
			{Metadata: terminatingMeta("deleted-staging")},
		}}))
	}))

	envs, err := c.ListEnvironments(context.Background(), "ou-acme")

	require.NoError(t, err)
	require.Len(t, envs, 1)
	assert.Equal(t, "prod", envs[0].Name)
}

func TestListDeploymentPipelines_SkipsTerminating(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(gen.DeploymentPipelineList{Items: []gen.DeploymentPipeline{
			{Metadata: terminatingMeta("deleted-pipeline")},
			{Metadata: liveMeta("default")},
		}}))
	}))

	pipelines, err := c.ListDeploymentPipelines(context.Background(), "ou-acme")

	require.NoError(t, err)
	require.Len(t, pipelines, 1)
	assert.Equal(t, "default", pipelines[0].Name)
}

// A live resource must still be listed — the filter keys off the deletionTimestamp only.
func TestIsTerminating(t *testing.T) {
	assert.False(t, isTerminating(liveMeta("live")))
	assert.True(t, isTerminating(terminatingMeta("gone")))
}

// conflictServer answers every create (POST) with 409 and every read (GET) with a
// resource carrying meta — the state OpenChoreo is in while a deleted resource of the
// same name is still finalizing (meta terminating) or genuinely exists (meta live).
func conflictServer(t *testing.T, meta gen.ObjectMeta) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"already exists","code":"CONFLICT"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"metadata": meta}))
	})
}

// A create that collides with a same-named resource still held by its cleanup finalizer
// must say so, not "already exists": the user just deleted that resource and the console
// no longer lists it, so a bare conflict reads as a bug.
func TestCreate_ConflictWithTerminatingResource(t *testing.T) {
	ctx := context.Background()
	creates := map[string]func(c *openChoreoClient) error{
		"component": func(c *openChoreoClient) error {
			return c.CreateComponent(ctx, "acme", "proj", CreateComponentRequest{Name: "stage", ProvisioningType: ProvisioningExternal})
		},
		"project": func(c *openChoreoClient) error {
			return c.CreateProject(ctx, "acme", CreateProjectRequest{Name: "stage", DeploymentPipeline: "default"})
		},
		"environment": func(c *openChoreoClient) error {
			_, err := c.CreateEnvironment(ctx, "acme", CreateEnvironmentRequest{Name: "stage"})
			return err
		},
		"deployment pipeline": func(c *openChoreoClient) error {
			_, err := c.CreateDeploymentPipeline(ctx, "acme", "stage", nil, nil, nil)
			return err
		},
	}

	for kind, create := range creates {
		t.Run(kind+"/terminating", func(t *testing.T) {
			err := create(newTestClient(t, conflictServer(t, terminatingMeta("stage"))))
			require.ErrorIs(t, err, utils.ErrResourceBeingDeleted)
			assert.NotErrorIs(t, err, utils.ErrConflict,
				"must not read as a plain conflict, or conflict-recovery paths would update an object that is going away")
		})
		t.Run(kind+"/live", func(t *testing.T) {
			err := create(newTestClient(t, conflictServer(t, liveMeta("stage"))))
			require.ErrorIs(t, err, utils.ErrConflict)
			assert.NotErrorIs(t, err, utils.ErrResourceBeingDeleted)
		})
	}
}
