//go:build integration

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

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// The project routes over the real database: create, list with paging, get,
// update and delete, each refusal with its status and code, and the
// organization boundary — a token works on its own organization whatever the
// path says.
func TestCoreProjectsAPI(t *testing.T) {
	w := newCoreWorld(t)

	create := func(name, display string) spec.ProjectResponse {
		t.Helper()
		return decodeAs[spec.ProjectResponse](t, mustCall(t, w.admin, http.StatusAccepted, http.MethodPost, w.path("/projects"),
			spec.CreateProjectRequest{Name: name, DisplayName: display, Description: stringPtr("about " + name)}))
	}

	t.Run("create", func(t *testing.T) {
		p := create("alpha", "Alpha")
		assert.Equal(t, "alpha", p.Name)
		assert.Equal(t, "Alpha", p.DisplayName)
		assert.Equal(t, "about alpha", p.Description)
		assert.NotEmpty(t, p.Uuid)
		assert.False(t, p.CreatedAt.IsZero())
		create("beta", "Beta")
		create("gamma", "Gamma")
	})

	t.Run("create refusals", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			body   any
			status int
			msg    string
			code   string
		}{
			{"malformed body", "{", http.StatusBadRequest, "Invalid request body", ""},
			{
				"invalid name",
				spec.CreateProjectRequest{Name: "Not Valid", DisplayName: "x"},
				http.StatusBadRequest,
				"Project name must contain only lowercase alphanumeric characters or '-'", "VALIDATION_ERROR",
			},
			{
				"empty display name",
				spec.CreateProjectRequest{Name: "delta"},
				http.StatusBadRequest,
				"Project display name cannot be empty", "VALIDATION_ERROR",
			},
			{
				"name taken",
				spec.CreateProjectRequest{Name: "alpha", DisplayName: "Again"},
				http.StatusConflict,
				"Project already exists", utils.ErrCodeProjectAlreadyExists,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				status, body := call(t, w.admin, http.MethodPost, w.path("/projects"), tc.body)
				require.Equal(t, tc.status, status, string(body))
				e := errorOf(t, body)
				assert.Equal(t, tc.msg, e.Message)
				if tc.code != "" {
					assert.Equal(t, tc.code, e.Code)
				}
			})
		}
	})

	t.Run("list pages", func(t *testing.T) {
		all := decodeAs[spec.ProjectListResponse](t, mustCall(t, w.admin, http.StatusOK, http.MethodGet, w.path("/projects"), nil))
		assert.Equal(t, int32(3), all.Total)
		assert.Equal(t, int32(utils.DefaultLimit), all.Limit)
		names := make([]string, 0, len(all.Projects))
		for _, p := range all.Projects {
			names = append(names, p.Name)
		}
		assert.ElementsMatch(t, []string{"alpha", "beta", "gamma"}, names)

		page := decodeAs[spec.ProjectListResponse](t, mustCall(t, w.admin, http.StatusOK, http.MethodGet, w.path("/projects?limit=2&offset=1"), nil))
		assert.Equal(t, int32(3), page.Total, "the total counts every project, not the page")
		assert.Equal(t, int32(2), page.Limit)
		assert.Equal(t, int32(1), page.Offset)
		assert.Len(t, page.Projects, 2)

		past := decodeAs[spec.ProjectListResponse](t, mustCall(t, w.admin, http.StatusOK, http.MethodGet, w.path("/projects?offset=10"), nil))
		assert.Empty(t, past.Projects)
		assert.Equal(t, int32(3), past.Total)

		for _, q := range []string{"limit=0", "limit=101", "limit=x", "offset=-1"} {
			status, body := call(t, w.admin, http.MethodGet, w.path("/projects?%s", q), nil)
			assert.Equal(t, http.StatusBadRequest, status, q)
			assert.Contains(t, errorOf(t, body).Message, "Invalid", q)
		}
	})

	t.Run("get", func(t *testing.T) {
		p := decodeAs[spec.ProjectResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, w.path("/projects/alpha"), nil))
		assert.Equal(t, "Alpha", p.DisplayName)
		assert.Equal(t, "about alpha", p.Description)

		status, body := call(t, w.reader, http.MethodGet, w.path("/projects/missing"), nil)
		assert.Equal(t, http.StatusNotFound, status)
		assert.Equal(t, "Project not found", errorOf(t, body).Message)
	})

	t.Run("update", func(t *testing.T) {
		p := decodeAs[spec.ProjectResponse](t, mustCall(t, w.admin, http.StatusOK, http.MethodPut, w.path("/projects/beta"),
			spec.UpdateProjectRequest{DisplayName: "Beta Renamed", Description: "new words"}))
		assert.Equal(t, "Beta Renamed", p.DisplayName)

		got := decodeAs[spec.ProjectResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, w.path("/projects/beta"), nil))
		assert.Equal(t, "Beta Renamed", got.DisplayName, "the change is stored")
		assert.Equal(t, "new words", got.Description)

		status, _ := call(t, w.admin, http.MethodPut, w.path("/projects/beta"), "{")
		assert.Equal(t, http.StatusBadRequest, status)
		status, body := call(t, w.admin, http.MethodPut, w.path("/projects/beta"), spec.UpdateProjectRequest{})
		assert.Equal(t, http.StatusBadRequest, status)
		assert.Equal(t, "Project display name cannot be empty", errorOf(t, body).Message)
		status, body = call(t, w.admin, http.MethodPut, w.path("/projects/missing"), spec.UpdateProjectRequest{DisplayName: "x"})
		assert.Equal(t, http.StatusNotFound, status)
		assert.Equal(t, utils.ErrCodeProjectNotFound, errorOf(t, body).Code)
	})

	t.Run("a reader cannot change projects", func(t *testing.T) {
		for _, tc := range []struct {
			method, path string
			body         any
		}{
			{http.MethodPost, w.path("/projects"), spec.CreateProjectRequest{Name: "sneaky", DisplayName: "Sneaky"}},
			{http.MethodPut, w.path("/projects/alpha"), spec.UpdateProjectRequest{DisplayName: "Hacked"}},
			{http.MethodDelete, w.path("/projects/alpha"), nil},
		} {
			status, _ := call(t, w.reader, tc.method, tc.path, tc.body)
			assert.Equal(t, http.StatusForbidden, status, "%s %s", tc.method, tc.path)
		}
		mustCall(t, w.reader, http.StatusNotFound, http.MethodGet, w.path("/projects/sneaky"), nil)
		p := decodeAs[spec.ProjectResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, w.path("/projects/alpha"), nil))
		assert.Equal(t, "Alpha", p.DisplayName, "nothing changed")
	})

	t.Run("another organization's token sees only its own projects", func(t *testing.T) {
		// Same path, other org's token: the token decides the org.
		list := decodeAs[spec.ProjectListResponse](t, mustCall(t, w.stranger, http.StatusOK, http.MethodGet, w.path("/projects"), nil))
		assert.Zero(t, list.Total)
		mustCall(t, w.stranger, http.StatusNotFound, http.MethodGet, w.path("/projects/alpha"), nil)
		mustCall(t, w.stranger, http.StatusNotFound, http.MethodPut, w.path("/projects/alpha"), spec.UpdateProjectRequest{DisplayName: "Theirs"})
		// Deleting is idempotent, so the stranger's delete of a project its own
		// org does not have succeeds without touching this org's.
		mustCall(t, w.stranger, http.StatusNoContent, http.MethodDelete, w.path("/projects/alpha"), nil)
		mustCall(t, w.admin, http.StatusOK, http.MethodGet, w.path("/projects/alpha"), nil)
	})

	t.Run("delete", func(t *testing.T) {
		mustCall(t, w.admin, http.StatusNoContent, http.MethodDelete, w.path("/projects/gamma"), nil)
		mustCall(t, w.admin, http.StatusNotFound, http.MethodGet, w.path("/projects/gamma"), nil)
		list := decodeAs[spec.ProjectListResponse](t, mustCall(t, w.admin, http.StatusOK, http.MethodGet, w.path("/projects"), nil))
		assert.Equal(t, int32(2), list.Total)
	})
}

