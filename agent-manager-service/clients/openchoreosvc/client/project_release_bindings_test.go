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
//

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// newTestClient wires an openChoreoClient against a stub OpenChoreo API.
func newTestClient(t *testing.T, handler http.Handler) *openChoreoClient {
	t.Helper()
	srv := httptest.NewServer(jsonContentType(handler))
	t.Cleanup(srv.Close)

	gc, err := gen.NewClientWithResponses(srv.URL)
	require.NoError(t, err)
	return &openChoreoClient{ocClient: gc, defaultNamespace: "default"}
}

// jsonContentType stamps the header the generated response parsers key on to
// decide whether a body is decodable — without it every stub reply parses as an
// empty struct.
func jsonContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

func TestProjectReleaseBindingName(t *testing.T) {
	// Must stay in step with the Helm chart's
	// templates/project-release-binding.yaml, or the chart-managed binding for
	// the default project would be duplicated under a second name.
	assert.Equal(t, "my-project-dev", projectReleaseBindingName("my-project", "dev"))
}

func TestEnsureProjectReleaseBinding_CreatesBinding(t *testing.T) {
	var gotBody gen.ProjectReleaseBinding
	var gotPath string
	srv := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.WriteHeader(http.StatusCreated)
		require.NoError(t, json.NewEncoder(w).Encode(gotBody))
	}))

	err := srv.EnsureProjectReleaseBinding(context.Background(), "acme", "my-project", "dev")

	require.NoError(t, err)
	assert.Equal(t, "/api/v1/namespaces/default/projectreleasebindings", gotPath)
	assert.Equal(t, "my-project-dev", gotBody.Metadata.Name)
	require.NotNil(t, gotBody.Spec)
	assert.Equal(t, "my-project", gotBody.Spec.Owner.ProjectName)
	assert.Equal(t, "dev", gotBody.Spec.Environment)
	// The Project controller seeds the pin with the latest ProjectRelease;
	// sending one here would freeze the project at whatever release existed now.
	assert.Nil(t, gotBody.Spec.ProjectRelease)
}

// The organization's UUID has to reach the binding, because the project type
// merges these labels onto the cell namespace and that is the only place usage
// measured from pod metrics can be attributed to an organization. A missing
// label breaks nothing at create time -- it yields pods that meter correctly and
// bill to no customer -- so it is asserted rather than left to inspection.
func TestEnsureProjectReleaseBinding_StampsOrgUUIDOnCellNamespace(t *testing.T) {
	var gotBody gen.ProjectReleaseBinding
	srv := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.WriteHeader(http.StatusCreated)
		require.NoError(t, json.NewEncoder(w).Encode(gotBody))
	}))

	const ouID = "019eafc8-0f23-7974-9119-d762c59c83a1"
	require.NoError(t, srv.EnsureProjectReleaseBinding(context.Background(), ouID, "my-project", "dev"))

	require.NotNil(t, gotBody.Spec)
	require.NotNil(t, gotBody.Spec.EnvironmentConfigs)
	nsLabels, ok := (*gotBody.Spec.EnvironmentConfigs)["namespaceLabels"].(map[string]interface{})
	require.True(t, ok, "namespaceLabels missing: %#v", *gotBody.Spec.EnvironmentConfigs)
	assert.Equal(t, ouID, nsLabels[string(LabelKeyOrgUUID)])
}

func TestEnsureProjectReleaseBinding_ExistingBindingIsSuccess(t *testing.T) {
	srv := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusConflict)
			require.NoError(t, json.NewEncoder(w).Encode(gen.Conflict{Error: "already exists"}))
			return
		}
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(gen.ProjectReleaseBinding{
			Metadata: gen.ObjectMeta{Name: "my-project-dev"},
			Spec: &gen.ProjectReleaseBindingSpec{
				Owner: struct {
					ProjectName string `json:"projectName"`
				}{ProjectName: "my-project"},
				Environment: "dev",
			},
		}))
	}))

	err := srv.EnsureProjectReleaseBinding(context.Background(), "acme", "my-project", "dev")

	require.NoError(t, err, "an existing binding for the same project and environment is what we wanted")
}

