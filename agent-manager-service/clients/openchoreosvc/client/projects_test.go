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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// OpenChoreo 1.2.0+ requires Project.spec.type, and rejects a change to it after
// creation, so the create request is the only chance to get it right. The kind
// must be the namespaced ProjectType: a ClusterProjectType is shared by every
// tenant in the cluster, which a multi-tenant deployment cannot use.
func TestCreateProject_SetsNamespacedProjectType(t *testing.T) {
	var gotBody gen.Project
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.WriteHeader(http.StatusCreated)
		require.NoError(t, json.NewEncoder(w).Encode(gotBody))
	}))

	err := c.CreateProject(context.Background(), "acme", CreateProjectRequest{
		Name:               "my-project",
		DisplayName:        "My Project",
		Description:        "desc",
		DeploymentPipeline: "default",
	})
	require.NoError(t, err)

	require.NotNil(t, gotBody.Spec)
	require.NotNil(t, gotBody.Spec.Type, "spec.type is required from OpenChoreo 1.2.0 — omitting it fails CRD validation")
	require.NotNil(t, gotBody.Spec.Type.Kind)
	assert.Equal(t, gen.ProjectTypeRefKindProjectType, *gotBody.Spec.Type.Kind,
		"must reference the namespaced ProjectType, not the cluster-wide ClusterProjectType")
	assert.Equal(t, DefaultProjectTypeName, gotBody.Spec.Type.Name)

	// The pipeline reference must survive alongside the new field.
	require.NotNil(t, gotBody.Spec.DeploymentPipelineRef)
	assert.Equal(t, "default", gotBody.Spec.DeploymentPipelineRef.Name)
}

// This name is a contract with the Helm chart, which hardcodes the same literal
// in templates/project-type.yaml and templates/project.yaml — deliberately not a
// configurable value, because if the two diverged the chart would provision a
// ProjectType under one name while the service referenced another, and every
// project created through the API would fail with ProjectTypeNotFound.
func TestDefaultProjectTypeName(t *testing.T) {
	assert.Equal(t, "default", DefaultProjectTypeName)
}

// Controllers map ErrProjectNotFound to a 404 and DeleteProject treats it as an
// idempotent success; a bare ErrNotFound reaches the client as a 500. Callers that
// translate the generic ErrNotFound themselves must keep matching it.
func TestProjectScopedCalls_NotFoundIsProjectNotFound(t *testing.T) {
	ctx := context.Background()
	calls := map[string]func(c *openChoreoClient) error{
		"GetProject": func(c *openChoreoClient) error {
			_, err := c.GetProject(ctx, "acme", "missing")
			return err
		},
		"PatchProject": func(c *openChoreoClient) error {
			return c.PatchProject(ctx, "acme", "missing", PatchProjectRequest{DeploymentPipeline: "default"})
		},
		"DeleteProject": func(c *openChoreoClient) error {
			return c.DeleteProject(ctx, "acme", "missing")
		},
		"ListComponents": func(c *openChoreoClient) error {
			_, err := c.ListComponents(ctx, "acme", "missing")
			return err
		},
		"CountProjectComponents": func(c *openChoreoClient) error {
			_, err := c.CountProjectComponents(ctx, "acme", "missing")
			return err
		},
	}

	for name, call := range calls {
		t.Run(name+"/404", func(t *testing.T) {
			c := newTestClient(t, statusServer(http.StatusNotFound, gen.NOTFOUND))
			err := call(c)
			require.ErrorIs(t, err, utils.ErrProjectNotFound)
			require.ErrorIs(t, err, utils.ErrNotFound)
		})
		t.Run(name+"/404 without OpenChoreo body", func(t *testing.T) {
			c := newTestClientRawHeaders(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte("<html>404 Not Found</html>"))
			}))
			err := call(c)
			require.Error(t, err)
			require.NotErrorIs(t, err, utils.ErrProjectNotFound)
			require.NotErrorIs(t, err, utils.ErrNotFound)
		})
		t.Run(name+"/500", func(t *testing.T) {
			c := newTestClient(t, statusServer(http.StatusInternalServerError, gen.INTERNALERROR))
			err := call(c)
			require.ErrorIs(t, err, utils.ErrInternalServerError)
			require.NotErrorIs(t, err, utils.ErrProjectNotFound)
		})
	}
}

func statusServer(status int, code gen.ErrorResponseCode) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(gen.ErrorResponse{Code: code, Error: "upstream says " + string(code)})
	})
}

// A missing project must reach the controller as a project-specific 404,
// including when cleanup polls after a successful deletion.
func TestGetProject_ErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		want   error
	}{
		{"missing project", http.StatusNotFound, utils.ErrProjectNotFound},
		{"server failure", http.StatusInternalServerError, utils.ErrInternalServerError},
		{"forbidden", http.StatusForbidden, utils.ErrForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				require.NoError(t, json.NewEncoder(w).Encode(map[string]string{"error": tc.name}))
			}))
			_, err := c.GetProject(context.Background(), "acme", "deleted-project")
			require.ErrorIs(t, err, tc.want)
			if tc.status != http.StatusNotFound {
				assert.NotErrorIs(t, err, utils.ErrProjectNotFound)
			}
		})
	}
}