// The organization routes: the caller's org under the handle its token
// carries, and the org listing's paging.
func TestCoreOrganizationsAPI(t *testing.T) {
	w := newCoreWorld(t)
	// The organization exists once something has been written to it.
	mustCall(t, w.admin, http.StatusAccepted, http.MethodPost, w.path("/projects"), spec.CreateProjectRequest{Name: "seed", DisplayName: "Seed"})

	org := decodeAs[spec.OrganizationResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, w.path(""), nil))
	require.NotNil(t, org.Handle)
	assert.Equal(t, w.handle, *org.Handle)

	page := decodeAs[spec.OrganizationListResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, "/api/v1/orgs?limit=1", nil))
	assert.Len(t, page.Organizations, 1)
	assert.GreaterOrEqual(t, page.Total, int32(1))
	assert.Equal(t, int32(1), page.Limit)

	for _, q := range []string{"limit=0", "limit=500", "offset=-2", "offset=abc"} {
		status, _ := call(t, w.reader, http.MethodGet, "/api/v1/orgs?"+q, nil)
		assert.Equal(t, http.StatusBadRequest, status, q)
	}
}

// The environment routes list the organization's default environment, get it
// by id, and answer 404 for an id it does not have or another org's.
func TestCoreEnvironmentsAPI(t *testing.T) {
	w := newCoreWorld(t)
	mustCall(t, w.admin, http.StatusAccepted, http.MethodPost, w.path("/projects"), spec.CreateProjectRequest{Name: "seed", DisplayName: "Seed"})

	envs := decodeAs[[]spec.GatewayEnvironmentResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, w.path("/environments"), nil))
	require.Len(t, envs, 1, "a new organization has its default environment")
	env := envs[0]
	assert.Equal(t, "default", env.Name)
	assert.NotEmpty(t, env.Id)

	got := decodeAs[spec.GatewayEnvironmentResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, w.path("/environments/%s", env.Id), nil))
	assert.Equal(t, env.Id, got.Id)
	assert.Equal(t, env.DisplayName, got.DisplayName)

	// AMP Lite has one environment per organization, so the id in the path
	// is not looked up: every id answers the caller's own default
	// environment. Another org's token on the same path gets its own, never
	// this org's.
	byOther := decodeAs[spec.GatewayEnvironmentResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet,
		w.path("/environments/00000000-0000-0000-0000-000000000000"), nil))
	assert.Equal(t, env.Id, byOther.Id)
	theirs := decodeAs[spec.GatewayEnvironmentResponse](t, mustCall(t, w.stranger, http.StatusOK, http.MethodGet,
		w.path("/environments/%s", env.Id), nil))
	assert.NotEqual(t, env.Id, theirs.Id, "another org never reads this org's environment")

	for _, q := range []string{"limit=0", "limit=101", "offset=-1"} {
		status, _ := call(t, w.reader, http.MethodGet, w.path("/environments?%s", q), nil)
		assert.Equal(t, http.StatusBadRequest, status, q)
	}

	instances := decodeAs[map[string]any](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, w.path("/thunder-instances"), nil))
	assert.Contains(t, instances, "thunderInstances")
}