// Bindings created before the org UUID label existed are never re-created, so
// the 409 path is the only place they can pick it up. Other environment configs
// and namespace labels on the binding must survive the update.
func TestEnsureProjectReleaseBinding_BackfillsOrgUUIDOnExistingBinding(t *testing.T) {
	const ouID = "019eafc8-0f23-7974-9119-d762c59c83a1"
	var updated *gen.ProjectReleaseBinding
	srv := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusConflict)
			require.NoError(t, json.NewEncoder(w).Encode(gen.Conflict{Error: "already exists"}))
		case http.MethodPut:
			updated = &gen.ProjectReleaseBinding{}
			require.NoError(t, json.NewDecoder(r.Body).Decode(updated))
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(updated))
		default:
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(gen.ProjectReleaseBinding{
				Metadata: gen.ObjectMeta{Name: "my-project-dev"},
				Spec: &gen.ProjectReleaseBindingSpec{
					Owner: struct {
						ProjectName string `json:"projectName"`
					}{ProjectName: "my-project"},
					Environment: "dev",
					EnvironmentConfigs: &map[string]interface{}{
						"other":            "kept",
						namespaceLabelsKey: map[string]string{"team": "a"},
					},
				},
			}))
		}
	}))

	require.NoError(t, srv.EnsureProjectReleaseBinding(context.Background(), ouID, "my-project", "dev"))

	require.NotNil(t, updated, "existing binding without the label must be updated")
	configs := *updated.Spec.EnvironmentConfigs
	assert.Equal(t, "kept", configs["other"])
	nsLabels, ok := configs[namespaceLabelsKey].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, ouID, nsLabels[string(LabelKeyOrgUUID)])
	assert.Equal(t, "a", nsLabels["team"])
}

func TestEnsureProjectReleaseBinding_ExistingLabelledBindingIsNotUpdated(t *testing.T) {
	const ouID = "019eafc8-0f23-7974-9119-d762c59c83a1"
	srv := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusConflict)
			require.NoError(t, json.NewEncoder(w).Encode(gen.Conflict{Error: "already exists"}))
		case http.MethodPut:
			t.Error("binding already carries the label; no update expected")
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(gen.ProjectReleaseBinding{
				Metadata: gen.ObjectMeta{Name: "my-project-dev"},
				Spec: &gen.ProjectReleaseBindingSpec{
					Owner: struct {
						ProjectName string `json:"projectName"`
					}{ProjectName: "my-project"},
					Environment: "dev",
					EnvironmentConfigs: &map[string]interface{}{
						namespaceLabelsKey: map[string]string{string(LabelKeyOrgUUID): ouID},
					},
				},
			}))
		}
	}))

	require.NoError(t, srv.EnsureProjectReleaseBinding(context.Background(), ouID, "my-project", "dev"))
}

// The label is for metering only, so failing to backfill it must not block the
// deploy that triggered the ensure.
func TestEnsureProjectReleaseBinding_BackfillFailureIsNotFatal(t *testing.T) {
	srv := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusConflict)
			require.NoError(t, json.NewEncoder(w).Encode(gen.Conflict{Error: "already exists"}))
		case http.MethodPut:
			w.WriteHeader(http.StatusForbidden)
			require.NoError(t, json.NewEncoder(w).Encode(gen.Forbidden{Error: "no permission"}))
		default:
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(gen.ProjectReleaseBinding{
				Metadata: gen.ObjectMeta{Name: "my-project-dev"},
				Spec: &gen.ProjectReleaseBindingSpec{
					Owner: struct {
						ProjectName string `json:"projectName"`
					}{ProjectName: "my-project"},
					Environment: "dev",
				},
			}))
		}
	}))

	require.NoError(t, srv.EnsureProjectReleaseBinding(context.Background(), "acme", "my-project", "dev"))
}

func TestEnsureProjectReleaseBinding_NameCollisionWithAnotherProjectIsAnError(t *testing.T) {
	// Binding names are "<project>-<environment>", so project "my" + env
	// "project-dev" collides with project "my-project" + env "dev". Treating
	// that as success would silently point one project at another's namespace.
	srv := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusConflict)
			require.NoError(t, json.NewEncoder(w).Encode(gen.Conflict{Error: "already exists"}))
			return
		}
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(gen.ProjectReleaseBinding{
			Metadata: gen.ObjectMeta{Name: "my-project-dev"},
			Spec: &gen.ProjectReleaseBindingSpec{
				Owner: struct {
					ProjectName string `json:"projectName"`
				}{ProjectName: "my"},
				Environment: "project-dev",
			},
		}))
	}))

	err := srv.EnsureProjectReleaseBinding(context.Background(), "acme", "my-project", "dev")

	require.Error(t, err)
	assert.Contains(t, err.Error(), `already exists for project "my" environment "project-dev"`)
}

func TestEnsureProjectReleaseBinding_PropagatesAPIErrors(t *testing.T) {
	srv := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		require.NoError(t, json.NewEncoder(w).Encode(gen.Forbidden{Error: "no permission"}))
	}))

	err := srv.EnsureProjectReleaseBinding(context.Background(), "acme", "my-project", "dev")

	assert.ErrorIs(t, err, utils.ErrForbidden)
}
